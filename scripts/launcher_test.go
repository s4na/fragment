package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceHashUsesImplementationContent(t *testing.T) {
	root := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	hash := func() string {
		t.Helper()
		value, err := sourceHash(root)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	write("internal/app/app.go", "package app\n")
	write("web/src/main.tsx", "export const value = 1;\n")
	initial := hash()
	for _, path := range []string{"README.md", "docs/design-doc.md", "internal/app/app_test.go", "web/src/api.test.ts", "web/src/__tests__/fixtures.json", "bin/fragment", "web/dist/app.js", "web/node_modules/example/index.ts"} {
		write(path, "irrelevant change")
		if hash() != initial {
			t.Fatalf("non-build input changed fingerprint: %s", path)
		}
	}
	// The filesystem is authoritative, regardless of whether changes are staged.
	for _, path := range []string{"internal/app/app.go", "internal/app/new.go", "web/src/style.css", "web/.env.local", "web/package.json", "go.sum"} {
		before := hash()
		write(path, "implementation change")
		if hash() == before {
			t.Fatalf("implementation change was missed: %s", path)
		}
	}
	before := hash()
	if err := os.Remove(filepath.Join(root, "internal/app/new.go")); err != nil {
		t.Fatal(err)
	}
	if hash() == before {
		t.Fatal("implementation removal was missed")
	}
	asset := "web/public/logo.svg"
	before = hash()
	write(asset, "<svg>first</svg>")
	if hash() == before {
		t.Fatal("public asset addition was missed")
	}
	before = hash()
	write(asset, "<svg>second</svg>")
	if hash() == before {
		t.Fatal("public asset update was missed")
	}
	before = hash()
	if err := os.Remove(filepath.Join(root, asset)); err != nil {
		t.Fatal(err)
	}
	if hash() == before {
		t.Fatal("public asset removal was missed")
	}
}

func TestReuseRequiresMatchingCommitSourceToolsAndBinary(t *testing.T) {
	root := t.TempDir()
	binary, record := filepath.Join(root, "fragment"), filepath.Join(root, "build.json")
	current := inputs{"commit", "source", "tools"}
	if reusable(binary, record, current) {
		t.Fatal("missing build reused")
	}
	if err := os.WriteFile(binary, []byte("executable"), 0700); err != nil {
		t.Fatal(err)
	}
	digest, err := fileHash(binary)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(buildRecord{current, digest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, b, 0600); err != nil {
		t.Fatal(err)
	}
	if !reusable(binary, record, current) {
		t.Fatal("matching build not reused")
	}
	for _, changed := range []inputs{{"new", "source", "tools"}, {"commit", "new", "tools"}, {"commit", "source", "new"}} {
		if reusable(binary, record, changed) {
			t.Fatal("stale build reused", changed)
		}
	}
	if err := os.WriteFile(binary, []byte("replaced executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if reusable(binary, record, current) {
		t.Fatal("replaced binary reused")
	}
}
