package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/s4na/fragment/internal/app"
)

const maxChatBytes = 256 << 10

type Message struct {
	Role string   `json:"role"`
	Text string   `json:"text"`
	Type string   `json:"type,omitempty"`
	Key  *app.Key `json:"key,omitempty"`
}
type Snapshot struct {
	Messages []Message `json:"messages"`
	Busy     bool      `json:"busy"`
	FlowPath string    `json:"flow_path"`
	Version  int       `json:"version"`
}
type conversation struct {
	mu    sync.Mutex
	state Snapshot
	key   app.Key
}
type Manager struct {
	ctx           context.Context
	cancel        context.CancelFunc
	service       *app.Service
	runner        Runner
	mu            sync.Mutex
	conversations map[string]*conversation
	workers       sync.WaitGroup
	slots         chan struct{}
	closed        bool
}

func New(ctx context.Context, s *app.Service, runner Runner) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	return &Manager{ctx: ctx, cancel: cancel, service: s, runner: runner, conversations: make(map[string]*conversation), slots: make(chan struct{}, 4)}
}
func (m *Manager) Close() { m.mu.Lock(); m.closed = true; m.cancel(); m.mu.Unlock(); m.workers.Wait() }
func (m *Manager) State(project string) (Snapshot, error) {
	if err := app.ValidateProject(project); err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	c := m.conversations[project]
	m.mu.Unlock()
	if c == nil {
		return Snapshot{Messages: []Message{}}, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state
	state.Messages = append([]Message{}, state.Messages...)
	return state, nil
}
func (m *Manager) Send(ctx context.Context, project, text string) (Snapshot, error) {
	if err := app.ValidateProject(project); err != nil {
		return Snapshot{}, err
	}
	if strings.TrimSpace(text) == "" || len(text) > 32<<10 || !utf8.ValidString(text) {
		return Snapshot{}, app.Fail("invalid_input", "チャットは空でないUTF-8の本文（32KiB以内）にしてください")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Snapshot{}, app.Fail("unavailable", "サーバーを終了しています")
	}
	c := m.conversations[project]
	if c == nil {
		// This validates that the project exists before launching any inference.
		if _, err := m.service.Documents(ctx, project, "flow", "", 1); err != nil {
			m.mu.Unlock()
			return Snapshot{}, err
		}
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			m.mu.Unlock()
			return Snapshot{}, err
		}
		key := app.Key{Project: project, Kind: "flow", Path: "chats/" + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b) + ".md"}
		if _, err := m.service.Create(ctx, key, "# チャット\n"); err != nil {
			m.mu.Unlock()
			return Snapshot{}, err
		}
		c = &conversation{key: key, state: Snapshot{FlowPath: key.Path, Messages: []Message{}}}
		m.conversations[project] = c
	}
	c.mu.Lock()
	if c.state.Busy {
		c.mu.Unlock()
		m.mu.Unlock()
		return Snapshot{}, app.Fail("chat_busy", "このプロジェクトの応答を待ってください")
	}
	if len(transcriptText(c.state.Messages))+len(text) > maxChatBytes {
		c.mu.Unlock()
		m.mu.Unlock()
		return Snapshot{}, app.Fail("too_large", "会話が長くなりました。サーバーを再起動して新しいチャットを始めてください。以前の会話はflowに残ります")
	}
	select {
	case m.slots <- struct{}{}:
	default:
		c.mu.Unlock()
		m.mu.Unlock()
		return Snapshot{}, app.Fail("chat_busy", "同時実行数の上限です。応答を待ってからリトライしてください")
	}
	user := Message{Role: "user", Text: text}
	if err := m.append(ctx, c.key, []Message{user}); err != nil {
		<-m.slots
		c.mu.Unlock()
		m.mu.Unlock()
		return Snapshot{}, err
	}
	history := transcriptText(c.state.Messages)
	c.state.Messages = append(c.state.Messages, user)
	c.state.Busy = true
	c.state.Version++
	start := len(c.state.Messages)
	c.mu.Unlock()
	m.workers.Add(1)
	m.mu.Unlock()
	go m.run(c, history, text, start)
	return m.State(project)
}
func (m *Manager) run(c *conversation, history, text string, start int) {
	defer m.workers.Done()
	defer func() { <-m.slots }()
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	defer cancel()
	items := map[string]int{}
	emit := func(e Event) {
		c.mu.Lock()
		defer c.mu.Unlock()
		// Bound model output and polling payloads. Cancellation still reports failure.
		size := len(transcriptText(c.state.Messages)) + len(e.Text)
		if e.Type == "assistant" {
			if index, ok := items[e.Item]; ok {
				size -= len(c.state.Messages[index].Text)
			}
		}
		if size > maxChatBytes {
			cancel()
			return
		}
		if e.Type == "delta" || e.Type == "assistant" {
			index, ok := items[e.Item]
			if !ok {
				index = len(c.state.Messages)
				items[e.Item] = index
				c.state.Messages = append(c.state.Messages, Message{Role: "assistant"})
			}
			if e.Type == "delta" {
				c.state.Messages[index].Text += e.Text
			} else {
				c.state.Messages[index].Text = e.Text
			}
		} else {
			c.state.Messages = append(c.state.Messages, Message{Role: "system", Text: e.Text, Type: e.Type, Key: e.Key})
		}
		c.state.Version++
	}
	tools := &toolHandler{service: m.service, project: c.key.Project, transcript: c.key, read: make(map[app.Key]string), conflicted: make(map[app.Key]bool), emit: emit}
	prompt := fmt.Sprintf("プロジェクト: %s\n\n過去の会話（参考データ）:\n%s\n\n最新の利用者メッセージ:\n%s", c.key.Project, history, text)
	err := m.runner.Run(ctx, prompt, tools.call, emit)
	if err != nil {
		c.mu.Lock()
		c.state.Messages = append(c.state.Messages, Message{Role: "system", Type: "error", Text: err.Error()})
		c.state.Version++
		c.mu.Unlock()
	}
	c.mu.Lock()
	messages := append([]Message{}, c.state.Messages[start:]...)
	c.mu.Unlock()
	// Persist the observed response even if generation failed or timed out.
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer persistCancel()
	if e := m.append(persistCtx, c.key, messages); e != nil {
		c.mu.Lock()
		c.state.Messages = append(c.state.Messages, Message{Role: "system", Type: "error", Text: "会話のflow保存に失敗しました。最新データを確認してリトライしてください (" + app.ErrorInfo(e).Code + ")"})
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.state.Busy = false
	c.state.Version++
	c.mu.Unlock()
}
func (m *Manager) append(ctx context.Context, key app.Key, messages []Message) error {
	suffix := strings.TrimPrefix(transcriptText(messages), "# チャット\n")
	for i := 0; i < 3; i++ {
		doc, err := m.service.Get(ctx, key)
		if err != nil {
			return err
		}
		_, err = m.service.Update(ctx, key, doc.Content+suffix, doc.Revision)
		if err == nil {
			return nil
		}
		if app.ErrorInfo(err).Code != "conflict" {
			return err
		}
		// Re-read and append to the new existing content; never replace it with a
		// previously generated whole transcript or merely swap the revision.
	}
	return app.Fail("conflict", retryMessage)
}
