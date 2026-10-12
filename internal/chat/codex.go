package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

// Runner starts a real app-server turn. A fresh ephemeral thread receives the
// conversation history, while all storage tools run in the Go process.
type Runner interface {
	Run(context.Context, string, func(context.Context, string, json.RawMessage) (any, bool), func(Event)) error
}

// DefaultExecutable prefers the CLI shipped with the macOS desktop app so
// the model catalog and protocol match that app. Other hosts use PATH.
func DefaultExecutable() string {
	if runtime.GOOS == "darwin" {
		for _, path := range []string{"/Applications/Codex.app/Contents/Resources/codex", "/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex"} {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
				return path
			}
		}
	}
	return "codex"
}

type Codex struct {
	Executable string
	Model      string
}
type rpcMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}
type rpc struct {
	ctx          context.Context
	out          *json.Encoder
	messages     <-chan rpcMessage
	next         int
	tool         func(context.Context, string, json.RawMessage) (any, bool)
	emit         func(Event)
	thread, turn string
	finished     bool
	status       string
	failure      string
}

func (c Codex) Run(ctx context.Context, prompt string, tool func(context.Context, string, json.RawMessage) (any, bool), emit func(Event)) error {
	executable := c.Executable
	if executable == "" {
		executable = DefaultExecutable()
	}
	cwd, err := os.MkdirTemp("", "fragment-codex-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cwd)
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(childCtx, executable, "app-server", "--listen", "stdio://")
	cmd.Dir = cwd
	cmd.Stderr = io.Discard // Never forward local config, credentials, or protocol logs to the browser.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("Codex app-serverを起動できません。codexをインストールし、codex loginでログインしてください")
	}
	messages := make(chan rpcMessage, 32)
	scannerDone := make(chan struct{})
	go func() {
		defer close(scannerDone)
		defer close(messages)
		scan := bufio.NewScanner(stdout)
		scan.Buffer(make([]byte, 64<<10), 6*8<<20)
		for scan.Scan() {
			var msg rpcMessage
			if json.Unmarshal(scan.Bytes(), &msg) != nil {
				continue
			}
			select {
			case messages <- msg:
			case <-childCtx.Done():
				return
			}
		}
	}()
	defer func() { cancel(); _ = stdin.Close(); _ = cmd.Wait(); <-scannerDone }()
	r := &rpc{ctx: ctx, out: json.NewEncoder(stdin), messages: messages, tool: tool, emit: emit}
	if _, err = r.request("initialize", map[string]any{"clientInfo": map[string]string{"name": "fragment", "title": "fragment", "version": "0.1.0"}, "capabilities": map[string]any{"experimentalApi": true}}); err != nil {
		return err
	}
	if err = r.out.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return err
	}
	// Keep the user's authentication/model settings, but disable unrelated MCP
	// servers and connectors for this document-only thread. No config is saved.
	configResult, err := r.request("config/read", map[string]any{"includeLayers": false})
	if err != nil {
		return err
	}
	var conf struct {
		Config struct {
			Model   string                     `json:"model"`
			Servers map[string]json.RawMessage `json:"mcp_servers"`
		} `json:"config"`
	}
	if err = json.Unmarshal(configResult, &conf); err != nil {
		return err
	}
	modelsResult, err := r.request("model/list", map[string]any{})
	if err != nil {
		return err
	}
	var models struct {
		Data []struct {
			Model   string `json:"model"`
			Default bool   `json:"isDefault"`
		} `json:"data"`
	}
	if err = json.Unmarshal(modelsResult, &models); err != nil {
		return err
	}
	model := c.Model
	if model == "" {
		model = conf.Config.Model
		found := false
		for _, entry := range models.Data {
			found = found || entry.Model == model
		}
		if !found {
			model = ""
			for _, entry := range models.Data {
				if entry.Default {
					model = entry.Model
					break
				}
			}
			if model == "" {
				return fmt.Errorf("Codexで利用できるモデルを確認できません。serve --modelで指定してください")
			}
			emit(Event{Type: "status", Text: "設定モデルがCodexの一覧にないため、標準モデル " + model + " を使用します。"})
		}
	}
	config := map[string]any{"features.shell_tool": false, "features.apply_patch_freeform": false, "features.code_mode": false, "features.multi_agent": false, "features.apps": false, "features.plugins": false, "web_search": "disabled"}
	for name := range conf.Config.Servers {
		config["mcp_servers."+name+".enabled"] = false
	}
	result, err := r.request("thread/start", map[string]any{"cwd": cwd, "approvalPolicy": "never", "sandbox": "read-only", "ephemeral": true, "model": model, "baseInstructions": instructions, "developerInstructions": instructions, "dynamicTools": toolSpecs(), "config": config})
	if err != nil {
		return err
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = json.Unmarshal(result, &thread); err != nil || thread.Thread.ID == "" {
		return fmt.Errorf("Codexのスレッドを開始できません")
	}
	r.thread = thread.Thread.ID
	result, err = r.request("turn/start", map[string]any{"threadId": r.thread, "input": []map[string]string{{"type": "text", "text": prompt}}})
	if err != nil {
		return err
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err = json.Unmarshal(result, &turn); err != nil || turn.Turn.ID == "" {
		return fmt.Errorf("Codexの応答を開始できません")
	}
	r.turn = turn.Turn.ID
	for !r.finished {
		msg, e := r.receive()
		if e != nil {
			return e
		}
		if e = r.handle(msg); e != nil {
			return e
		}
	}
	if r.status != "completed" {
		return fmt.Errorf("Codexの応答が完了しませんでした (%s)。既に保存された文書を確認してからリトライしてください", r.status+": "+r.failure)
	}
	return nil
}
func (r *rpc) receive() (rpcMessage, error) {
	select {
	case <-r.ctx.Done():
		return rpcMessage{}, fmt.Errorf("Codexの応答を中断しました。最新データを確認してからリトライしてください")
	case msg, ok := <-r.messages:
		if !ok {
			return msg, fmt.Errorf("Codex app-serverとの接続が切れました。最新データを確認してからリトライしてください")
		}
		return msg, nil
	}
}
func (r *rpc) request(method string, params any) (json.RawMessage, error) {
	r.next++
	id := r.next
	if err := r.out.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		msg, err := r.receive()
		if err != nil {
			return nil, err
		}
		if msg.Method == "" && string(msg.ID) == fmt.Sprint(id) {
			if msg.Error != nil {
				return nil, fmt.Errorf("Codex app-server: %s", msg.Error.Message)
			}
			return msg.Result, nil
		}
		if err = r.handle(msg); err != nil {
			return nil, err
		}
	}
}
func (r *rpc) handle(msg rpcMessage) error {
	if msg.Method == "" {
		return nil
	}
	var p struct {
		ThreadID  string                          `json:"threadId"`
		TurnID    string                          `json:"turnId"`
		ItemID    string                          `json:"itemId"`
		Delta     string                          `json:"delta"`
		Tool      string                          `json:"tool"`
		Arguments json.RawMessage                 `json:"arguments"`
		Item      struct{ ID, Type, Text string } `json:"item"`
		Turn      struct {
			ID, Status string
			Error      *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		return err
	}
	if len(msg.ID) > 0 {
		if msg.Method != "item/tool/call" {
			return r.out.Encode(map[string]any{"id": msg.ID, "error": map[string]any{"code": -32601, "message": "fragment supports only document tools"}})
		}
		if p.ThreadID != r.thread || (r.turn != "" && p.TurnID != r.turn) {
			return fmt.Errorf("Codexツールの会話が一致しません")
		}
		value, success := r.tool(r.ctx, p.Tool, p.Arguments)
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return r.out.Encode(map[string]any{"id": msg.ID, "result": map[string]any{"success": success, "contentItems": []map[string]string{{"type": "inputText", "text": string(b)}}}})
	}
	if r.thread == "" || p.ThreadID != r.thread {
		return nil
	}
	switch msg.Method {
	case "item/agentMessage/delta":
		r.emit(Event{Type: "delta", Text: p.Delta, Item: p.ItemID})
	case "item/completed":
		if p.Item.Type == "agentMessage" {
			r.emit(Event{Type: "assistant", Text: p.Item.Text, Item: p.Item.ID})
		}
	case "turn/completed":
		if r.turn == "" || p.Turn.ID == r.turn {
			r.finished = true
			r.status = p.Turn.Status
			if p.Turn.Error != nil {
				r.failure = p.Turn.Error.Message
			}
		}
	}
	return nil
}
