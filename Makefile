.PHONY: build web web-deps test lint lint-go lint-web clean

web: web-deps
	sfw pnpm --dir web build

web-deps:
	sfw pnpm --dir web install --frozen-lockfile

build: web
	go build -o bin/fragment ./cmd/fragment

test: web
	go test -race ./...
	go vet ./...

lint: lint-go lint-web

# go:embed requires the built Web UI even for go vet.
lint-go: web
	sh scripts/lint-go.sh

lint-web: web-deps
	sfw pnpm --dir web lint

clean:
	rm -rf bin web/dist
