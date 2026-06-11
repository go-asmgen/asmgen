//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/s390x"
)

// This generator emits two 16-byte (128-bit, one vector register) byte-parallel
// kernels for the s390x vector facility, driven entirely by the s390x backend:
// scalar pointer-argument loads, the vector body via Raw, and the leaf return.
//
// Both kernels are byte-element-wise, so the s390x big-endian lane order is
// invisible to the RESULT: VL loads 16 bytes from the lowest address into V0's
// high-order lane downwards, VST writes them back in the same order, and VX /
// VCEQB operate per-byte. A reference computed in Go over the same byte slices
// therefore matches exactly — which is the point of choosing byte-parallel ops
// to validate the backend without an endian fix-up. (A cross-lane op such as
// VLGVB would expose the lane numbering; see the package doc.)
func main() {
	f := emit.NewFile("s390x")

	// xor16(a, b, out *[16]byte): out = a XOR b, 16 bytes at once.
	{
		sig := s390x.Layout(
			[]string{"a", "b", "out"},
			[]s390x.Type{s390x.Ptr, s390x.Ptr, s390x.Ptr},
			nil, nil,
		)
		b := s390x.NewFunc("xor16", sig, 0)
		b.LoadArg("a", "R1").
			LoadArg("b", "R2").
			LoadArg("out", "R3").
			Raw("VL (R1), V0"). // load 16 bytes of a (big-endian: lane 0 = lowest addr)
			Raw("VL (R2), V1"). // load 16 bytes of b
			Raw("VX V0, V1, V2"). // V2 = V0 XOR V1 (per-byte; lane order irrelevant)
			Raw("VST V2, (R3)").  // store 16 bytes to out
			Ret()
		f.Add(b.Func())
	}

	// eq16(a, b, out *[16]byte): out[i] = 0xFF if a[i]==b[i] else 0x00.
	// VCEQB sets each result byte to all-ones on equality, all-zeros otherwise.
	{
		sig := s390x.Layout(
			[]string{"a", "b", "out"},
			[]s390x.Type{s390x.Ptr, s390x.Ptr, s390x.Ptr},
			nil, nil,
		)
		b := s390x.NewFunc("eq16", sig, 0)
		b.LoadArg("a", "R1").
			LoadArg("b", "R2").
			LoadArg("out", "R3").
			Raw("VL (R1), V0").
			Raw("VL (R2), V1").
			Raw("VCEQB V0, V1, V2"). // per-byte compare-equal -> 0xFF / 0x00
			Raw("VST V2, (R3)").
			Ret()
		f.Add(b.Func())
	}

	if err := os.WriteFile("simd_s390x.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_s390x.s")
}
