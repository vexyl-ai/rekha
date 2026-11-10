//go:build js && wasm

package main

func main() {
	install()
	select {}
}
