package chat

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/storage"
)

func testService(t *testing.T) *app.Service {
	t.Helper()
	st, e := storage.Init(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	s := &app.Service{Repo: st}
	if e = s.CreateProject(context.Background(), "p"); e != nil {
		t.Fatal(e)
	}
	return s
}
func args(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func TestConflictRequiresReadBeforeRetry(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	key := app.Key{Project: "p", Kind: "stock", Path: "概要.md"}
	old, e := s.Create(ctx, key, "# 概要\n既存本文\n")
	if e != nil {
		t.Fatal(e)
	}
	var events []Event
	h := &toolHandler{service: s, project: "p", read: map[app.Key]string{}, conflicted: map[app.Key]bool{}, emit: func(e Event) { events = append(events, e) }}
	read := args(map[string]any{"kind": "stock", "path": key.Path})
	if _, ok := h.call(ctx, "fragment_get_document", read); !ok {
		t.Fatal("initial read failed")
	}
	concurrent, e := s.Update(ctx, key, old.Content+"別クライアントからの追記\n", old.Revision)
	if e != nil {
		t.Fatal(e)
	}
	stale := args(map[string]string{"kind": "stock", "path": key.Path, "content": old.Content + "AI追記\n", "revision": old.Revision})
	value, ok := h.call(ctx, "fragment_update_document", stale)
	if ok || value.(map[string]any)["retry_required"] != true || len(events) != 1 || events[0].Type != "conflict" || !strings.Contains(events[0].Text, "リトライ") {
		t.Fatal(value, events)
	}
	// Knowing the new revision from elsewhere still does not allow a blind retry.
	value, ok = h.call(ctx, "fragment_update_document", args(map[string]string{"kind": "stock", "path": key.Path, "content": "stale", "revision": concurrent.Revision}))
	if ok || value.(map[string]any)["error"].(*app.Error).Code != "reread_required" {
		t.Fatal(value)
	}
	value, ok = h.call(ctx, "fragment_get_document", read)
	if !ok {
		t.Fatal(value)
	}
	value, ok = h.call(ctx, "fragment_update_document", args(map[string]string{"kind": "stock", "path": key.Path, "content": old.Content + "AI追記\n", "revision": concurrent.Revision}))
	if ok || value.(map[string]any)["error"].(*app.Error).Code != "reconsider_required" {
		t.Fatal("stale proposal was accepted after re-read", value)
	}
	value, ok = h.call(ctx, "fragment_get_document", read)
	if !ok {
		t.Fatal(value)
	}
	latest := value.(app.Document)
	if !strings.Contains(latest.Content, "別クライアント") {
		t.Fatal(latest)
	}
	value, ok = h.call(ctx, "fragment_update_document", args(map[string]string{"kind": "stock", "path": key.Path, "content": latest.Content + "AI追記\n", "revision": latest.Revision}))
	if !ok {
		t.Fatal(value)
	}
	final, e := s.Get(ctx, key)
	if e != nil || !strings.Contains(final.Content, "別クライアント") || !strings.Contains(final.Content, "AI追記") {
		t.Fatal(final, e)
	}
	if events[1].Type != "retry" || events[len(events)-1].Type != "updated" {
		t.Fatal(events)
	}
}

type runnerFunc func(context.Context, string, func(context.Context, string, json.RawMessage) (any, bool), func(Event)) error

func (f runnerFunc) Run(ctx context.Context, prompt string, tool func(context.Context, string, json.RawMessage) (any, bool), emit func(Event)) error {
	return f(ctx, prompt, tool, emit)
}
func waitChat(t *testing.T, m *Manager) Snapshot {
	t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		state, e := m.State("p")
		if e != nil {
			t.Fatal(e)
		}
		if !state.Busy {
			return state
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("chat did not finish")
	return Snapshot{}
}
func TestConversationPersistsAndKeepsExternalFlowEdits(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	started := make(chan struct{})
	release := make(chan struct{})
	runner := runnerFunc(func(ctx context.Context, prompt string, tool func(context.Context, string, json.RawMessage) (any, bool), emit func(Event)) error {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		if _, ok := tool(ctx, "fragment_create_document", args(map[string]string{"kind": "stock", "path": "保存.md", "content": "# 保存された内容"})); !ok {
			t.Error("write failed")
		}
		emit(Event{Type: "delta", Item: "a", Text: "保存"})
		emit(Event{Type: "assistant", Item: "a", Text: "保存しました。"})
		return nil
	})
	m := New(ctx, s, runner)
	defer m.Close()
	state, e := m.Send(ctx, "p", "stockに保存して")
	if e != nil {
		t.Fatal(e)
	}
	<-started
	if _, e = m.Send(ctx, "p", "二重送信"); app.ErrorInfo(e).Code != "chat_busy" {
		t.Fatal(e)
	}
	key := app.Key{Project: "p", Kind: "flow", Path: state.FlowPath}
	doc, e := s.Get(ctx, key)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Update(ctx, key, doc.Content+"\n外部からのメモ\n", doc.Revision); e != nil {
		t.Fatal(e)
	}
	close(release)
	final := waitChat(t, m)
	flow, e := s.Get(ctx, key)
	if e != nil || !strings.Contains(flow.Content, "外部からのメモ") || !strings.Contains(flow.Content, "stockに保存して") || strings.Count(flow.Content, "## assistant\n\n保存しました。") != 1 {
		t.Fatal(flow, e)
	}
	if len(final.Messages) != 3 || final.Messages[2].Text != "保存しました。" {
		t.Fatal(final)
	}
}

// A controlled concurrent writer forces a real Codex turn to re-read and merge.
// Opt in explicitly: FRAGMENT_CHAT_LIVE_TEST=1 go test ./internal/chat -run TestLiveCodex -v
// This consumes the signed-in Codex account and touches only temporary data.
type conflictRepo struct {
	app.Repository
	service *app.Service
	key     app.Key
	once    sync.Once
}

func (r *conflictRepo) Read(ctx context.Context, k app.Key) (app.Document, error) {
	doc, e := r.Repository.Read(ctx, k)
	if e == nil && k == r.key {
		r.once.Do(func() {
			_, _ = r.service.Update(ctx, k, doc.Content+"\nConcurrentMarker: 外部クライアントの変更は残してください。\n", doc.Revision)
		})
	}
	return doc, e
}
func TestLiveCodex(t *testing.T) {
	if os.Getenv("FRAGMENT_CHAT_LIVE_TEST") != "1" {
		t.Skip("explicit live inference opt-in required")
	}
	s := testService(t)
	ctx := context.Background()
	key := app.Key{Project: "p", Kind: "stock", Path: "概要.md"}
	if _, e := s.Create(ctx, key, "# 概要\n\n既存の重要な説明。\n"); e != nil {
		t.Fatal(e)
	}
	wrapper := &app.Service{Repo: &conflictRepo{Repository: s.Repo, service: s, key: key}}
	m := New(ctx, wrapper, Codex{Executable: os.Getenv("FRAGMENT_CHAT_LIVE_CODEX"), Model: os.Getenv("FRAGMENT_CHAT_LIVE_MODEL")})
	defer m.Close()
	if _, e := m.Send(ctx, "p", "stock/概要.mdの既存の内容を保ち、末尾に『AI追加: チャットから更新しました。』という行を追記して保存してください。競合した場合は最新の本文を再取得し、他の変更を保持した上で追記をリトライしてください。"); e != nil {
		t.Fatal(e)
	}
	state := waitChat(t, m)
	for _, msg := range state.Messages {
		t.Log(msg.Role, msg.Type, msg.Text)
	}
	doc, e := s.Get(ctx, key)
	if e != nil {
		t.Fatal(e)
	}
	conflict, retry, updated := false, false, false
	for _, msg := range state.Messages {
		conflict = conflict || msg.Type == "conflict"
		retry = retry || msg.Type == "retry"
		updated = updated || msg.Type == "updated"
	}
	if !conflict || !retry || !updated || !strings.Contains(doc.Content, "ConcurrentMarker") || !strings.Contains(doc.Content, "AI追加") || !strings.Contains(doc.Content, "既存の重要な説明") {
		t.Fatal("live chat did not preserve concurrent data", doc, state)
	}
}
