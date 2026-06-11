//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/ppc64"
)

func main() {
	f := emit.NewFile("ppc64le")

	// xor16 computes out = a ^ b over a 16-byte vector using VSX: two LXVD2X
	// loads of 128-bit vectors, one VXOR, one STXVD2X store. The pointers are
	// plain ABI0 scalar arguments loaded into GPRs R3/R4/R5; the SIMD body is
	// emitted via Raw. LXVD2X/STXVD2X address (Rbase)(Rindex); R0 reads as the
	// constant 0, giving the (base+0) form.
	//
	// VSX↔VMX register aliasing: the AltiVec vector register Vn aliases the VSX
	// register VS(32+n) — NOT VSn. So a VXOR over V0/V1/V2 must be fed by
	// LXVD2X loads into VS32/VS33 and stored from VS34. (Loading into VS0/VS1
	// then operating on V0/V1 reads uninitialised registers — a bug the qemu
	// run catches.)
	sig := ppc64.Layout(
		[]string{"a", "b", "out"}, []ppc64.Type{ppc64.Ptr, ppc64.Ptr, ppc64.Ptr},
		nil, nil,
	)
	xor := ppc64.NewFunc("xor16", sig, 0)
	xor.LoadArg("a", "R3").
		LoadArg("b", "R4").
		LoadArg("out", "R5").
		Raw("LXVD2X (R3)(R0), VS32"). // load 16 bytes of a into V0 (=VS32)
		Raw("LXVD2X (R4)(R0), VS33"). // load 16 bytes of b into V1 (=VS33)
		Raw("VXOR V0, V1, V2").       // V2 = V0 ^ V1
		Raw("STXVD2X VS34, (R5)(R0)"). // store V2 (=VS34) to out
		Ret()
	f.Add(xor.Func())

	// eq16 compares two 16-byte vectors for per-byte equality using VCMPEQUB
	// (each output byte is 0xFF where equal, 0x00 otherwise), exercising a
	// second VSX/AltiVec form through the same backend.
	sigEq := ppc64.Layout(
		[]string{"a", "b", "out"}, []ppc64.Type{ppc64.Ptr, ppc64.Ptr, ppc64.Ptr},
		nil, nil,
	)
	eq := ppc64.NewFunc("eq16", sigEq, 0)
	eq.LoadArg("a", "R3").
		LoadArg("b", "R4").
		LoadArg("out", "R5").
		Raw("LXVD2X (R3)(R0), VS32"). // V0 = a
		Raw("LXVD2X (R4)(R0), VS33"). // V1 = b
		Raw("VCMPEQUB V0, V1, V2").    // per-byte equality mask in V2
		Raw("STXVD2X VS34, (R5)(R0)"). // store V2 to out
		Ret()
	f.Add(eq.Func())

	if err := os.WriteFile("simd_ppc64le.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_ppc64le.s")
}
