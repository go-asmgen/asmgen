// Package riscv64 builds go-fft's riscv64 kernel (github.com/go-fft/fft) on top of
// go-asmgen's riscv64 builder: the RISC-V Vector (RVV 1.0) pointwise complex
// product.
package riscv64

import (
	"github.com/go-asmgen/asmgen/emit"
	asm "github.com/go-asmgen/asmgen/riscv64"
)

// CMulRVV emits name(a, b *complex128, n int), which computes a[i] = a[i] *
// b[i] for i in [0,n).
//
// A complex128 is two contiguous float64 {re, im} (16 bytes). The kernel is a
// strip-mined RVV loop: each iteration asks the hardware for a vector length
// vl = min(remaining, VLMAX) via VSETVLI (E64, M1), processes vl complex128 at
// once, and advances by vl. The segment load VLSEG2E64V deinterleaves a run of
// {re, im} pairs into a vector of reals and a vector of imaginaries; the
// arithmetic runs vl-wide; VSSEG2E64V re-interleaves on store. Because VSETVLI
// folds the tail (the last iteration simply gets a smaller vl), there is NO
// separate scalar tail — strip-mining is the idiomatic RVV form and handles any
// n, including 1 and odd counts, with the same vector code.
//
// Per element, with a = [ar, ai] and b = [br, bi], the product is
//
//	re = ar*br - ai*bi
//	im = ar*bi + ai*br
//
// Bit-for-bit identity with the scalar oracle CMulScalar is the contract, and it
// constrains the FUSION form. FMA is baseline on riscv64, so the gc compiler
// emits a FUSED form for the oracle (verified by disassembling CMulScalar with
// GOARCH=riscv64 -gcflags=-S):
//
//	re = ar*br        (FMULD,  exactly-rounded multiply)
//	re = re - ai*bi   (FNMSUBD, fused negative multiply-subtract)
//	im = ar*bi        (FMULD,  exactly-rounded multiply)
//	im = im + ai*br   (FMADDD,  fused multiply-add)
//
// The RVV kernel reproduces exactly that fusion, vl-wide — it is the precise
// RVV analogue of the arm64 NEON kernel (which matches arm64's FMSUBD/FMADDD):
//
//	VFMULVV   br, ar  -> re = ar*br            (== oracle FMULD)
//	VFNMSACVV bi, ai  -> re = re - ai*bi       (== oracle FNMSUBD: vd = -(vs1*vs2)+vd)
//	VFMULVV   bi, ar  -> im = ar*bi            (== oracle FMULD)
//	VFMACCVV  br, ai  -> im = im + ai*br       (== oracle FMADDD:  vd = +(vs1*vs2)+vd)
//
// VFMACC.VV computes vd = +(vs1*vs2) + vd and VFNMSAC.VV computes
// vd = -(vs1*vs2) + vd (RVV 1.0): exactly the oracle's fused add/subtract onto a
// separately-rounded leading product. A non-fused RVV kernel — or one whose
// fusion form differed — would diverge by up to 1 ULP; the random SIMD-vs-scalar
// test catches it, so this is validated, not assumed. The lesson is the arm64/
// s390x one: match the fusion, don't avoid it.
//
// Soundness / why this kernel cannot crash CI. The V extension is OPTIONAL on
// riscv64 and is absent from the default qemu-riscv64 CPU the CI arch-qemu job
// runs (VSETVLI would SIGILL there). The dispatch (cmul_riscv64.go) therefore
// probes for V at run time (parsing /proc/cpuinfo) and calls cmulRVV ONLY when V
// is present; under CI's non-V qemu it takes the scalar path, and the
// SIMD-vs-scalar test SKIPs its RVV comparison. On real RVV hardware (cfarm95,
// a SpacemiT X60, RVA22 + RVV 1.0) the V path is taken and the test RUNS and
// proves bit-identity. This kernel is hardware-validated on cfarm95.
//
// Endianness is a non-issue: riscv64 is little-endian, every lane does the
// identical scalar computation, and the segment load/store pair the {re,im}
// fields correctly by construction.
func CMulRVV(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"a", "b", "n"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Int64},
		nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("a", "X5"). // X5 = &a[0]
				LoadArg("b", "X6"). // X6 = &b[0]
				LoadArg("n", "X7"). // X7 = remaining element count
				Raw("loop:").
				Raw("BEQZ X7, done").
		// vl = min(X7, VLMAX) for e64/m1; vl returned in X10.
		Raw("VSETVLI X7, E64, M1, TA, MA, X10").
		// Deinterleave a -> V8=reals, V9=imags; b -> V10=reals, V11=imags.
		Raw("VLSEG2E64V (X5), V8").
		Raw("VLSEG2E64V (X6), V10").
		// re = ar*br - ai*bi  (FMUL then fused negative multiply-subtract).
		Raw("VFMULVV V8, V10, V12").   // V12 = ar*br            (== oracle FMULD)
		Raw("VFNMSACVV V9, V11, V12"). // V12 = V12 - ai*bi      (== oracle FNMSUBD)
		// im = ar*bi + ai*br  (FMUL then fused multiply-add).
		Raw("VFMULVV V8, V11, V13").  // V13 = ar*bi             (== oracle FMULD)
		Raw("VFMACCVV V9, V10, V13"). // V13 = V13 + ai*br       (== oracle FMADDD)
		// Re-interleave {re,im} and store back into a.
		Raw("VSSEG2E64V V12, (X5)").
		// Advance pointers by vl*16 bytes and decrement remaining count.
		Raw("SLLI $4, X10, X11"). // X11 = vl * 16
		Raw("ADD X11, X5").
		Raw("ADD X11, X6").
		Raw("SUB X10, X7"). // remaining -= vl
		Raw("JMP loop").
		Raw("done:").
		Ret()
	f.Add(b.Func())
}
