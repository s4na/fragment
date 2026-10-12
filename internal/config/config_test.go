package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4na/fragment/internal/app"
)

func TestDefaultStoreWithoutConfiguration(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config", "config.json")
	c, e := load(path, func() (string, error) { return home, nil }, true)
	want := filepath.Join(home, "Documents", "fragment")
	if e != nil || c.DataRoot != want {
		t.Fatal(c, e)
	}
	if _, e = os.Stat(want); !os.IsNotExist(e) {
		t.Fatal("loading config created data", e)
	}
	one, e := openStore(c)
	if e != nil {
		t.Fatal(e)
	}
	service := &app.Service{Repo: one}
	ctx := context.Background()
	if e = service.CreateProject(ctx, "p"); e != nil {
		t.Fatal(e)
	}
	key := app.Key{Project: "p", Kind: "stock", Path: "概要.md"}
	if _, e = service.Create(ctx, key, "# 共通の実ファイル"); e != nil {
		t.Fatal(e)
	}
	before, e := os.Stat(filepath.Join(want, ".fragment/write.lock"))
	if e != nil {
		t.Fatal(e)
	}
	two, e := openStore(c)
	if e != nil {
		t.Fatal(e)
	}
	after, e := os.Stat(filepath.Join(want, ".fragment/write.lock"))
	if e != nil || !os.SameFile(before, after) {
		t.Fatal("initialization replaced lock file", e)
	}
	doc, e := (&app.Service{Repo: two}).Get(ctx, key)
	if e != nil || doc.Content != "# 共通の実ファイル" {
		t.Fatal(doc, e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("default persisted a needless config file", e)
	}
}
func TestExplicitConfigurationDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	fallback := func() (string, error) { t.Fatal("explicit config requested home/default"); return "", nil }
	root := filepath.Join(dir, "custom")
	b, _ := json.Marshal(Config{DataRoot: root})
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	c, e := load(path, fallback, true)
	if e != nil || c.DataRoot != root {
		t.Fatal(c, e)
	}
	if _, e = openStore(c); e == nil {
		t.Fatal("explicit missing root was silently initialized")
	}
	if e = os.WriteFile(path, []byte("invalid"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = load(path, fallback, true); app.ErrorInfo(e).Code != "config_error" {
		t.Fatal(e)
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if _, e = load(path, fallback, false); app.ErrorInfo(e).Code != "config_error" {
		t.Fatal("isolated config switched to normal data", e)
	}
}
