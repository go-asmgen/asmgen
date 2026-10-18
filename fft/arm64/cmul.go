// Package arm64 builds the arm64 FFT kernels of go-fft (github.com/go-fft/fft)
// on top of go-asmgen's arm64 builder: NEON Stockham passes of radix 2, 3, 4,
// 5 and 8 in float64 and float32 (one pass, its final pass, a batched pass,
// and the block-split layout), the real-FFT untangle and retangle, and the
// pointwise complex product.
//
// Each builder appends one TEXT block to an emit.File under the symbol the
// caller names. Its arguments, register use and arithmetic are fixed and
// documented on the builder; the caller's Go declaration must match the
// arguments (go vet's asmdecl checks it). The kernels need no constants
// table: their constants are immediates.
//
// The arithmetic is a contract with a scalar reference, the Go code as gc
// compiles it for arm64, where FMA is baseline and gc fuses: each kernel
// fuses exactly the products gc fuses and rounds the ones it rounds, so it is
// bit-identical to the reference. go-fft's tests hold each kernel to it; this
// package's tests hold the emitted text to golden files and assemble it.
package arm64

import (
	"fmt"
	"slices"

	asm "github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

// CMulNEON emits name(a, b *complex128, n int), which computes a[i] = a[i] *
// b[i] for i in [0,n).
//
// A complex128 is two contiguous float64 {re, im} (16 bytes). The kernel
// processes TWO complex128 per iteration with NEON: VLD2 deinterleaves a pair
// into a vector of reals and a vector of imaginaries, the arithmetic runs
// two-lane, and VST2 re-interleaves on store. A scalar tail handles an odd
// final element.
//
// Per element, with a = [ar, ai] and b = [br, bi], the product is
//
//	re = ar*br - ai*bi
//	im = ar*bi + ai*br
//
// Bit-for-bit identity with the scalar oracle CMulScalar is the contract, and
// it constrains the FUSION form. Unlike amd64 (whose SSE2 MULPD/ADDPD are
// separately rounded and whose GOAMD64=v1 oracle does NOT fuse), the gc
// compiler emits a FUSED form for the oracle on arm64, because FMA is baseline:
//
//	re = (ar*br) then FMSUBD  -> a single fused multiply-subtract for ai*bi
//	im = (ar*bi) then FMADDD  -> a single fused multiply-add for ai*br
//
// (verified by disassembling CMulScalar with -gcflags=-S). A NEON kernel is
// therefore bit-identical ONLY if it reproduces that same fusion:
//
//	re = VFMUL ar,br (rounded); VFMLS ai,bi (re -= ai*bi, fused)
//	im = VFMUL ar,bi (rounded); VFMLA ai,br (im += ai*br, fused)
//
// A naive non-fused NEON (separate multiply then add) — or, equally, a fused
// kernel whose fusion form differs from the oracle's — diverges by up to 1 ULP;
// the random SIMD-vs-scalar test catches it, so this is validated, not assumed.
//
// Until Go 1.27 the arm64 assembler had no VFMUL, and the kernel formed each
// rounded product as VFMLA into a zeroed accumulator. That is the product
// rounded once, but not on signed zeros: +0 + (-0) is +0, so (-0)·1 came out
// +0 where the oracle's FMULD gives -0 (found 2026-10-05, go-fft's
// TestCMulSignedZeroNEON). VFMUL is exact there too.
func CMulNEON(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"a", "b", "n"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Int64},
		nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("a", "R0").
		LoadArg("b", "R1").
		LoadArg("n", "R2").
		Raw("loop2:").
		Raw("CMP $2, R2").
		Raw("BLT tail").
		Raw("VLD2 (R0), [V0.D2, V1.D2]"). // V0=[ar0,ar1] V1=[ai0,ai1]
		Raw("VLD2 (R1), [V2.D2, V3.D2]"). // V2=[br0,br1] V3=[bi0,bi1]
		VFMUL2D(4, 0, 2).                 // re = ar*br
		VFMLS2D(4, 1, 3).                 // re -= ai*bi  (fused, matches oracle FMSUBD)
		VFMUL2D(5, 0, 3).                 // im = ar*bi
		VFMLA2D(5, 1, 2).                 // im += ai*br  (fused, matches oracle FMADDD)
		Raw("VST2 [V4.D2, V5.D2], (R0)").
		Raw("ADD $32, R0").
		Raw("ADD $32, R1").
		Raw("SUB $2, R2").
		Raw("B loop2").
		Raw("tail:").
		Raw("CBZ R2, done").
		Raw("FMOVD (R0), F0").        // ar
		Raw("FMOVD 8(R0), F1").       // ai
		Raw("FMOVD (R1), F2").        // br
		Raw("FMOVD 8(R1), F3").       // bi
		Raw("FMULD F2, F0, F4").      // ar*br
		Raw("FMSUBD F1, F4, F3, F4"). // F4 - ai*bi  (fused, matches oracle)
		Raw("FMULD F3, F0, F5").      // ar*bi
		Raw("FMADDD F2, F5, F1, F5"). // F5 + ai*br  (fused, matches oracle)
		Raw("FMOVD F4, (R0)").
		Raw("FMOVD F5, 8(R0)").
		Raw("done:").
		Ret()
	f.Add(b.Func())
}

// check panics unless r is one of the radices a builder supports.
func check(builder string, r int, radices ...int) {
	if !slices.Contains(radices, r) {
		panic(fmt.Sprintf("fft/arm64: %s: radix %d not in %v", builder, r, radices))
	}
}
