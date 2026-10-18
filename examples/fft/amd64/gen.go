//go:build ignore

// Command gen produces fft_amd64.s with go-asmgen's fft/amd64 builders. Run
// with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
	fft "github.com/go-asmgen/asmgen/fft/amd64"
)

func main() {
	f := emit.NewFile("amd64")
	fft.CMulSSE2(f, "cmulSSE2")
	fft.CMulAVX2(f, "cmulAVX2")
	// The radix-4 kernels read one row of the constants table: the rotation
	// sign mask, at offset 0 (see sign in fft.go).
	k := &fft.Kernels{K: fft.Consts{Sign: 0}}
	k.Pass(f, "pass4", 4)
	k.Last(f, "last4", 4)
	f.Add(amd64.FeatureProbe("hasAVX2", amd64.AVX2))
	if err := os.WriteFile("fft_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote fft_amd64.s")
}
