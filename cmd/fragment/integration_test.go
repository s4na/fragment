package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/s4na/fragment/internal/app"
)

func TestCLIAndMCPShareFiles(t *testing.T) {
	temp := t.TempDir()
	binary := filepath.Join(temp, "fragment")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	env := append(os.Environ(), "FRAGMENT_CONFIG_DIR="+filepath.Join(temp, "config"))
	root := filepath.Join(temp, "documents")
	run := func(cwd, input string, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = env
		cmd.Dir = cwd
		cmd.Stdin = strings.NewReader(input)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
		return out
	}
	run(temp, "", "config", "init", root)
	run(temp, "", "project", "create", "p")
	output := run(temp, "# CLI\n", "doc", "create", "--project", "p", "--kind", "stock", "--path", "a.md")
	var original app.Document
	if err := json.Unmarshal(output, &original); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "fragment-test", Version: "1.0"}, nil)
	cmd := exec.CommandContext(ctx, binary, "mcp")
	cmd.Env = env
	cmd.Dir = temp
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil || len(tools.Tools) != 6 {
		t.Fatalf("tools=%+v err=%v", tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "update_document", Arguments: map[string]any{"project": "p", "kind": "stock", "path": "a.md", "content": "# MCP\n自由な内容", "revision": original.Revision}})
	if err != nil || result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	elsewhere := filepath.Join(temp, "other-worktree")
	if err = os.Mkdir(elsewhere, 0700); err != nil {
		t.Fatal(err)
	}
	read := run(elsewhere, "", "doc", "get", "--project", "p", "--kind", "stock", "--path", "a.md")
	var updated app.Document
	if err = json.Unmarshal(read, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Content != "# MCP\n自由な内容" {
		t.Fatal(updated)
	}
	actual, err := os.ReadFile(filepath.Join(root, "p/stock/a.md"))
	if err != nil || !bytes.Equal(actual, []byte(updated.Content)) {
		t.Fatalf("actual=%s err=%v", actual, err)
	}
	conflict, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "update_document", Arguments: map[string]any{"project": "p", "kind": "stock", "path": "a.md", "content": "stale", "revision": original.Revision}})
	if err != nil || !conflict.IsError {
		t.Fatalf("stale result=%+v err=%v", conflict, err)
	}
}
