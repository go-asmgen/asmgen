package arm64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// float64 real-FFT untangle and its inverse (Round 27), into
// untangle_arm64.s.
//
// armrealUntangleNEON(dst, z, tw *complex128, m, pairs int) is the fft
// package's rfftUntangle loop for bins k = 1 .. 2·pairs, and
// armrealRetangleNEON(z, x, tw *complex128, h *float64, m, pairs int) its
// irfftRetangle loop (retangle, inlined) for the same k: genF32rUntangle on
// .2D, two consecutive k per register pair. Z[k], Z[k+1] and the mirrored
// Z[m-k-1], Z[m-k] are loaded as they lie (VLD1) and split into real and
// imaginary parts by VZIP1/VZIP2, whose operand order also puts bin m-k-q in
// lane q of the mirrors; the outputs are joined the same way before their
// VST1. The twiddles come split from the table (kernels.UntangleTable:
// wr_k, wr_k+1, wi_k, wi_k+1 per pair), so one VLD1 loads them with no
// permute.
//
// Round 37 (Neoverse-N1) replaced Round 27's VLD2/VST2 and the four VEXT $8
// lane swaps a step with this: VLD2 and VST2 cost N1 more vector work than a
// VLD1 or VST1 and two VZIPs, and the split table drops the twiddles'
// permute altogether. One process, nine rotated rounds, untangle alone
// 1.19–1.21× and retangle 1.17–1.19× as fast from 256 to 4096 points
// (BENCHMARKS.md, Round 37).
//
// Bit identity. gc compiles rfftUntangle and retangle with the fusions it
// gives their float32 twins (go build -gcflags=-S, Go 1.27.1, realplan.go),
// with FMULD, FNMULD, FMSUBD, FMADDD and FNMSUBD for the single-precision
// forms; with s = a+b, d = a-b of the pair (a, b) = (bin k, bin m-k):
//
//	untangle: xor = s.im·0.5 and xoi = -(d.re·0.5) rounded (FMULD, FNMULD);
//	  tr = round(wr·xor) − wi·xoi and ti = round(wr·xoi) + wi·xor fused
//	  (FMSUBD, FMADDD); dst[k] = (tr + s.re·0.5, ti + d.im·0.5) fused
//	  (FMADDD); dst[m-k] = (s.re·0.5 − tr, −(d.im·0.5 − ti)) fused (FNMSUBD,
//	  then FNEGD).
//	retangle: dr = d.re·h, di = s.im·h rounded; xor = round(wr·dr) + wi·di,
//	  xoi = round(wr·di) − wi·dr fused; z[k] = (h·s.re − xoi, xor + h·d.im),
//	  z[m-k] = (xoi + h·s.re, −(h·d.im − xor)) fused.
//
// FNMSUBD (p − c, one rounding) has no vector form: the kernels negate c and
// VFMLA, the same operation on signed zeros too. A -race build fuses
// otherwise, so the tests compare bit for bit outside -race only.

// armrealOp is one instruction of an untangle step, its registers numbered
// from 0; emitting it at offset o adds o to each of them (V31, the constant,
// excepted).
type armrealOp func(b *asm.Builder, o int)

