//go:build ignore

// Command gen produces simd_amd64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	f := emit.NewFile("amd64")

	// func addI32x4(a, b, out *[4]int32) — SSE2 packed 32-bit add.
	sig := amd64.Layout(
		[]string{"a", "b", "out"}, []amd64.Type{amd64.Ptr, amd64.Ptr, amd64.Ptr},
		nil, nil,
	)
	b := amd64.NewFunc("addI32x4", sig, 0)
	b.LoadArg("a", "AX").
		LoadArg("b", "BX").
		LoadArg("out", "CX").
		Raw("MOVOU (AX), X0"). // load 4 int32 lanes
		Raw("MOVOU (BX), X1").
		Raw("PADDL X1, X0").   // packed add, dword lanes
		Raw("MOVOU X0, (CX)"). // store 4 lanes
		Ret()
	f.Add(b.Func())

	if err := os.WriteFile("simd_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_amd64.s")
}
