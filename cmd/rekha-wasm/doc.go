//go:build js && wasm

// Command rekha-wasm is the WebAssembly build of Rekha: it installs a
// global "rekha" object with segment, finish, free, trace and version. It
// builds only with GOOS=js GOARCH=wasm; see docs/wasm.md.
package main