// armrealStep lists one step of the untangle (or, when inverse, retangle)
// loop: two bins k, k+1 and their mirrors, in V0..V13 (V31 = 0.5 or h).
// in, mir, tw, out and mout are the pointer registers: the bins, the mirrors
// (lower address), the twiddles, and the two outputs; in, tw and out
// post-increment.
func armrealStep(inverse bool, in, mir, tw, out, mout string) []armrealOp {
	const h = 31
	v3 := func(f func(*asm.Builder, int, int, int) *asm.Builder, d, n, m int) armrealOp {
		return func(b *asm.Builder, o int) {
			on := func(r int) int {
				if r == h {
					return r
				}
				return r + o
			}
			f(b, on(d), on(n), on(m))
		}
	}
	neg := func(d, n int) armrealOp {
		return func(b *asm.Builder, o int) { b.VFNEG2D(d+o, n+o) }
	}
	raw := func(format string, regs ...int) armrealOp {
		return func(b *asm.Builder, o int) {
			args := make([]any, len(regs))
			for i, r := range regs {
				args[i] = r + o
			}
			b.Raw(format, args...)
		}
	}
	add, sub, mul := (*asm.Builder).VFADD2D, (*asm.Builder).VFSUB2D, (*asm.Builder).VFMUL2D
	fmla, fmls := (*asm.Builder).VFMLA2D, (*asm.Builder).VFMLS2D
	// Ten registers a step: each is reused once its value is dead.
	// VZIP1 Vm, Vn, Vd (Go's operand order) is [Vn.d0, Vm.d0], VZIP2 the
	// same with lane 1.
	ops := []armrealOp{
		raw("VLD1.P 32("+in+"), [V%d.D2, V%d.D2]", 6, 7), // a[k], a[k+1] as they lie
		raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", 7, 6, 0),     // a: re
		raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", 7, 6, 1),     // a: im
		raw("VLD1 ("+mir+"), [V%d.D2, V%d.D2]", 8, 9),    // b[m-k-1], b[m-k] as they lie
		raw("VLD1.P 32("+tw+"), [V%d.D2, V%d.D2]", 4, 5), // wr, wi, split in the table
		raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", 8, 9, 2),     // b: re, lane q = bin m-k-q
		raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", 8, 9, 3),     // b: im
	}
	ops = append(ops,
		v3(add, 6, 0, 2), // s.re
		v3(sub, 7, 1, 3), // d.im
	)
	var kr, ki, mr, mi int // the registers stored
	if !inverse {
		ops = append(ops,
			v3(add, 8, 1, 3),  // s.im
			v3(sub, 9, 0, 2),  // d.re
			v3(mul, 0, 8, h),  // xor = s.im·0.5
			v3(mul, 1, 9, h),  // d.re·0.5
			neg(1, 1),         // xoi
			v3(mul, 2, 0, 4),  // wr·xor
			v3(fmls, 2, 5, 1), // tr = wr·xor − wi·xoi
			v3(mul, 3, 1, 4),  // wr·xoi
			v3(fmla, 3, 5, 0), // ti = wr·xoi + wi·xor
			neg(8, 2),         //
			v3(fmla, 8, 6, h), // dst[m-k].re = s.re·0.5 − tr
			v3(fmla, 2, 6, h), // dst[k].re = tr + s.re·0.5
			neg(9, 3),         //
			v3(fmla, 9, 7, h), // d.im·0.5 − ti
			neg(9, 9),         // dst[m-k].im
			v3(fmla, 3, 7, h), // dst[k].im = ti + d.im·0.5
		)
		kr, ki, mr, mi = 2, 3, 8, 9
	} else {
		ops = append(ops,
			v3(sub, 8, 0, 2),  // d.re
			v3(mul, 8, 8, h),  // dr = d.re·h
			v3(add, 9, 1, 3),  // s.im
			v3(mul, 9, 9, h),  // di = s.im·h
			v3(mul, 3, 8, 4),  // wr·dr
			v3(fmla, 3, 9, 5), // xor = wr·dr + wi·di
			v3(mul, 0, 9, 4),  // wr·di
			v3(fmls, 0, 8, 5), // xoi = wr·di − wi·dr
			neg(2, 0),         //
			v3(fmla, 2, 6, h), // z[k].re = h·s.re − xoi
			v3(fmla, 0, 6, h), // z[m-k].re = xoi + h·s.re
			neg(1, 3),         //
			v3(fmla, 1, 7, h), // h·d.im − xor
			neg(1, 1),         // z[m-k].im
			v3(fmla, 3, 7, h), // z[k].im = xor + h·d.im
		)
		kr, ki, mr, mi = 2, 3, 0, 1
	}
	// V4–V7 (the twiddles, s.re and d.im) are dead by now.
	return append(ops,
		raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", ki, kr, 6), // bin k
		raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", ki, kr, 7), // bin k+1
		raw("VST1.P [V%d.D2, V%d.D2], 32("+out+")", 6, 7),
		raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", mi, mr, 4), // bin m-k-1
		raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", mi, mr, 5), // bin m-k
		raw("VST1 [V%d.D2, V%d.D2], ("+mout+")", 4, 5),
	)
}

