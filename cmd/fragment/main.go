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
	"syscall"
	"time"

	"github.com/s4na/fragment/internal/adapters/cli"
	httpadapter "github.com/s4na/fragment/internal/adapters/http"
	mcpadapter "github.com/s4na/fragment/internal/adapters/mcp"
	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/chat"
	"github.com/s4na/fragment/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"error": app.ErrorInfo(err)})
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) error {
	if len(args) > 0 && (args[0] == "mcp" || args[0] == "serve") {
		s, store, err := cli.Service()
		if err != nil {
			return err
		}
		if args[0] == "mcp" {
			if len(args) != 1 {
				return app.Fail("invalid_input", "fragment mcp")
			}
			return mcpadapter.Run(ctx, s)
		}
		flags := flag.NewFlagSet("serve", flag.ContinueOnError)
		model := flags.String("model", "", "Codexのモデル（省略時は設定またはapp-serverの標準）")
		codex := flags.String("codex", chat.DefaultExecutable(), "Codex CLIの実行ファイル")
		addr := flags.String("addr", "127.0.0.1:8787", "loopbackアドレス")
		if err = flags.Parse(args[1:]); err != nil {
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
		fmt.Fprintln(os.Stderr, "fragment:", handler.BootstrapURL())
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
	return cli.Run(ctx, args, os.Stdin, os.Stdout, os.Stderr)
}
