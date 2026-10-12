package web

import "embed"

// Assets contains the compiled Preact UI. Run sfw pnpm --dir web build first.
//
//go:embed dist/*
var Assets embed.FS
