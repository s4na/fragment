package render

import (
	"bytes"
	"net/url"
	"path"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/s4na/fragment/internal/app"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func HTML(k app.Key, content string) (string, error) {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	src := []byte(content)
	doc := md.Parser().Parse(text.NewReader(src))
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		u, err := url.Parse(string(link.Destination))
		if err != nil || u.IsAbs() || u.Host != "" || u.Path == "" {
			return ast.WalkContinue, nil
		}
		if strings.HasPrefix(u.Path, "/") {
			link.Destination = []byte("#")
			return ast.WalkContinue, nil
		}
		target := path.Join(k.Kind, path.Dir(k.Path), u.Path)
		parts := strings.SplitN(target, "/", 2)
		if len(parts) != 2 {
			return ast.WalkContinue, nil
		}
		key := app.Key{Project: k.Project, Kind: parts[0], Path: parts[1]}
		if err := key.Validate(); err != nil {
			link.Destination = []byte("#")
			return ast.WalkContinue, nil
		}
		q := url.Values{"project": {key.Project}, "kind": {key.Kind}, "path": {key.Path}}
		dest := url.URL{Path: "/", RawQuery: q.Encode(), Fragment: u.Fragment}
		link.Destination = []byte(dest.String())
		return ast.WalkContinue, nil
	})
	var out bytes.Buffer
	if err := md.Renderer().Render(&out, src, doc); err != nil {
		return "", err
	}
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("id").OnElements("h1", "h2", "h3", "h4", "h5", "h6")
	policy.RequireNoReferrerOnLinks(true)
	return policy.Sanitize(out.String()), nil
}
