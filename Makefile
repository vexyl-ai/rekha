export CGO_ENABLED=0

.PHONY: test lint wasm-check build

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)" && go vet ./...

wasm-check:
	GOOS=js GOARCH=wasm go build ./...

build:
	go build -o bin/ ./cmd/...
