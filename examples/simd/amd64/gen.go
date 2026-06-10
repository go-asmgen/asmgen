//go:build ignore

// Command gen produces simd_amd64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

func ptrSig() amd64.Signature {
	return amd64.Layout(
		[]string{"a", "b", "out"}, []amd64.Type{amd64.Ptr, amd64.Ptr, amd64.Ptr},
		nil, nil,
	)
}

func main() {
	f := emit.NewFile("amd64")

	// SSE2: 4 x int32 packed add.
	sse := amd64.NewFunc("addI32x4", ptrSig(), 0)
	sse.LoadArg("a", "AX").LoadArg("b", "BX").LoadArg("out", "CX").
		Raw("MOVOU (AX), X0").
		Raw("MOVOU (BX), X1").
		Raw("PADDL X1, X0").
		Raw("MOVOU X0, (CX)").
		Ret()
	f.Add(sse.Func())

	// AVX2: 8 x int32 packed add (256-bit Y registers).
	avx := amd64.NewFunc("addI32x8", ptrSig(), 0)
	avx.LoadArg("a", "AX").LoadArg("b", "BX").LoadArg("out", "CX").
		Raw("VMOVDQU (AX), Y0").
		Raw("VMOVDQU (BX), Y1").
		Raw("VPADDD Y1, Y0, Y2").
		Raw("VMOVDQU Y2, (CX)").
		Ret()
	f.Add(avx.Func())

	if err := os.WriteFile("simd_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_amd64.s")
}
