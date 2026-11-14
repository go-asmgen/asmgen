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

	// The typed 256-bit ops: the four bitwise forms, a rotate, and the 4x4
	// transpose of 32-bit words a bitsliced cipher loads its blocks with.
	logic := amd64.NewFunc("ymmLogic", ptrSig(), 0)
	logic.LoadArg("a", "AX").LoadArg("b", "BX").LoadArg("out", "CX").
		VMOVDQU(amd64.Y0, "(AX)").
		VMOVDQU(amd64.Y1, "(BX)").
		VPXOR(amd64.Y2, amd64.Y0, amd64.Y1).VMOVDQUStore("(CX)", amd64.Y2).
		VPAND(amd64.Y2, amd64.Y0, amd64.Y1).VMOVDQUStore("32(CX)", amd64.Y2).
		VPOR(amd64.Y2, amd64.Y0, amd64.Y1).VMOVDQUStore("64(CX)", amd64.Y2).
		VPANDN(amd64.Y2, amd64.Y0, amd64.Y1).VMOVDQUStore("96(CX)", amd64.Y2).
		Ones(amd64.Y3).VPXOR(amd64.Y3, amd64.Y3, amd64.Y0).VMOVDQUStore("128(CX)", amd64.Y3).
		RotL32(amd64.Y4, amd64.Y0, 13, amd64.Y5).VMOVDQUStore("160(CX)", amd64.Y4).
		VPSLLD(amd64.Y6, amd64.Y0, 3).VMOVDQUStore("192(CX)", amd64.Y6).
		VPSRLD(amd64.Y6, amd64.Y0, 7).VMOVDQUStore("224(CX)", amd64.Y6).
		VPBROADCASTD(amd64.Y7, "4(BX)").VMOVDQUStore("256(CX)", amd64.Y7).
		VZEROUPPER().
		Ret()
	f.Add(logic.Func())

	tr := amd64.NewFunc("ymmTranspose", amd64.Layout(
		[]string{"p"}, []amd64.Type{amd64.Ptr}, nil, nil), 0)
	tr.LoadArg("p", "AX").
		VMOVDQU(amd64.Y0, "(AX)").VMOVDQU(amd64.Y1, "32(AX)").
		VMOVDQU(amd64.Y2, "64(AX)").VMOVDQU(amd64.Y3, "96(AX)").
		Transpose4x32(amd64.Y0, amd64.Y1, amd64.Y2, amd64.Y3, amd64.Y4, amd64.Y5).
		VMOVDQUStore("(AX)", amd64.Y0).VMOVDQUStore("32(AX)", amd64.Y1).
		VMOVDQUStore("64(AX)", amd64.Y2).VMOVDQUStore("96(AX)", amd64.Y3).
		VZEROUPPER().
		Ret()
	f.Add(tr.Func())

	// AVX-512F: ternary logic, the native rotate, an embedded-broadcast XOR
	// and the 16-lane transpose.
	zl := amd64.NewFunc("zmmLogic", ptrSig(), 0)
	zl.LoadArg("a", "AX").LoadArg("b", "BX").LoadArg("out", "CX").
		VMOVDQU32(amd64.Z0, "(AX)").
		VMOVDQU32(amd64.Z1, "(BX)").
		VMOVDQU32(amd64.Z2, "64(BX)").
		VMOVDQA32(amd64.Z3, amd64.Z0).VPTERNLOGD(0x96, amd64.Z3, amd64.Z1, amd64.Z2).VMOVDQU32Store("(CX)", amd64.Z3).
		VMOVDQA32(amd64.Z3, amd64.Z0).VPTERNLOGD(0xca, amd64.Z3, amd64.Z1, amd64.Z2).VMOVDQU32Store("64(CX)", amd64.Z3).
		VPROLD(amd64.Z4, amd64.Z0, 13).VMOVDQU32Store("128(CX)", amd64.Z4).
		VPXORDBcst(amd64.Z5, amd64.Z0, "4(BX)").VMOVDQU32Store("192(CX)", amd64.Z5).
		VPSLLD512(amd64.Z6, amd64.Z0, 3).VPSRLD512(amd64.Z6, amd64.Z6, 1).VMOVDQU32Store("256(CX)", amd64.Z6).
		VPADDD512(amd64.Z7, amd64.Z0, amd64.Z1).VMOVDQU32Store("320(CX)", amd64.Z7).
		VPBROADCASTD512(amd64.Z8, "8(BX)").VMOVDQU32Store("384(CX)", amd64.Z8).
		VZEROUPPER().
		Ret()
	f.Add(zl.Func())

	zt := amd64.NewFunc("zmmTranspose", amd64.Layout(
		[]string{"p"}, []amd64.Type{amd64.Ptr}, nil, nil), 0)
	zt.LoadArg("p", "AX").
		VMOVDQU32(amd64.Z0, "(AX)").VMOVDQU32(amd64.Z1, "64(AX)").
		VMOVDQU32(amd64.Z2, "128(AX)").VMOVDQU32(amd64.Z3, "192(AX)").
		Transpose4x32Z(amd64.Z0, amd64.Z1, amd64.Z2, amd64.Z3, amd64.Z4, amd64.Z5).
		VMOVDQU32Store("(AX)", amd64.Z0).VMOVDQU32Store("64(AX)", amd64.Z1).
		VMOVDQU32Store("128(AX)", amd64.Z2).VMOVDQU32Store("192(AX)", amd64.Z3).
		VZEROUPPER().
		Ret()
	f.Add(zt.Func())
	f.Add(amd64.FeatureProbe("hasAVX512F", amd64.AVX512F))

	// The gate in front of it. Without this the example calls a 256-bit
	// instruction on whatever CPU runs it, which is an illegal instruction on
	// anything older than Haswell -- the hazard every dispatched SIMD package
	// has to handle, so the example that shows the kernel should show the gate.
	f.Add(amd64.FeatureProbe("hasAVX2", amd64.AVX2))

	if err := os.WriteFile("simd_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_amd64.s")
}
