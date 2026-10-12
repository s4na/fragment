#!/bin/sh
set -eu

# Include tracked and new source files; skip ignored dependencies and build output.
unformatted=$(git ls-files --cached --others --exclude-standard -z -- '*.go' | xargs -0 gofmt -l)
if [ -n "$unformatted" ]; then
    printf 'gofmtで整形が必要なファイル:\n%s\n' "$unformatted" >&2
    exit 1
fi

go vet ./...
