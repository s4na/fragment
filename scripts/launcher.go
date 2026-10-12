// The launcher is a standalone standard-library program so the shell entry
// point can bootstrap it from any working directory without invoking make.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

type inputs struct {
	Commit string
	Source string
	Tools  string
}

type buildRecord struct {
	Inputs inputs
	Binary string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "fragment: 起動スクリプトから実行してください")
		os.Exit(1)
	}
	path, err := ensureBuild(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "fragment:", err)
		os.Exit(1)
	}
	fmt.Println(path)
}

func command(root, name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Dir = root
	// This checkout is a standalone Go module, independent of the caller's go.work.
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GOWORK=") {
			c.Env = append(c.Env, value)
		}
	}
	c.Env = append(c.Env, "GOWORK=off")
	return c
}

func output(root, name string, args ...string) ([]byte, error) {
	c := command(root, name, args...)
	c.Stderr = os.Stderr
	b, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("%s の実行に失敗しました: %w", name, err)
	}
	return b, nil
}

func run(root, name string, args ...string) error {
	c := command(root, name, args...)
	// MCP and CLI JSON must be the only output on stdout.
	c.Stdout, c.Stderr = os.Stderr, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s のビルドに失敗しました: %w", name, err)
	}
	return nil
}

func relevant(path string) bool {
	// Vite copies all public files, including names which resemble test fixtures.
	if strings.HasPrefix(path, "web/public/") {
		return true
	}
	if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/__tests__/") {
		return false
	}
	if strings.HasPrefix(path, "web/src/") {
		name := filepath.Base(path)
		return !strings.Contains(name, ".test.") && !strings.Contains(name, ".spec.")
	}
	switch path {
	case "go.mod", "go.sum", "fragment", "web/package.json", "web/pnpm-lock.yaml", "web/tsconfig.json", "web/index.html":
		return true
	}
	if strings.HasPrefix(path, "web/.env") || strings.HasPrefix(path, "web/vite.config.") || strings.HasPrefix(path, "vendor/") {
		return true
	}
	switch filepath.Ext(path) {
	case ".go", ".c", ".h", ".s", ".S", ".syso":
		return true
	}
	// Conservatively include assets which future Go code may embed.
	return strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/")
}

func sourceHash(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			switch relative {
			case ".git", "bin", "web/node_modules", "web/dist", "web/.vitest":
				return filepath.SkipDir
			}
			return nil
		}
		if !relevant(relative) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%d:%s:%d:", len(relative), relative, len(content))
		h.Write(content)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

func snapshot(root string) (inputs, error) {
	commit, err := output(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return inputs{}, err
	}
	source, err := sourceHash(root)
	if err != nil {
		return inputs{}, fmt.Errorf("実装ファイルの確認に失敗しました: %w", err)
	}
	version, err := output(root, "go", "version")
	if err != nil {
		return inputs{}, err
	}
	config, err := output(root, "go", "env", "-json", "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOEXPERIMENT", "GOAMD64", "GOARM64", "GOTOOLCHAIN", "CC", "CXX", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_LDFLAGS")
	if err != nil {
		return inputs{}, err
	}
	var platform struct{ GOOS, GOARCH string }
	if err := json.Unmarshal(config, &platform); err != nil {
		return inputs{}, err
	}
	if platform.GOOS != runtime.GOOS || platform.GOARCH != runtime.GOARCH {
		return inputs{}, errors.New("CLI起動時のGOOS・GOARCHは、この端末のOS・CPUに合わせてください")
	}
	environment := []string{}
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "VITE_") || strings.HasPrefix(value, "NODE_ENV=") {
			environment = append(environment, value)
		}
	}
	sort.Strings(environment)
	h := sha256.New()
	h.Write(version)
	h.Write(config)
	for _, value := range environment {
		fmt.Fprintf(h, "%d:%s", len(value), value)
	}
	return inputs{strings.TrimSpace(string(commit)), source, hex.EncodeToString(h.Sum(nil))}, nil
}

func fileHash(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func reusable(binary, record string, current inputs) bool {
	info, err := os.Stat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return false
	}
	b, err := os.ReadFile(record)
	if err != nil {
		return false
	}
	var saved buildRecord
	if json.Unmarshal(b, &saved) != nil || saved.Inputs != current {
		return false
	}
	actual, err := fileHash(binary)
	return err == nil && actual == saved.Binary
}

func acquire(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Minute)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) || time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("ビルドのロックを取得できませんでした: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func ensureBuild(root string) (string, error) {
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		return "", err
	}
	lock, err := acquire(filepath.Join(bin, ".build.lock"))
	if err != nil {
		return "", err
	}
	defer lock.Close()
	binary, record := filepath.Join(bin, "fragment"), filepath.Join(bin, "fragment-build.json")
	for attempt := 0; attempt < 3; attempt++ {
		before, err := snapshot(root)
		if err != nil {
			return "", err
		}
		if reusable(binary, record, before) {
			return binary, nil
		}
		fmt.Fprintln(os.Stderr, "fragment: 現在の実装をビルドします（初回・コミット・実装変更・ビルド状態を確認）")
		if err := run(root, "sfw", "pnpm", "--dir", "web", "install", "--frozen-lockfile"); err != nil {
			return "", err
		}
		if err := run(root, "sfw", "pnpm", "--dir", "web", "build"); err != nil {
			return "", err
		}
		temporary, err := os.MkdirTemp(bin, ".build-")
		if err != nil {
			return "", err
		}
		candidate := filepath.Join(temporary, "fragment")
		flags := "-X main.buildCommit=" + before.Commit + " -X main.buildSource=" + before.Source
		// Stamp the actual checkout explicitly: VCS directory detection can find
		// a parent repository instead of a worktree's .git file.
		err = run(root, "go", "build", "-buildvcs=false", "-ldflags", flags, "-o", candidate, "./cmd/fragment")
		if err != nil {
			os.RemoveAll(temporary)
			return "", err
		}
		after, err := snapshot(root)
		if err != nil {
			os.RemoveAll(temporary)
			return "", err
		}
		if before != after {
			os.RemoveAll(temporary)
			fmt.Fprintln(os.Stderr, "fragment: ビルド中に実装が変更されたため、確認し直します")
			continue
		}
		digest, err := fileHash(candidate)
		if err == nil {
			var data []byte
			data, err = json.MarshalIndent(buildRecord{after, digest}, "", "  ")
			if err == nil {
				err = os.WriteFile(filepath.Join(temporary, "record.json"), append(bytes.TrimSpace(data), '\n'), 0600)
			}
		}
		if err == nil {
			err = os.Rename(candidate, binary)
		}
		if err == nil {
			err = os.Rename(filepath.Join(temporary, "record.json"), record)
		}
		os.RemoveAll(temporary)
		if err != nil {
			return "", err
		}
		return binary, nil
	}
	return "", errors.New("ビルド中の変更が続いています。編集が終わってからリトライしてください")
}
