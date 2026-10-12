package storage

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s4na/fragment/internal/app"
)

func setup(t *testing.T) (*Store, *app.Service, app.Key) {
	t.Helper()
	s, err := Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := &app.Service{Repo: s}
	if err = svc.CreateProject(context.Background(), "日本語 project"); err != nil {
		t.Fatal(err)
	}
	return s, svc, app.Key{Project: "日本語 project", Kind: "stock", Path: "folder/文書.md"}
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || app.ErrorInfo(err).Code != want {
		t.Fatalf("error=%v, want code %s", err, want)
	}
}
func TestSharedFilesAndConflicts(t *testing.T) {
	s, svc, k := setup(t)
	ctx := context.Background()
	body := "自由な本文\r\n"
	doc, err := svc.Create(ctx, k, body)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Open(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	svc2 := &app.Service{Repo: other}
	read, err := svc2.Get(ctx, k)
	if err != nil || read.Content != body || read.Revision != doc.Revision {
		t.Fatalf("read=%+v err=%v", read, err)
	}
	_, err = svc.Create(ctx, k, "上書き")
	code(t, err, "already_exists")
	next, err := svc2.Update(ctx, k, "更新", read.Revision)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Update(ctx, k, "古い変更", doc.Revision)
	code(t, err, "conflict")
	content, err := os.ReadFile(filepath.Join(s.Root, k.Project, k.Kind, filepath.FromSlash(k.Path)))
	if err != nil || string(content) != "更新" {
		t.Fatalf("file=%s err=%v", content, err)
	}
	if next.Revision == doc.Revision {
		t.Fatal("revision did not change")
	}
	k.Kind = "flow"
	k.Path = "empty.md"
	if _, err = svc.Create(ctx, k, ""); err != nil {
		t.Fatal(err)
	}
	if err = svc.CreateProject(ctx, k.Project); err == nil {
		t.Fatal("duplicate project accepted")
	}
}
func TestBoundaryAndPagination(t *testing.T) {
	s, svc, k := setup(t)
	ctx := context.Background()
	for _, p := range []string{"../outside.md", "/absolute.md", "a/../../outside.md", "a/.fragment/private.md", ".fragment-tmp-x.md"} {
		bad := k
		bad.Path = p
		_, err := svc.Create(ctx, bad, "x")
		code(t, err, "invalid_input")
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(s.Root, k.Project, "stock", "escape")); err != nil {
		t.Fatal(err)
	}
	bad := k
	bad.Path = "escape/evil.md"
	_, err := svc.Create(ctx, bad, "x")
	code(t, err, "invalid_input")
	if err = os.Remove(filepath.Join(s.Root, k.Project, "stock", "escape")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a.md", "b.md", "c.md"} {
		key := k
		key.Path = p
		if _, err = svc.Create(ctx, key, "x"); err != nil {
			t.Fatal(err)
		}
	}
	page, err := svc.Documents(ctx, k.Project, k.Kind, "", 2)
	if err != nil || strings.Join(page.Paths, ",") != "a.md,b.md" || page.Next != "b.md" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	page, err = svc.Documents(ctx, k.Project, k.Kind, page.Next, 2)
	if err != nil || strings.Join(page.Paths, ",") != "c.md" || page.Next != "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}
func TestSaveFailureContract(t *testing.T) {
	t.Run("before publication", func(t *testing.T) {
		s, svc, k := setup(t)
		doc, err := svc.Create(context.Background(), k, "old")
		if err != nil {
			t.Fatal(err)
		}
		s.renameFn = func(string, string) error { return errors.New("injected rename failure") }
		_, err = svc.Update(context.Background(), k, "new", doc.Revision)
		code(t, err, "io_error")
		got, err := svc.Get(context.Background(), k)
		if err != nil || got.Content != "old" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		entries, _ := os.ReadDir(filepath.Join(s.Root, k.Project, k.Kind, "folder"))
		if len(entries) != 1 {
			t.Fatalf("temporary files leaked: %v", entries)
		}
	})
	t.Run("after publication", func(t *testing.T) {
		s, svc, k := setup(t)
		doc, err := svc.Create(context.Background(), k, "old")
		if err != nil {
			t.Fatal(err)
		}
		s.syncFn = func(string) error { return errors.New("injected directory sync failure") }
		_, err = svc.Update(context.Background(), k, "new", doc.Revision)
		code(t, err, "commit_unknown")
		got, err := svc.Get(context.Background(), k)
		if err != nil || got.Content != "new" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	})
	t.Run("project publication", func(t *testing.T) {
		s, _, _ := setup(t)
		s.renameFn = func(string, string) error { return errors.New("injected publication failure") }
		err := s.CreateProject(context.Background(), "incomplete")
		code(t, err, "io_error")
		if _, err = os.Stat(filepath.Join(s.Root, "incomplete")); !os.IsNotExist(err) {
			t.Fatal("partial project published")
		}
	})
}

// Separate OS processes exercise flock and abrupt termination, not a Go mutex.
func TestProcessWorker(t *testing.T) {
	action := os.Getenv("FRAGMENT_TEST_ACTION")
	if action == "" {
		t.Skip("subprocess helper")
	}
	s, err := Open(os.Getenv("FRAGMENT_TEST_ROOT"))
	if err != nil {
		panic(err)
	}
	if action == "hold" {
		unlock, err := s.lock(context.Background())
		if err != nil {
			panic(err)
		}
		defer unlock()
		fmt.Println("HELD")
		time.Sleep(10 * time.Minute)
		os.Exit(0)
	}
	svc := &app.Service{Repo: s}
	_, err = svc.Update(context.Background(), app.Key{Project: "日本語 project", Kind: "stock", Path: "folder/文書.md"}, action, os.Getenv("FRAGMENT_TEST_REVISION"))
	if err == nil {
		fmt.Println("RESULT:ok")
	} else {
		fmt.Println("RESULT:" + app.ErrorInfo(err).Code)
	}
	os.Exit(0)
}
func worker(s *Store, action, revision string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessWorker$")
	cmd.Env = append(os.Environ(), "FRAGMENT_TEST_ACTION="+action, "FRAGMENT_TEST_ROOT="+s.Root, "FRAGMENT_TEST_REVISION="+revision)
	return cmd
}
func TestTwoProcessesAndCrashRelease(t *testing.T) {
	s, svc, k := setup(t)
	doc, err := svc.Create(context.Background(), k, "initial")
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	one := worker(s, "one", doc.Revision)
	two := worker(s, "two", doc.Revision)
	one.Stdout = &a
	two.Stdout = &b
	one.Stderr = &a
	two.Stderr = &b
	if err = one.Start(); err != nil {
		t.Fatal(err)
	}
	if err = two.Start(); err != nil {
		t.Fatal(err)
	}
	if err = one.Wait(); err != nil {
		t.Fatal(err, a.String())
	}
	if err = two.Wait(); err != nil {
		t.Fatal(err, b.String())
	}
	combined := a.String() + b.String()
	if strings.Count(combined, "RESULT:ok") != 1 || strings.Count(combined, "RESULT:conflict") != 1 {
		t.Fatal(combined)
	}
	lockPath := filepath.Join(s.Root, ".fragment/write.lock")
	before, _ := os.Stat(lockPath)
	held := worker(s, "hold", "")
	stdout, err := held.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = held.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Process.Kill(); _ = held.Wait() }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "HELD\n" {
		t.Fatalf("lock not acquired: %s %v", line, err)
	}
	s.LockTimeout = 100 * time.Millisecond
	_, err = svc.Create(context.Background(), app.Key{Project: k.Project, Kind: "flow", Path: "after.md"}, "x")
	code(t, err, "lock_timeout")
	if err = held.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = held.Wait()
	s.LockTimeout = time.Second
	if _, err = svc.Create(context.Background(), app.Key{Project: k.Project, Kind: "flow", Path: "after.md"}, "x"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(lockPath)
	if !os.SameFile(before, after) {
		t.Fatal("lock file was replaced")
	}
}
