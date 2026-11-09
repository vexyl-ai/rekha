export CGO_ENABLED=0

.PHONY: test lint wasm-check build bench

bench: build
	./bin/rekha-bench -corpus testdata/corpus -out bench/out

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)" && go vet ./...

wasm-check:
	GOOS=js GOARCH=wasm go build ./...

build:
	go build -o bin/ ./cmd/...
