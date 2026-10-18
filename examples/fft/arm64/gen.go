//go:build ignore

// Command gen produces fft_arm64.s with go-asmgen's fft/arm64 builders. Run
// with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	fft "github.com/go-asmgen/asmgen/fft/arm64"
)

func main() {
	f := emit.NewFile("arm64")
	fft.CMulNEON(f, "cmulNEON")
	fft.Pass(f, "pass4", 4, false)
	fft.Last(f, "last4", 4, false)
	if err := os.WriteFile("fft_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote fft_arm64.s")
}
