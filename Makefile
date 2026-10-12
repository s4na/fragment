.PHONY: build web test clean

web:
	sfw pnpm --dir web install --frozen-lockfile
	sfw pnpm --dir web build

build: web
	go build -o bin/fragment ./cmd/fragment

test: web
	go test -race ./...
	go vet ./...

clean:
	rm -rf bin web/dist
