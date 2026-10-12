package render

import (
	"github.com/s4na/fragment/internal/app"
	"strings"
	"testing"
)

func TestMarkdownAndBoundaries(t *testing.T) {
	html, err := HTML(app.Key{Project: "日本語", Kind: "flow", Path: "chat.md"}, "# Title\n\n[stock](../stock/概要.md)\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))\n\n| A | B |\n|---|---|\n|1|2|\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<h1", "<table>", "kind=stock", "path="} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %s: %s", want, html)
		}
	}
	for _, bad := range []string{"<script", "javascript:"} {
		if strings.Contains(html, bad) {
			t.Fatalf("unsafe: %s", html)
		}
	}
	html, err = HTML(app.Key{Project: "p", Kind: "stock", Path: "a.md"}, "[outside](../../secret.md)")
	if err != nil || strings.Contains(html, "secret.md") {
		t.Fatalf("outside link: %s %v", html, err)
	}
}
