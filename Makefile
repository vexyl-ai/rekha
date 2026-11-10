export CGO_ENABLED=0

.PHONY: test lint wasm-check wasm wasm-test web-test build bench

bench: build
	./bin/rekha-bench -corpus testdata/corpus -out bench/out

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)" && go vet ./...

wasm-check:
	GOOS=js GOARCH=wasm go build ./...

wasm:
	GOOS=js GOARCH=wasm go build -o web/rekha.wasm ./cmd/rekha-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/

wasm-test:
	GOOS=js GOARCH=wasm go test -exec "$$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./cmd/rekha-wasm $(ARGS)

web-test:
	node --test web/

build:
	go build -o bin/ ./cmd/...
