package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/s4na/fragment/internal/adapters/cli"
	httpadapter "github.com/s4na/fragment/internal/adapters/http"
	mcpadapter "github.com/s4na/fragment/internal/adapters/mcp"
	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/chat"
	"github.com/s4na/fragment/internal/desktop"
	"github.com/s4na/fragment/web"
)

// Set by the source launcher or release build, independently of VCS detection.
var buildVersion = "dev"
var buildCommit = "unknown"
var buildSource = "unknown"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"error": app.ErrorInfo(err)})
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "open" {
		return openDesktop(ctx, args)
	}
	if args[0] == "status" || args[0] == "stop" {
		if len(args) != 1 {
			return app.Fail("invalid_input", "fragment status / fragment stop")
		}
		var info desktop.Info
		var err error
		if args[0] == "stop" {
			info, err = desktop.Stop(ctx)
		} else {
			info, err = desktop.Status(ctx)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(info)
	}
	if args[0] == "_serve" {
		if len(args) != 2 {
			return app.Fail("invalid_input", "バックグラウンド起動の引数が不正です")
		}
		return desktop.Run(ctx, args[1], buildVersion, buildCommit, func(ctx context.Context, ready func(*httpadapter.Server) error) error {
			return serve(ctx, []string{"--addr", "127.0.0.1:0"}, ready)
		})
	}
	if len(args) > 0 && args[0] == "version" {
		if len(args) != 1 {
			return app.Fail("invalid_input", "fragment version")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"version": buildVersion, "commit": buildCommit, "source": buildSource})
	}
	if args[0] == "serve" {
		return serve(ctx, args[1:], nil)
	}
	if args[0] == "mcp" {
		if len(args) != 1 {
			return app.Fail("invalid_input", "fragment mcp")
		}
		service, _, err := cli.Service()
		if err != nil {
			return err
		}
		return mcpadapter.Run(ctx, service)
	}

	return cli.Run(ctx, args, os.Stdin, os.Stdout, os.Stderr)
}

func openDesktop(ctx context.Context, args []string) error {
	if len(args) != 0 {
		args = args[1:]
	}
	// Allow the project before or after --no-open, matching natural CLI use.
	noOpen := false
	flagsDone := false
	positional := []string{}
	for _, arg := range args {
		if !flagsDone && arg == "--" {
			flagsDone = true
		} else if !flagsDone && arg == "--no-open" {
			noOpen = true
		} else if !flagsDone && strings.HasPrefix(arg, "-") {
			return app.Fail("invalid_input", "fragment open [プロジェクト名] [--no-open]")
		} else {
			positional = append(positional, arg)
		}
	}
	if len(positional) > 1 {
		return app.Fail("invalid_input", "fragment open [プロジェクト名] [--no-open]")
	}
	project := ""
	if len(positional) == 1 {
		project = positional[0]
		if err := app.ValidateProject(project); err != nil {
			return err
		}
	}
	_, store, err := cli.Service()
	if err != nil {
		return err
	}
	info, err := desktop.Open(ctx, store.Root, project)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(os.Stdout).Encode(info); err != nil {
		return err
	}
	if noOpen {
		return nil
	}
	return desktop.Browse(ctx, info.URL)
}

func serve(ctx context.Context, args []string, ready func(*httpadapter.Server) error) error {
	s, store, err := cli.Service()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	model := flags.String("model", "", "Codexのモデル（省略時は設定またはapp-serverの標準）")
	codex := flags.String("codex", chat.DefaultExecutable(), "Codex CLIの実行ファイル")
	addr := flags.String("addr", "127.0.0.1:8787", "loopbackアドレス")
	if err = flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return app.Fail("invalid_input", "予期しない引数があります")
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return app.Fail("invalid_input", "serveのアドレスはloopbackのIPにしてください")
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	address := listener.Addr().String()
	origin := "http://" + address
	assets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		return err
	}
	handler := httpadapter.New(s, store.Root, origin, address, assets)
	handler.Chat = chat.New(ctx, s, chat.Codex{Executable: *codex, Model: *model})
	defer handler.Chat.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	if ready != nil {
		if err = ready(handler); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(os.Stderr, "fragment:", handler.SessionURL(""))
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-done:
		}
	}()
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
