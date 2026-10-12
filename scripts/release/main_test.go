package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLatestVersion(t *testing.T) {
	for _, test := range []struct{ tags, want string }{
		{"", "v0.0.0"},
		{"v0.0.9\nv0.0.10\nv0.0.2\n", "v0.0.10"},
		{"v1.2.9\nv2.0.0\nv1.10.4\n", "v2.0.0"},
		{"v1.2.3\nv1.2.4-rc1\nunrelated\nv01.2.9\nv999999999999999999999.0.0\n", "v1.2.3"},
	} {
		got, err := latestVersion(strings.NewReader(test.tags))
		if err != nil || got.String() != test.want {
			t.Fatalf("tags %q: %s, %v; want %s", test.tags, got, err, test.want)
		}
	}
}

func TestArchiveAndManifest(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "input")
	if err := os.WriteFile(binary, []byte("executable contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if err := archive(binary, filepath.Join(dir, archiveName("v0.0.1", target))); err != nil {
			t.Fatal(err)
		}
	}
	if err := manifest("v0.0.1", dir); err != nil {
		t.Fatal(err)
	}
	formula, err := os.ReadFile(filepath.Join(dir, "fragment.rb"))
	if err != nil {
		t.Fatal(err)
	}
	checksums, err := os.ReadFile(filepath.Join(dir, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		name := archiveName("v0.0.1", target)
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		sha := fmt.Sprintf("%x", sha256.Sum256(data))
		if !strings.Contains(string(formula), name) || !strings.Contains(string(formula), sha) || !strings.Contains(string(checksums), sha+"  "+name) {
			t.Fatal("formula or checksum does not describe the actual archive")
		}
	}
	file, err := os.Open(filepath.Join(dir, archiveName("v0.0.1", targets[0])))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	header, err := reader.Next()
	if err != nil || header.Name != "fragment" || header.Mode != 0o755 {
		t.Fatalf("bad archive header: %v, %v", header, err)
	}
	contents, err := io.ReadAll(reader)
	if err != nil || string(contents) != "executable contents" {
		t.Fatalf("bad archived contents: %q, %v", contents, err)
	}
	if _, err := reader.Next(); err != io.EOF {
		t.Fatalf("unexpected extra entry: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, archiveName("v0.0.1", targets[0]))); err != nil {
		t.Fatal(err)
	}
	if err := manifest("v0.0.1", dir); err == nil {
		t.Fatal("missing platform archive must fail")
	}
}
