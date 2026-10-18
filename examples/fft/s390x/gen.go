//go:build ignore

// Command gen produces fft_s390x.s with go-asmgen's fft/s390x builder. Run with:
// go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	fft "github.com/go-asmgen/asmgen/fft/s390x"
)

func main() {
	f := emit.NewFile("s390x")
	fft.CMulVX(f, "cmulVX")
	if err := os.WriteFile("fft_s390x.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote fft_s390x.s")
}
