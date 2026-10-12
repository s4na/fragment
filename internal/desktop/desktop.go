// Package desktop manages a per-configuration background Web UI through a
// private Unix socket. Documents are always accessed through the existing core.
package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	httpadapter "github.com/s4na/fragment/internal/adapters/http"
	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/config"
)

type Info struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	URL     string `json:"url,omitempty"`
	Root    string `json:"data_root,omitempty"`
	Version string `json:"version,omitempty"`
	Commit  string `json:"commit,omitempty"`
}

type paths struct{ dir, socket, log string }

func locations() (paths, error) {
	p, err := config.Path()
	if err != nil {
		return paths{}, err
	}
	id := app.Revision([]byte(p))[:16]
	base, err := runtimeBase()
	if err != nil {
		return paths{}, err
	}
	if err = privateDir(base); err != nil {
		return paths{}, err
	}
	dir := filepath.Join(base, id)
	if err = privateDir(dir); err != nil {
		return paths{}, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return paths{}, err
	}
	if err = os.MkdirAll(cache, 0o700); err != nil {
		return paths{}, err
	}
	logs := filepath.Join(cache, "fragment")
	if err = privateDir(logs); err != nil {
		return paths{}, err
	}
	return paths{dir, filepath.Join(dir, "control.sock"), filepath.Join(logs, "server-"+id+".log")}, nil
}

type request struct{ Action, Project string }
type response struct {
	Info
	Error *app.Error `json:"error,omitempty"`
}

func call(ctx context.Context, p paths, action, project string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", p.socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	b, _ := json.Marshal(request{action, project})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://fragment.local/control", bytes.NewReader(b))
	if err != nil {
		return Info{}, err
	}
	r.Header.Set("Content-Type", "application/json")
	res, err := client.Do(r)
	if err != nil {
		return Info{}, err
	}
	defer res.Body.Close()
	var value response
	if err = json.NewDecoder(io.LimitReader(res.Body, 32<<10)).Decode(&value); err != nil {
		return Info{}, app.Fail("runtime_error", "バックグラウンドサーバーの応答が不正です")
	}
	if value.Error != nil {
		return Info{}, value.Error
	}
	if res.StatusCode != http.StatusOK || value.PID <= 0 {
		return Info{}, app.Fail("runtime_error", "バックグラウンドサーバーの状態が不正です")
	}
	return value.Info, nil
}

func Status(ctx context.Context) (Info, error) {
	p, err := locations()
	if err != nil {
		return Info{}, err
	}
	info, err := call(ctx, p, "status", "")
	if unavailable(err) {
		return Info{Running: false}, nil
	}
	return info, err
}

func Stop(ctx context.Context) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := locations()
	if err != nil {
		return Info{}, err
	}
	unlock, err := lock(ctx, filepath.Join(p.dir, "start.lock"), true)
	if err != nil {
		return Info{}, err
	}
	defer unlock()
	info, err := call(ctx, p, "stop", "")
	if unavailable(err) {
		return Info{Running: false}, nil
	}
	if err != nil {
		return Info{}, err
	}
	// Wait for shutdown and chat cancellation before another opener can start.
	done, err := lock(ctx, filepath.Join(p.dir, "server.lock"), true)
	if err != nil {
		return Info{}, err
	}
	done()
	info.Running = false
	info.URL = ""
	return info, nil
}

func Open(ctx context.Context, root, project string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := locations()
	if err != nil {
		return Info{}, err
	}
	unlock, err := lock(ctx, filepath.Join(p.dir, "start.lock"), true)
	if err != nil {
		return Info{}, err
	}
	defer unlock()
	info, err := call(ctx, p, "status", "")
	if err != nil && !unavailable(err) {
		return Info{}, err
	}
	if err != nil {
		// A dying server may have closed its socket but still hold its lock.
		done, e := lock(ctx, filepath.Join(p.dir, "server.lock"), true)
		if e != nil {
			return Info{}, e
		}
		done()
		process, e := start(p, root)
		if e != nil {
			return Info{}, e
		}
		for {
			info, err = call(ctx, p, "status", "")
			if err == nil {
				_ = process.Release()
				break
			}
			select {
			case <-ctx.Done():
				_ = process.Kill()
				_, _ = process.Wait()
				return Info{}, app.Fail("server_start_failed", "サーバーを起動できませんでした。ログ: "+p.log)
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	if info.Root != root {
		return Info{}, app.Fail("config_changed", "実行中のサーバーと保存先設定が異なります。fragment stopのあと、もう一度開いてください")
	}
	return call(ctx, p, "open", project)
}

func start(p paths, root string) (*os.Process, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	log, err := privateFile(p.log, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	cmd := exec.Command(executable, "_serve", root)
	cmd.Dir = "/" // The launch directory never selects the document store.
	cmd.Stdout, cmd.Stderr = log, log
	detach(cmd)
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

// Run owns the daemon lock for the entire Web/chat lifetime. The management
// listener is Unix-only and private to the OS user; it is never exposed on TCP.
func Run(ctx context.Context, root, version, commit string, serve func(context.Context, func(*httpadapter.Server) error) error) error {
	p, err := locations()
	if err != nil {
		return err
	}
	unlock, err := lock(ctx, filepath.Join(p.dir, "server.lock"), false)
	if err != nil {
		return err
	}
	defer unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var control *http.Server
	var listener net.Listener
	defer func() {
		if control != nil {
			shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			_ = control.Shutdown(shutdown)
		}
		if listener != nil {
			_ = listener.Close()
		}
	}()
	return serve(ctx, func(web *httpadapter.Server) error {
		if web.Root != root {
			return app.Fail("config_changed", "起動中に保存先設定が変更されました。もう一度開いてください")
		}
		if err = os.Remove(p.socket); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		listener, err = net.Listen("unix", p.socket)
		if err != nil {
			return err
		}
		if err = os.Chmod(p.socket, 0o600); err != nil {
			return err
		}
		info := Info{true, os.Getpid(), web.Origin, root, version, commit}
		control = &http.Server{ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var in request
			if r.Method != http.MethodPost || r.URL.Path != "/control" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in) != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(response{Error: app.ErrorInfo(app.Fail("invalid_input", "起動管理リクエストが不正です"))})
				return
			}
			value := info
			var e error
			switch in.Action {
			case "status":
			case "open":
				if in.Project != "" {
					e = app.ValidateProject(in.Project)
					if e == nil {
						var projects []string
						projects, e = web.Service.Projects(r.Context())
						if e == nil && !slices.Contains(projects, in.Project) {
							e = app.Fail("not_found", "指定したプロジェクトが共通保存先に見つかりません")
						}
					}
				}
				if e == nil {
					value.URL = web.SessionURL(in.Project)
				}
			case "stop":
			default:
				e = app.Fail("invalid_input", "起動管理の操作が不正です")
			}
			if e != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(response{Error: app.ErrorInfo(e)})
				return
			}
			_ = json.NewEncoder(w).Encode(response{Info: value})
			if in.Action == "stop" {
				cancel()
			}
		})}
		go func() {
			if e := control.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
				cancel()
			}
		}()
		return nil
	})
}

func Browse(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	default:
		return app.Fail("unsupported", "このOSのブラウザ起動は未対応です")
	}
	if err := cmd.Run(); err != nil {
		return app.Fail("browser_error", "ブラウザを開けませんでした。表示したURLから接続するか、fragment open --no-openでURLを取得してください")
	}
	return nil
}
