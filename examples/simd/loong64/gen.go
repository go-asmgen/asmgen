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

	// float64 FMA and broadcast loads through the LASX/LSX encoders
	// (lasx.go): Go's assembler has vector float64 add/mul but neither the
	// fused multiply-add nor the broadcast load.
	fmaSig := loong64.Layout([]string{"a", "b", "c", "out"},
		[]loong64.Type{loong64.Ptr, loong64.Ptr, loong64.Ptr, loong64.Ptr}, nil, nil)
	xf := loong64.NewFunc("fmaF64x4", fmaSig, 0) // out = a*b + c, 4 lanes
	xf.LoadArg("a", "R4").LoadArg("b", "R5").LoadArg("c", "R6").LoadArg("out", "R7").
		Raw("XVMOVQ (R4), X0").Raw("XVMOVQ (R5), X1").Raw("XVMOVQ (R6), X2").
		XVFMADDD(3, 0, 1, 2).
		Raw("XVMOVQ X3, (R7)").Ret()
	f.Add(xf.Func())
	vf := loong64.NewFunc("fmaF64x2", fmaSig, 0) // out = a*b + c, 2 lanes
	vf.LoadArg("a", "R4").LoadArg("b", "R5").LoadArg("c", "R6").LoadArg("out", "R7").
		Raw("VMOVQ (R4), V0").Raw("VMOVQ (R5), V1").Raw("VMOVQ (R6), V2").
		VFMADDD(3, 0, 1, 2).
		Raw("VMOVQ V3, (R7)").Ret()
	f.Add(vf.Func())
	bSig := loong64.Layout([]string{"p", "out4", "out2"},
		[]loong64.Type{loong64.Ptr, loong64.Ptr, loong64.Ptr}, nil, nil)
	bc := loong64.NewFunc("bcastF64", bSig, 0) // out4[i] = p[3], out2[i] = p[1]
	bc.LoadArg("p", "R4").LoadArg("out4", "R5").LoadArg("out2", "R6").
		XVLDREPLD(0, 4, 24).VLDREPLD(1, 4, 8).
		Raw("XVMOVQ X0, (R5)").Raw("VMOVQ V1, (R6)").Ret()
	f.Add(bc.Func())

	if err := os.WriteFile("simd_loong64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_loong64.s")
}
