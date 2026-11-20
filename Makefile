.PHONY: build test web-test cross

build:
	@if [ -f web/package.json ]; then cd web && npm ci && npm run build; else echo "build: web/package.json not present, skipping web build"; fi
	go build -o bin/flypanel ./cmd/flypanel

test:
	go test ./...

web-test:
	@if [ -f web/package.json ]; then cd web && npx vitest run; else echo "web-test: web/package.json not present, skipping"; fi

cross:
	GOOS=linux GOARCH=amd64 go build -o bin/flypanel-linux-amd64 ./cmd/flypanel