// armrealUnroll is how many untangle steps one loop iteration interleaves.
const armrealUnroll = 3

// genArmrealUntangle emits armrealUntangleNEON, or armrealRetangleNEON when
// inverse. R5 = &in[k], R7 = &out[k], R9 = &tw[k] (all post-incremented),
// R6 = &in[m-k-1] and R8 = &out[m-k-1] for the first step of an iteration,
// R11/R12 and R13/R14 the same 32 and 64 bytes lower for the next two, R4 =
// pairs left; V31 = 0.5 (or h) in both lanes.
//
// The loop runs armrealUnroll steps per iteration, their instructions
// interleaved (registers V0–V9, V10–V19, V20–V29). One step alone is a
// dependence chain of some 30 cycles carrying ~30 vector µops, which fill
// Neoverse-N1's vector queues: consecutive steps barely overlapped, and a
// one-step kernel ran no faster than the Go loop (Round 27). The pairs left
// over run as two steps or one.
func Untangle(f *emit.File, name string, inverse bool) {
	args := []string{"dst", "z", "tw", "m", "pairs"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}
	if inverse {
		args = []string{"z", "x", "tw", "h", "m", "pairs"}
		types = []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}
	}
	b := asm.NewFunc(name, asm.Layout(args, types, nil, nil), 0)
	b.LoadArg(args[0], "R0").LoadArg(args[1], "R1").LoadArg("tw", "R2").
		LoadArg("m", "R3").LoadArg("pairs", "R4")
	if inverse {
		b.LoadArg("h", "R10").Raw("FMOVD (R10), F31")
	} else {
		b.Raw("FMOVD $(0.5), F31")
	}
	b.Raw("VDUP V31.D[0], V31.D2").
		Raw("LSL $4, R3, R3"). // m·16
		Raw("ADD $16, R1, R5").
		Raw("ADD R3, R1, R6").
		Raw("SUB $32, R6, R6").
		Raw("ADD $16, R0, R7").
		Raw("ADD R3, R0, R8").
		Raw("SUB $32, R8, R8").
		Raw("ADD $16, R2, R9")
	mirs := [][2]string{{"R6", "R8"}, {"R11", "R12"}, {"R13", "R14"}}
	// steps emits u interleaved steps, then moves the mirror pointers down.
	steps := func(u int) {
		for s := 1; s < u; s++ {
			b.Raw("SUB $%d, R6, %s", 32*s, mirs[s][0]).
				Raw("SUB $%d, R8, %s", 32*s, mirs[s][1])
		}
		all := make([][]armrealOp, u)
		for s := range all {
			all[s] = armrealStep(inverse, "R5", mirs[s][0], "R9", "R7", mirs[s][1])
		}
		for i := range all[0] {
			for s := range all {
				all[s][i](b, 10*s)
			}
		}
		b.Raw("SUB $%d, R6, R6", 32*u).
			Raw("SUB $%d, R8, R8", 32*u)
	}
	u := armrealUnroll
	b.Label("loop").
		Raw("CMP $%d, R4", u).
		Raw("BLT tail")
	steps(u)
	b.Raw("SUB $%d, R4, R4", u).
		Raw("B loop").
		Label("tail")
	for t := u - 1; t >= 1; t-- {
		b.Raw("CMP $%d, R4", t).
			Raw("BLT tail%d", t)
		steps(t)
		b.Raw("SUB $%d, R4, R4", t)
		b.Label(fmt.Sprintf("tail%d", t))
	}
	b.Ret()
	f.Add(b.Func())
}
