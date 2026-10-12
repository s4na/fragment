package mcpadapter

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/s4na/fragment/internal/app"
)

type Empty struct{}
type ProjectInput struct {
	Project string `json:"project" jsonschema:"プロジェクト名"`
}
type ListInput struct {
	Project string `json:"project"`
	Kind    string `json:"kind" jsonschema:"flowまたはstock"`
	Limit   int    `json:"limit,omitempty"`
	After   string `json:"after,omitempty"`
}
type KeyInput struct {
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
}
type WriteInput struct {
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Content string `json:"content"`
}
type UpdateInput struct {
	Project  string `json:"project"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Revision string `json:"revision" jsonschema:"get_documentが返した内容ハッシュ"`
}

func result(value any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		b, _ := json.Marshal(app.ErrorInfo(err))
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
	}
	return nil, value, nil
}
func Server(s *app.Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "fragment", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_projects", Description: "共通保存先のプロジェクトを取得する"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ Empty) (*mcp.CallToolResult, any, error) {
		v, e := s.Projects(ctx)
		return result(map[string]any{"projects": v}, e)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "create_project", Description: "flowとstockを持つプロジェクトを作成する"}, func(ctx context.Context, _ *mcp.CallToolRequest, in ProjectInput) (*mcp.CallToolResult, any, error) {
		e := s.CreateProject(ctx, in.Project)
		return result(in, e)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_documents", Description: "flowまたはstockの文書を一覧する"}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, any, error) {
		v, e := s.Documents(ctx, in.Project, in.Kind, in.After, in.Limit)
		return result(v, e)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_document", Description: "Markdown本文と更新用リビジョンを取得する"}, func(ctx context.Context, _ *mcp.CallToolRequest, in KeyInput) (*mcp.CallToolResult, any, error) {
		v, e := s.Get(ctx, app.Key{Project: in.Project, Kind: in.Kind, Path: in.Path})
		return result(v, e)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "create_document", Description: "利用者が保存を指示した内容をflowまたはstockに新規保存する。内容や見出しは自由"}, func(ctx context.Context, _ *mcp.CallToolRequest, in WriteInput) (*mcp.CallToolResult, any, error) {
		v, e := s.Create(ctx, app.Key{Project: in.Project, Kind: in.Kind, Path: in.Path}, in.Content)
		return result(v, e)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "update_document", Description: "利用者の指示に従い取得時のリビジョンで更新する。競合したら最新の本文を取得して再検討する"}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateInput) (*mcp.CallToolResult, any, error) {
		v, e := s.Update(ctx, app.Key{Project: in.Project, Kind: in.Kind, Path: in.Path}, in.Content, in.Revision)
		return result(v, e)
	})
	return server
}
func Run(ctx context.Context, s *app.Service) error { return Server(s).Run(ctx, &mcp.StdioTransport{}) }
