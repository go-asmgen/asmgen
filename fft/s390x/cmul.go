// Package s390x builds go-fft's s390x kernel (github.com/go-fft/fft) on top of
// go-asmgen's s390x builder: the z/Architecture vector-facility pointwise complex
// product.
package s390x

import (
	"github.com/go-asmgen/asmgen/emit"
	asm "github.com/go-asmgen/asmgen/s390x"
)

// CMulVX emits name(a, b *complex128, n int), which computes a[i] = a[i] *
// b[i] for i in [0,n).
//
// A complex128 is two contiguous float64 {re, im} (16 bytes). The kernel
// processes TWO complex128 per iteration with the z/Architecture vector facility
// (V0–V31, 128-bit, two float64 lanes): it deinterleaves a pair into a vector of
// reals and a vector of imaginaries, runs the arithmetic two-lane, and
// re-interleaves on store. A scalar tail handles an odd final element.
//
// s390x is the ONLY big-endian go-asmgen target, and the only one of the four
// previously-scalar targets that can ship a bit-identical SIMD kernel here:
//   - riscv64's RVV faults under the CI's default qemu CPU (SIGILL on VSETVLI),
//   - loong64's LSX exposes no vector float add/mul/FMA in the Go assembler,
//   - ppc64le's VSX exposes no vector double arithmetic in the Go assembler,
//
// so all three keep the validated scalar fallback (see go-fft's cmul_generic.go). s390x's
// vector facility DOES expose 2-wide double FMA (VFMADB / VFMSDB) and plain
// multiply (VFMDB), so a bit-identical kernel is both expressible and validated
// (the per-arch qemu job runs it).
//
// Per element, with a = [ar, ai] and b = [br, bi], the product is
//
//	re = ar*br - ai*bi
//	im = ar*bi + ai*br
//
// Bit-for-bit identity with the scalar oracle CMulScalar is the contract, and it
// constrains the FUSION form. FMA is baseline on s390x, so the gc compiler emits
// a FUSED form for the oracle (verified by disassembling CMulScalar with
// -gcflags=-S):
//
//	t  = ai*bi              (FMUL, separately rounded)
//	re = ar*br - t          (FMSUB, fused multiply-subtract)
//	u  = ai*br              (FMUL, separately rounded)
//	im = ar*bi + u          (FMADD, fused multiply-add)
//
// The vector kernel reproduces exactly that fusion two-lane:
//
//	t  = VFMDB(ai, bi)            -> [ai*bi]          (exactly-rounded mul, == FMUL)
//	re = VFMSDB(ar, br, t)        -> [ar*br - ai*bi]  (fused, == FMSUB)
//	u  = VFMDB(ai, br)            -> [ai*br]          (exactly-rounded mul, == FMUL)
//	im = VFMADB(ar, bi, u)        -> [ar*bi + ai*br]  (fused, == FMADD)
//
// A non-fused vector kernel — or one whose fusion form differed — would diverge
// by up to 1 ULP; the random SIMD-vs-scalar test catches it, so this is
// validated, not assumed. The lesson is the arm64 one: match the fusion, don't
// avoid it.
//
// # Big-endian lane order (the part that bit-for-bit correctness hinges on)
//
// On s390x lane 0 of a vector is the HIGH-order (leftmost) doubleword, and a VL
// of {re, im} from memory puts re in lane 0, im in lane 1. The deinterleave uses
// VMRHG/VMRLG (merge high/low doubleword): VMRHG Va,Vb,Vd yields
// [Va.lane0, Vb.lane0] and VMRLG Va,Vb,Vd yields [Va.lane1, Vb.lane1]
// (confirmed empirically under qemu). So with V0={re0,im0}, V1={re1,im1}:
//
//	VMRHG V0,V1 -> [re0,re1]   (the two reals)
//	VMRLG V0,V1 -> [im0,im1]   (the two imaginaries)
//
// and re-interleaving the result vectors re=[r0,r1], im=[i0,i1] back to memory:
//
//	VMRHG re,im -> [r0,i0]     (first complex)
//	VMRLG re,im -> [r1,i1]     (second complex)
//
// Because every lane does the identical scalar computation, the result is
// endian-agnostic so long as the deinterleave/interleave are paired correctly —
// which the qemu job proves on this, the project's only big-endian arch.
func CMulVX(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"a", "b", "n"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Int64},
		nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("a", "R1").
		LoadArg("b", "R2").
		LoadArg("n", "R3").
		Raw("loop2:").
		Raw("CMPBLT R3, $2, tail").
		Raw("VL (R1), V0").             // V0 = a[0] = [ar0, ai0]
		Raw("VL 16(R1), V1").           // V1 = a[1] = [ar1, ai1]
		Raw("VL (R2), V2").             // V2 = b[0] = [br0, bi0]
		Raw("VL 16(R2), V3").           // V3 = b[1] = [br1, bi1]
		Raw("VMRHG V0, V1, V4").        // V4 = [ar0, ar1]  (reals of a)
		Raw("VMRLG V0, V1, V5").        // V5 = [ai0, ai1]  (imags of a)
		Raw("VMRHG V2, V3, V6").        // V6 = [br0, br1]  (reals of b)
		Raw("VMRLG V2, V3, V7").        // V7 = [bi0, bi1]  (imags of b)
		Raw("VFMDB V5, V7, V16").       // V16 = ai*bi            (== oracle FMUL)
		Raw("VFMSDB V4, V6, V16, V17"). // V17 = ar*br - ai*bi   (== oracle FMSUB)
		Raw("VFMDB V5, V6, V18").       // V18 = ai*br            (== oracle FMUL)
		Raw("VFMADB V4, V7, V18, V19"). // V19 = ar*bi + ai*br   (== oracle FMADD)
		Raw("VMRHG V17, V19, V20").     // V20 = [re0, im0]  (first complex)
		Raw("VMRLG V17, V19, V21").     // V21 = [re1, im1]  (second complex)
		Raw("VST V20, (R1)").
		Raw("VST V21, 16(R1)").
		Raw("MOVD $32, R4").
		Raw("ADD R4, R1").
		Raw("ADD R4, R2").
		Raw("SUB $2, R3").
		Raw("BR loop2").
		Raw("tail:").
		Raw("CMPBEQ R3, $0, done").
		Raw("FMOVD (R1), F0").   // ar
		Raw("FMOVD 8(R1), F1").  // ai
		Raw("FMOVD (R2), F2").   // br
		Raw("FMOVD 8(R2), F3").  // bi
		Raw("FMOVD F1, F4").     // save ai
		Raw("FMUL F3, F1").      // F1 = ai*bi          (== oracle FMUL)
		Raw("FMSUB F0, F2, F1"). // F1 = ar*br - ai*bi   (== oracle FMSUB)
		Raw("FMOVD F1, (R1)").
		Raw("FMUL F4, F2").      // F2 = ai*br          (== oracle FMUL)
		Raw("FMADD F3, F0, F2"). // F2 = ar*bi + ai*br   (== oracle FMADD)
		Raw("FMOVD F2, 8(R1)").
		Raw("done:").
		Ret()
	f.Add(b.Func())
}
