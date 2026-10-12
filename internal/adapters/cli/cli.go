package cli

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"

	"github.com/s4na/fragment/internal/app"
	"github.com/s4na/fragment/internal/config"
	"github.com/s4na/fragment/internal/storage"
)

func Service() (*app.Service, *storage.Store, error) {
	s, err := config.OpenStore()
	if err != nil {
		return nil, nil, err
	}
	return &app.Service{Repo: s}, s, nil
}
func Run(ctx context.Context, args []string, in io.Reader, out, errout io.Writer) error {
	if len(args) < 2 {
		return app.Fail("invalid_input", "使い方: fragment config init|show / project list|create / doc list|get|create|update / open / status / stop / serve / mcp")
	}
	var result any
	if args[0] == "config" {
		switch args[1] {
		case "init":
			if len(args) != 3 {
				return app.Fail("invalid_input", "fragment config init <保存先の絶対パス>")
			}
			c, err := config.Init(args[2])
			if err != nil {
				return err
			}
			result = c
		case "show":
			if len(args) != 2 {
				return app.Fail("invalid_input", "fragment config show")
			}
			c, err := config.Load()
			if err != nil {
				return err
			}
			result = c
		default:
			return app.Fail("invalid_input", "config initまたはshowを指定してください")
		}
	} else {
		s, _, err := Service()
		if err != nil {
			return err
		}
		switch args[0] {
		case "project":
			switch args[1] {
			case "list":
				if len(args) != 2 {
					return app.Fail("invalid_input", "fragment project list")
				}
				result, err = s.Projects(ctx)
			case "create":
				if len(args) != 3 {
					return app.Fail("invalid_input", "fragment project create <名前>")
				}
				err = s.CreateProject(ctx, args[2])
				result = map[string]string{"project": args[2]}
			default:
				return app.Fail("invalid_input", "project listまたはcreateを指定してください")
			}
		case "doc":
			flags := flag.NewFlagSet("doc", flag.ContinueOnError)
			flags.SetOutput(errout)
			project := flags.String("project", "", "プロジェクト")
			kind := flags.String("kind", "stock", "flowまたはstock")
			p := flags.String("path", "", "文書の相対パス")
			revision := flags.String("revision", "", "取得時のリビジョン")
			file := flags.String("file", "-", "本文の入力ファイル。-は標準入力")
			limit := flags.Int("limit", 100, "一覧の件数")
			after := flags.String("after", "", "一覧の継続位置")
			if err = flags.Parse(args[2:]); err != nil {
				return app.Fail("invalid_input", "文書操作の引数が不正です")
			}
			if flags.NArg() != 0 {
				return app.Fail("invalid_input", "予期しない位置引数があります")
			}
			key := app.Key{Project: *project, Kind: *kind, Path: *p}
			switch args[1] {
			case "list":
				result, err = s.Documents(ctx, *project, *kind, *after, *limit)
			case "get":
				result, err = s.Get(ctx, key)
			case "create", "update":
				reader := in
				if *file != "-" {
					f, e := os.Open(*file)
					if e != nil {
						return e
					}
					defer f.Close()
					reader = f
				}
				body, e := io.ReadAll(io.LimitReader(reader, app.MaxDocumentBytes+1))
				if e != nil {
					return e
				}
				if args[1] == "create" {
					result, err = s.Create(ctx, key, string(body))
				} else {
					result, err = s.Update(ctx, key, string(body), *revision)
				}
			default:
				return app.Fail("invalid_input", "doc list|get|create|updateを指定してください")
			}
		default:
			return app.Fail("invalid_input", "不明なコマンドです")
		}
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(out).Encode(result)
}
