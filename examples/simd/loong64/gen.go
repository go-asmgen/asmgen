//go:build ignore

// Command gen produces simd_loong64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/loong64"
)

func ptrSig() loong64.Signature {
	return loong64.Layout(
		[]string{"a", "b", "out"}, []loong64.Type{loong64.Ptr, loong64.Ptr, loong64.Ptr},
		nil, nil,
	)
}

func main() {
	f := emit.NewFile("loong64")

	// LSX: 4 x int32 packed add (128-bit V registers).
	lsx := loong64.NewFunc("addI32x4", ptrSig(), 0)
	lsx.LoadArg("a", "R4").LoadArg("b", "R5").LoadArg("out", "R6").
		Raw("VMOVQ (R4), V0").
		Raw("VMOVQ (R5), V1").
		Raw("VADDW V0, V1, V2").
		Raw("VMOVQ V2, (R6)").
		Ret()
	f.Add(lsx.Func())

	// LASX: 8 x int32 packed add (256-bit X registers).
	lasx := loong64.NewFunc("addI32x8", ptrSig(), 0)
	lasx.LoadArg("a", "R4").LoadArg("b", "R5").LoadArg("out", "R6").
		Raw("XVMOVQ (R4), X0").
		Raw("XVMOVQ (R5), X1").
		Raw("XVADDW X0, X1, X2").
		Raw("XVMOVQ X2, (R6)").
		Ret()
	f.Add(lasx.Func())

	if err := os.WriteFile("simd_loong64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_loong64.s")
}
