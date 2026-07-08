//go:build !windows

// Stub so `poc.exe window` fails gracefully on non-Windows instead of failing
// to compile. The real implementation lives in layered_windows.go (windows tag).
package main

import "fmt"

func runLayeredWindow() {
	fmt.Println("window mode requires Windows (WS_EX_LAYERED). Run on a Windows desktop.")
}
