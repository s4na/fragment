//go:build darwin || linux

package desktop

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrivateRuntimeDirectories(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := privateDir(real); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(real)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatal("runtime directory must be private", err)
	}
	link := filepath.Join(base, "link")
	if err = os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err = privateDir(link); err == nil {
		t.Fatal("symlink runtime directory accepted")
	}
	file := filepath.Join(base, "file")
	if err = os.WriteFile(file, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = privateDir(file); err == nil {
		t.Fatal("file accepted as runtime directory")
	}
	fileLink := filepath.Join(base, "file-link")
	if err = os.Symlink(file, fileLink); err != nil {
		t.Fatal(err)
	}
	if f, err := privateFile(fileLink, os.O_CREATE|os.O_WRONLY|os.O_TRUNC); err == nil {
		f.Close()
		t.Fatal("symlink log/lock accepted")
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "original" {
		t.Fatal("symlink target modified", err)
	}
}

func TestCanonicalConfigPath(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{link + "/missing/config.json", base + "/real/../real/missing/config.json"} {
		got, err := canonicalConfigPath(alias)
		if err != nil || got != filepath.Join(real, "missing/config.json") {
			t.Fatalf("absent config through alias: %q %v", got, err)
		}
	}
	file := filepath.Join(real, "config.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	fileLink := filepath.Join(base, "config.json")
	if err := os.Symlink(file, fileLink); err != nil {
		t.Fatal(err)
	}
	got, err := canonicalConfigPath(fileLink)
	if err != nil || got != fileLink {
		t.Fatalf("config file identity must use its directory: %q %v", got, err)
	}
	replacement := filepath.Join(base, "replacement")
	if err := os.WriteFile(replacement, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, fileLink); err != nil {
		t.Fatal(err)
	}
	after, err := canonicalConfigPath(fileLink)
	if err != nil || after != got {
		t.Fatalf("config rewrite changed daemon identity: %q %v", after, err)
	}
}

func TestOpenControlPreservesCallerDeadline(t *testing.T) {
	// Keep the Unix socket below macOS's path length limit.
	dir, err := os.MkdirTemp("/tmp", "fragment-call-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	p := paths{socket: filepath.Join(dir, "control.sock")}
	listener, err := net.Listen("unix", p.socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2100 * time.Millisecond):
		}
		_ = json.NewEncoder(w).Encode(response{Info: Info{Running: true, PID: 123}})
	})}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if info, err := call(ctx, p, "open", "project"); err != nil || info.PID != 123 {
		t.Fatalf("open truncated caller deadline: %+v %v", info, err)
	}
	short, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	if _, err := call(short, p, "open", "project"); err == nil {
		t.Fatal("open ignored shorter caller deadline")
	}
}
