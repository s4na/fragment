//go:build darwin || linux

package desktop

import (
	"os"
	"path/filepath"
	"testing"
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
