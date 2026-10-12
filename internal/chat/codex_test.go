package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/s4na/fragment/internal/app"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRPCServerHelper(t *testing.T) {
	if os.Getenv("FRAGMENT_RPC_HELPER") == "" {
		return
	}
	scan := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	send := func(v any) {
		if e := enc.Encode(v); e != nil {
			os.Exit(2)
		}
	}
	for scan.Scan() {
		var msg rpcMessage
		if json.Unmarshal(scan.Bytes(), &msg) != nil {
			os.Exit(3)
		}
		switch msg.Method {
		case "initialize":
			send(map[string]any{"id": msg.ID, "result": map[string]any{}})
		case "config/read":
			send(map[string]any{"id": msg.ID, "result": map[string]any{"config": map[string]any{"model": "test-model", "mcp_servers": map[string]any{"other": map[string]any{}}}}})
		case "model/list":
			send(map[string]any{"id": msg.ID, "result": map[string]any{"data": []map[string]any{{"model": "test-model", "isDefault": true}}}})
		case "thread/start":
			var p map[string]any
			_ = json.Unmarshal(msg.Params, &p)
			config := p["config"].(map[string]any)
			if p["sandbox"] != "read-only" || p["approvalPolicy"] != "never" || len(p["dynamicTools"].([]any)) != 4 || config["mcp_servers.other.enabled"] != false || config["features.shell_tool"] != false {
				os.Exit(4)
			}
			send(map[string]any{"id": msg.ID, "result": map[string]any{"thread": map[string]string{"id": "thread"}}})
		case "turn/start":
			if os.Getenv("FRAGMENT_RPC_HELPER") == "failed" {
				send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]any{"id": "turn", "status": "failed", "error": map[string]string{"message": "usage limit reached"}}}})
				send(map[string]any{"id": msg.ID, "result": map[string]any{"turn": map[string]string{"id": "turn"}}})
			} else {
				send(map[string]any{"id": msg.ID, "result": map[string]any{"turn": map[string]string{"id": "turn"}}})
				send(map[string]any{"id": "server-request", "method": "item/tool/call", "params": map[string]any{"threadId": "thread", "turnId": "turn", "callId": "tool", "tool": "fragment_create_document", "arguments": map[string]string{"kind": "stock", "path": "RPC.md", "content": "# RPC保存"}}})
			}
		case "":
			if string(msg.ID) == `"server-request"` {
				var v struct{ Success bool }
				_ = json.Unmarshal(msg.Result, &v)
				if !v.Success {
					os.Exit(5)
				}
				send(map[string]any{"method": "item/agentMessage/delta", "params": map[string]string{"threadId": "thread", "turnId": "turn", "itemId": "answer", "delta": "保存"}})
				send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "thread", "turnId": "turn", "item": map[string]string{"id": "answer", "type": "agentMessage", "text": "保存しました。"}}})
				send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]string{"id": "turn", "status": "completed"}}})
			}
		}
	}
	os.Exit(0)
}
func helperExecutable(t *testing.T) string {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(t.TempDir(), "codex")
	quoted := "'" + strings.ReplaceAll(exe, "'", "'\"'\"'") + "'"
	if e = os.WriteFile(file, []byte("#!/bin/sh\nexec "+quoted+" -test.run=TestRPCServerHelper\n"), 0700); e != nil {
		t.Fatal(e)
	}
	return file
}
func TestCodexRPCWritesThroughTools(t *testing.T) {
	t.Setenv("FRAGMENT_RPC_HELPER", "completed")
	s := testService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var events []Event
	h := &toolHandler{service: s, project: "p", read: map[app.Key]string{}, conflicted: map[app.Key]bool{}, emit: func(e Event) { events = append(events, e) }}
	if e := (Codex{Executable: helperExecutable(t)}).Run(ctx, "save", h.call, func(e Event) { events = append(events, e) }); e != nil {
		t.Fatal(e)
	}
	doc, e := s.Get(ctx, app.Key{Project: "p", Kind: "stock", Path: "RPC.md"})
	if e != nil || doc.Content != "# RPC保存" {
		t.Fatal(doc, e)
	}
	if len(events) != 3 || events[0].Type != "updated" || events[1].Type != "delta" || events[2].Text != "保存しました。" {
		t.Fatal(events)
	}
}
func TestCodexFailedTurnIsNotSuccess(t *testing.T) {
	t.Setenv("FRAGMENT_RPC_HELPER", "failed")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e := (Codex{Executable: helperExecutable(t)}).Run(ctx, "save", func(context.Context, string, json.RawMessage) (any, bool) { return nil, true }, func(Event) {})
	if e == nil || !strings.Contains(e.Error(), "usage limit reached") || !strings.Contains(e.Error(), "リトライ") {
		t.Fatal(fmt.Sprint(e))
	}
}
