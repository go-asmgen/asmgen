//go:build ignore

// Command gen produces fft_riscv64.s with go-asmgen's fft/riscv64 builder. Run with:
// go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	fft "github.com/go-asmgen/asmgen/fft/riscv64"
)

func main() {
	f := emit.NewFile("riscv64")
	fft.CMulRVV(f, "cmulRVV")
	if err := os.WriteFile("fft_riscv64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote fft_riscv64.s")
}
