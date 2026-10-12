//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/desktop"
)

func TestDesktopSharedStoreLifecycle(t *testing.T) {
	temp := t.TempDir()
	temp, err := filepath.EvalSymlinks(temp)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(temp, "fragment")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	env := append(os.Environ(), "FRAGMENT_CONFIG_DIR="+filepath.Join(temp, "config"))
	run := func(cwd string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = env
		cmd.Dir = cwd
		return cmd.CombinedOutput()
	}
	must := func(cwd string, args ...string) []byte {
		t.Helper()
		out, err := run(cwd, args...)
		if err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
		return out
	}
	root := filepath.Join(temp, "documents")
	project := "設計 + #?"
	must(temp, "config", "init", root)
	// Init replaces this file symlink later. The same daemon must remain
	// reachable so it can report the changed root and stop through an alias.
	configFile := filepath.Join(temp, "config", "config.json")
	configTarget := filepath.Join(temp, "config", "original.json")
	if err := os.Rename(configFile, configTarget); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(configTarget, configFile); err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	must(temp, "project", "create", project)
	elsewhere := filepath.Join(temp, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 12*time.Second)
		defer stop()
		cmd := exec.CommandContext(cleanup, binary, "stop")
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("cleanup stop: %v %s", err, out)
			return
		}
		// Remove only this test's runtime files, after its daemon has stopped.
		id := app.Revision([]byte(filepath.Join(temp, "config", "config.json")))[:16]
		if base, err := filepath.EvalSymlinks("/tmp"); err == nil {
			_ = os.RemoveAll(filepath.Join(base, "fragment-"+strconv.Itoa(os.Geteuid()), id))
		}
		if cache, err := os.UserCacheDir(); err == nil {
			_ = os.Remove(filepath.Join(cache, "fragment", "server-"+id+".log"))
		}
	})
	var group sync.WaitGroup
	infos := make(chan desktop.Info, 5)
	failures := make(chan error, 5)
	for i := 0; i < 5; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			out, err := run(elsewhere, "open", "--no-open")
			var info desktop.Info
			if err == nil {
				err = json.Unmarshal(out, &info)
			}
			if err != nil {
				failures <- fmt.Errorf("open: %w %s", err, out)
				return
			}
			infos <- info
		}()
	}
	group.Wait()
	close(infos)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var pid int
	for info := range infos {
		if !info.Running || info.Root != root || info.PID == 0 {
			t.Fatalf("wrong state: %+v", info)
		}
		if pid != 0 && pid != info.PID {
			t.Fatal("simultaneous opens started multiple servers")
		}
		pid = info.PID
	}
	configLink := filepath.Join(temp, "config-link")
	if err := os.Symlink(filepath.Join(temp, "config"), configLink); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{configLink, temp + "/config/../config"} {
		cmd := exec.CommandContext(ctx, binary, "open", project, "--no-open")
		cmd.Env = append(env, "FRAGMENT_CONFIG_DIR="+alias)
		cmd.Dir = elsewhere
		out, err := cmd.CombinedOutput()
		var aliased desktop.Info
		if err != nil || json.Unmarshal(out, &aliased) != nil || aliased.PID != pid {
			t.Fatalf("config alias must reuse daemon: %s %v %s", alias, err, out)
		}
	}
	var opened desktop.Info
	if err := json.Unmarshal(must(elsewhere, "open", project, "--no-open"), &opened); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(opened.URL)
	if err != nil || parsed.Query().Get("project") != project || parsed.Fragment != "" || opened.PID != pid {
		t.Fatalf("project opening: %+v %v", opened, err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	origin := parsed.Scheme + "://" + parsed.Host
	login := func(client *http.Client) int {
		t.Helper()
		response, err := client.Get(origin + "/api/session")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	if login(client) != 200 {
		t.Fatal("direct URL did not establish a session")
	}
	state := func() string {
		t.Helper()
		response, err := client.Get(origin + "/api/state")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var value struct {
			Root string `json:"data_root"`
		}
		if err = json.NewDecoder(response.Body).Decode(&value); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 {
			t.Fatal("existing browser session invalidated")
		}
		return value.Root
	}
	if state() != root {
		t.Fatal("browser used another store")
	}
	firstURL := opened.URL
	if err = json.Unmarshal(must(temp, "open", "--no-open", project), &opened); err != nil {
		t.Fatal(err)
	}
	if opened.PID != pid || firstURL != opened.URL || state() != root {
		t.Fatal("reopen did not reuse a stable session")
	}
	otherJar, _ := cookiejar.New(nil)
	otherClient := &http.Client{Jar: otherJar, Timeout: 3 * time.Second}
	if login(otherClient) != 200 {
		t.Fatal("reopening in another browser failed")
	}
	replayJar, _ := cookiejar.New(nil)
	if login(&http.Client{Jar: replayJar, Timeout: 3 * time.Second}) != 200 {
		t.Fatal("the same URL must work in another fresh browser")
	}
	if out, err := run(elsewhere, "open", "missing", "--no-open"); err == nil || !strings.Contains(string(out), "not_found") {
		t.Fatalf("missing project: %v %s", err, out)
	}
	if out, err := run(elsewhere, "open", "../elsewhere", "--no-open"); err == nil || !strings.Contains(string(out), "invalid_input") {
		t.Fatalf("arbitrary path: %v %s", err, out)
	}
	var status desktop.Info
	if err = json.Unmarshal(must(temp, "status"), &status); err != nil || status.PID != pid {
		t.Fatalf("status: %+v %v", status, err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = json.Unmarshal(must(temp, "status"), &status)
		if !status.Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status.Running {
		t.Fatal("killed server still responding")
	}
	if err = json.Unmarshal(must(elsewhere, "open", project, "--no-open"), &opened); err != nil || opened.PID == pid {
		t.Fatalf("crash recovery: %+v %v", opened, err)
	}
	newRoot := filepath.Join(temp, "changed-documents")
	must(temp, "config", "init", newRoot)
	newRoot, err = filepath.EvalSymlinks(newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := run(temp, "open", "--no-open"); err == nil || !strings.Contains(string(out), "config_changed") {
		t.Fatalf("changed config: %v %s", err, out)
	}
	if out, err := run(elsewhere, "open", project, "--no-open"); err == nil || !strings.Contains(string(out), "config_changed") {
		t.Fatalf("old project after config change: %v %s", err, out)
	}
	aliasStop := exec.CommandContext(ctx, binary, "stop")
	aliasStop.Env = append(env, "FRAGMENT_CONFIG_DIR="+configLink)
	if out, err := aliasStop.CombinedOutput(); err != nil {
		t.Fatalf("stop through config alias: %v %s", err, out)
	}
	if err = json.Unmarshal(must(temp, "status"), &status); err != nil || status.Running {
		t.Fatalf("stop: %+v %v", status, err)
	}
	if err = json.Unmarshal(must(elsewhere, "open", "--no-open"), &opened); err != nil || opened.Root != newRoot {
		t.Fatalf("new explicit config: %+v %v", opened, err)
	}
	if out, err := run(elsewhere, "open", project, "--no-open"); err == nil || !strings.Contains(string(out), "not_found") {
		t.Fatalf("old project after restart: %v %s", err, out)
	}
	must(temp, "stop")
	must(temp, "stop")
}
