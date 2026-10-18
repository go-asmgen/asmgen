package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// The float64 real-FFT untangle and its inverse (Round 4; Round 30).
//
// Untangle's name(dst, z, tw *complex128, k *float64, m, pairs int) is
// go-fft's rfftUntangle for bins k = 1 .. 2·pairs, two consecutive k per
// YMM register. Bin k reads Z[k] and Z[m-k]; for k and k+1 those are one
// forward load of Z[k..k+1] and one load of Z[m-k-1..m-k] with its 128-bit
// halves swapped, and the two mirrored outputs dst[m-k], dst[m-k-1] are stored
// the same way. Per bin, with Z[k] = a and Z[m-k] = b, s = a + b and
// d = a - b:
//
//	xe = [s.re, d.im]·0.5          (a blend, then the multiply)
//	xo = [s.im, -d.re]·0.5         (the other blend, a sign flip)
//	t  = W^k · xo                  (the complex product, VADDSUBPD)
//	dst[k] = xe + t;  dst[m-k] = conj(xe - t)
//
// which is rfftUntangle's arithmetic operation for operation, separately
// rounded with no FMA: bit-identical at GOAMD64=v1. k is the forward
// constants table: its row Sign (the forward rotation mask, [0, -0, 0, -0])
// is the imaginary-lane sign flip, row Half is 0.5. With inverse, the
// kernel is name(z, x, tw *complex128, k *float64, h *[4]float64, m, pairs
// int), irfftRetangle's loop the same way: with X[k] = a, X[m-k] = b
// and h = 0.5·scale,
//
//	xe = [s.re, d.im]·h;  dd = [d.re, s.im]·h
//	xo = conj(W^k)·dd
//	Z[k] = xe + i·xo;  Z[m-k] = conj(xe - i·xo)
//
// Round 4's kernels ran one step (two bins and their mirrors) per iteration,
// a chain of some twenty dependent instructions, with five shuffles besides
// the two lane swaps. Round 30 rebuilt them two ways, both of which pay on
// Zen 3: three independent steps per iteration, their instructions
// interleaved one by one (Round 27's lesson on Neoverse-N1), each step in its
// own four registers; and two shuffles fewer per step: the twiddle's real
// and imaginary parts come duplicated straight from memory (VMOVDDUP at tw
// and at tw+8, no VPERMILPD), and the untangle builds xo with its lanes
// swapped, [xo.im, xo.re] = [-d.re, s.im]·0.5 (the sign flipped before the
// halving, which is exact either way), so that one swap serves both
// products. Every product, sum and difference keeps the operands, in the
// order, of the Go code, so the results are the same bits. The pairs left
// over run one step at a time.

// smallUntangleSteps is how many steps one iteration interleaves: three use
// twelve registers, the constants three more.
const smallUntangleSteps = 3

// smallUntangleRegs gives step j its four registers Y(4j) .. Y(4j+3).
func smallUntangleRegs(j int) (a, b, c, d string) {
	r := func(i int) string { return fmt.Sprintf("Y%d", 4*j+i) }
	return r(0), r(1), r(2), r(3)
}

// smallUntangleStep returns step j's instructions, its loads and stores at
// offset 32·j from the cursors (AX forward input, BX mirrored input, CX
// twiddles, R9 forward output, R10 mirrored output). Y15 holds the
// imaginary-lane sign mask, Y13 the real-lane one, Y12 both lanes'; Y14
// holds 0.5 (untangle) or h (retangle).
func smallUntangleStep(j int, inverse bool) []string {
	a, b, c, d := smallUntangleRegs(j)
	o := 32 * j
	in := []string{
		fmt.Sprintf("VMOVUPD %d(AX), %s", o, a),
		fmt.Sprintf("VMOVUPD %d(BX), %s", -o, b),
		fmt.Sprintf("VPERM2F128 $1, %s, %s, %s", b, b, b),
		fmt.Sprintf("VADDPD %s, %s, %s", b, a, c), // s
		fmt.Sprintf("VSUBPD %s, %s, %s", b, a, d), // d
		fmt.Sprintf("VBLENDPD $10, %s, %s, %s", d, c, a),
		fmt.Sprintf("VMULPD Y14, %s, %s", a, a), // xe
		fmt.Sprintf("VBLENDPD $5, %s, %s, %s", d, c, b),
	}
	if !inverse {
		return append(in,
			fmt.Sprintf("VXORPD Y13, %s, %s", b, b),
			fmt.Sprintf("VMULPD Y14, %s, %s", b, b),    // [xo.im, xo.re]
			fmt.Sprintf("VMOVDDUP %d(CX), %s", o, c),   // [wr, wr]
			fmt.Sprintf("VMOVDDUP %d(CX), %s", o+8, d), // [wi, wi]
			fmt.Sprintf("VMULPD %s, %s, %s", b, d, d),
			fmt.Sprintf("VPERMILPD $5, %s, %s", b, b), // xo
			fmt.Sprintf("VMULPD %s, %s, %s", b, c, c),
			fmt.Sprintf("VADDSUBPD %s, %s, %s", d, c, c), // t
			fmt.Sprintf("VADDPD %s, %s, %s", c, a, b),
			fmt.Sprintf("VSUBPD %s, %s, %s", c, a, d),
			fmt.Sprintf("VXORPD Y15, %s, %s", d, d),
			fmt.Sprintf("VPERM2F128 $1, %s, %s, %s", d, d, d),
			fmt.Sprintf("VMOVUPD %s, %d(R9)", b, o),
			fmt.Sprintf("VMOVUPD %s, %d(R10)", d, -o),
		)
	}
	return append(in,
		fmt.Sprintf("VMULPD Y14, %s, %s", b, b),    // dd
		fmt.Sprintf("VMOVDDUP %d(CX), %s", o, d),   // [wr, wr]
		fmt.Sprintf("VMOVDDUP %d(CX), %s", o+8, c), // [wi, wi]
		fmt.Sprintf("VXORPD Y12, %s, %s", c, c),    // [-wi, -wi]
		fmt.Sprintf("VMULPD %s, %s, %s", b, d, d),
		fmt.Sprintf("VPERMILPD $5, %s, %s", b, b),
		fmt.Sprintf("VMULPD %s, %s, %s", b, c, c),
		fmt.Sprintf("VADDSUBPD %s, %s, %s", c, d, d), // xo
		fmt.Sprintf("VPERMILPD $5, %s, %s", d, b),
		fmt.Sprintf("VXORPD Y15, %s, %s", b, b), // [xoi, -xor]
		fmt.Sprintf("VSUBPD %s, %s, %s", b, a, c),
		fmt.Sprintf("VADDPD %s, %s, %s", b, a, d),
		fmt.Sprintf("VXORPD Y15, %s, %s", d, d),
		fmt.Sprintf("VPERM2F128 $1, %s, %s, %s", d, d, d),
		fmt.Sprintf("VMOVUPD %s, %d(R9)", c, o),
		fmt.Sprintf("VMOVUPD %s, %d(R10)", d, -o),
	)
}

// smallAdvance moves the five cursors past n steps.
func smallAdvance(b *asm.Builder, n int) {
	b.Raw("ADDQ $%d, AX", 32*n).
		Raw("SUBQ $%d, BX", 32*n).
		Raw("ADDQ $%d, R9", 32*n).
		Raw("SUBQ $%d, R10", 32*n).
		Raw("ADDQ $%d, CX", 32*n)
}

// Untangle emits the untangle kernel or, when inverse, the retangle kernel
// (see above): smallUntangleSteps steps per iteration while that many
// pairs are left, then one at a time. The last step reads tw[2·pairs+1].re
// (the VMOVDDUP at tw+8), which the Go wrappers bound-check.
func (c *Kernels) Untangle(f *emit.File, name string, inverse bool) {
	names := []string{"dst", "z", "tw", "k", "m", "pairs"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}
	if inverse {
		names = []string{"z", "x", "tw", "k", "h", "m", "pairs"}
		types = []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}
	}
	b := asm.NewFunc(name, asm.Layout(names, types, nil, nil), 0)
	b.LoadArg(names[0], "DI").LoadArg(names[1], "SI").LoadArg("tw", "CX").LoadArg("k", "R14").
		LoadArg("m", "DX").LoadArg("pairs", "R8")
	b.Raw("SHLQ $4, DX").
		Raw("LEAQ 16(SI), AX").                // &in[1]
		Raw("LEAQ -32(SI)(DX*1), BX").         // &in[m-2]
		Raw("LEAQ 16(DI), R9").                // &out[1]
		Raw("LEAQ -32(DI)(DX*1), R10").        // &out[m-2]
		Raw("ADDQ $16, CX").                   // &tw[1]
		Raw("VMOVUPD %d(R14), Y15", c.K.Sign). // imaginary-lane sign mask [0, -0]
		Raw("VPERMILPD $5, Y15, Y13").         // real-lane sign mask [-0, 0]
		Raw("VORPD Y15, Y13, Y12")             // both lanes
	if inverse {
		b.LoadArg("h", "R15").Raw("VMOVUPD (R15), Y14")
	} else {
		b.Raw("VMOVUPD %d(R14), Y14", c.K.Half) // 0.5
	}
	b.Raw("loop:").
		Raw("CMPQ R8, $%d", smallUntangleSteps).
		Raw("JLT tail")
	seqs := make([][]string, smallUntangleSteps)
	for j := range seqs {
		seqs[j] = smallUntangleStep(j, inverse)
	}
	for i := range seqs[0] {
		for j := range seqs {
			b.Raw("%s", seqs[j][i])
		}
	}
	smallAdvance(b, smallUntangleSteps)
	b.Raw("SUBQ $%d, R8", smallUntangleSteps).
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ R8, R8").
		Raw("JZ done")
	for _, ins := range smallUntangleStep(0, inverse) {
		b.Raw("%s", ins)
	}
	smallAdvance(b, 1)
	b.Raw("DECQ R8").
		Raw("JMP tail").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// clUntangleStep512 is smallUntangleStep at 512 bits (Round 31, Cascade
// Lake): four bins k .. k+3 and their mirrors m-k-3 .. m-k per step, at
// offset 64·j, in Z(4j) .. Z(4j+3). The operations are the AVX2 step's, lane
// for lane, with three substitutions AVX-512 needs: VBLENDPD becomes
// VBLENDMPD under K1 (odd lanes, 0xAA) or K2 (even lanes, 0x55); the mirror's
// four bins are reversed by VSHUFF64X2 $0x1B instead of VPERM2F128 swapping
// two; and VADDSUBPD, which has no 512-bit form, becomes a sign flip of the
// even lanes of the subtrahend (Z13, the real-lane mask) and an add: x − y
// and x + (−y) are the same IEEE operation, so the bits are the AVX2
// kernel's and the Go loop's. Z15, Z13, Z12 and Z14 hold the AVX2 kernel's
// Y15, Y13, Y12 and Y14 broadcast to eight lanes; VPXORQ flips signs
// (VXORPD on ZMM needs AVX512DQ). Only Z0..Z15 are written (Round 28: a
// dirty Z16..Z31 slowed code that never ran AVX-512).
func clUntangleStep512(j int, inverse bool) []string {
	z := func(i int) string { return fmt.Sprintf("Z%d", 4*j+i) }
	a, b, c, d := z(0), z(1), z(2), z(3)
	o := 64 * j
	in := []string{
		fmt.Sprintf("VMOVUPD %d(AX), %s", o, a),
		fmt.Sprintf("VMOVUPD %d(BX), %s", -o, b),
		fmt.Sprintf("VSHUFF64X2 $0x1b, %s, %s, %s", b, b, b),
		fmt.Sprintf("VADDPD %s, %s, %s", b, a, c), // s
		fmt.Sprintf("VSUBPD %s, %s, %s", b, a, d), // d
		fmt.Sprintf("VBLENDMPD %s, %s, K1, %s", d, c, a),
		fmt.Sprintf("VMULPD Z14, %s, %s", a, a), // xe
		fmt.Sprintf("VBLENDMPD %s, %s, K2, %s", d, c, b),
	}
	if !inverse {
		return append(in,
			fmt.Sprintf("VPXORQ Z13, %s, %s", b, b),
			fmt.Sprintf("VMULPD Z14, %s, %s", b, b),    // [xo.im, xo.re]
			fmt.Sprintf("VMOVDDUP %d(CX), %s", o, c),   // [wr, wr]
			fmt.Sprintf("VMOVDDUP %d(CX), %s", o+8, d), // [wi, wi]
			fmt.Sprintf("VMULPD %s, %s, %s", b, d, d),
			fmt.Sprintf("VPERMILPD $0x55, %s, %s", b, b), // xo
			fmt.Sprintf("VMULPD %s, %s, %s", b, c, c),
			fmt.Sprintf("VPXORQ Z13, %s, %s", d, d),
			fmt.Sprintf("VADDPD %s, %s, %s", d, c, c), // t
			fmt.Sprintf("VADDPD %s, %s, %s", c, a, b),
			fmt.Sprintf("VSUBPD %s, %s, %s", c, a, d),
			fmt.Sprintf("VPXORQ Z15, %s, %s", d, d),
			fmt.Sprintf("VSHUFF64X2 $0x1b, %s, %s, %s", d, d, d),
			fmt.Sprintf("VMOVUPD %s, %d(R9)", b, o),
			fmt.Sprintf("VMOVUPD %s, %d(R10)", d, -o),
		)
	}
	return append(in,
		fmt.Sprintf("VMULPD Z14, %s, %s", b, b),    // dd
		fmt.Sprintf("VMOVDDUP %d(CX), %s", o, d),   // [wr, wr]
		fmt.Sprintf("VMOVDDUP %d(CX), %s", o+8, c), // [wi, wi]
		fmt.Sprintf("VPXORQ Z12, %s, %s", c, c),    // [-wi, -wi]
		fmt.Sprintf("VMULPD %s, %s, %s", b, d, d),
		fmt.Sprintf("VPERMILPD $0x55, %s, %s", b, b),
		fmt.Sprintf("VMULPD %s, %s, %s", b, c, c),
		fmt.Sprintf("VPXORQ Z13, %s, %s", c, c),
		fmt.Sprintf("VADDPD %s, %s, %s", c, d, d), // xo
		fmt.Sprintf("VPERMILPD $0x55, %s, %s", d, b),
		fmt.Sprintf("VPXORQ Z15, %s, %s", b, b), // [xoi, -xor]
		fmt.Sprintf("VSUBPD %s, %s, %s", b, a, c),
		fmt.Sprintf("VADDPD %s, %s, %s", b, a, d),
		fmt.Sprintf("VPXORQ Z15, %s, %s", d, d),
		fmt.Sprintf("VSHUFF64X2 $0x1b, %s, %s, %s", d, d, d),
		fmt.Sprintf("VMOVUPD %s, %d(R9)", c, o),
		fmt.Sprintf("VMOVUPD %s, %d(R10)", d, -o),
	)
}

// Untangle512 emits name(dst, z, tw *complex128, k *float64, m, quads, pairs
// int) or, when inverse, name(z, x, tw *complex128, k *float64, h
// *[4]float64, m, quads, pairs int): Untangle's kernels at 512 bits, smallUntangleSteps
// steps of four bins per iteration while that many quads are left, then one
// at a time, nothing at all for quads = 0. The last step reads tw[4·quads+1].re (the VMOVDDUP at tw+8),
// which the Go wrappers bound-check. It then hands the pairs - 2·quads pairs
// left to the AVX2 kernel (Untangle's, whose symbol is tail) by a tail
// jump, its own arguments rewritten into the AVX2 kernel's (the slices 4·quads
// bins in, m - 8·quads, the pairs left), which share their layout: one call
// from Go whatever the CPU (Round 37: on Haswell the second call, with quads
// = 0, cost RFFT 256 about 2%). Round 35: when no pair is left it returns
// instead, as the wrappers' `if rest > 0` did.
func (c *Kernels) Untangle512(f *emit.File, name, tail string, inverse bool) {
	names := []string{"dst", "z", "tw", "k", "m", "quads", "pairs"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64, asm.Int64}
	if inverse {
		names = []string{"z", "x", "tw", "k", "h", "m", "quads", "pairs"}
		types = []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64, asm.Int64}
	}
	b := asm.NewFunc(name, asm.Layout(names, types, nil, nil), 0)
	b.LoadArg(names[0], "DI").LoadArg(names[1], "SI").LoadArg("tw", "CX").LoadArg("k", "R14").
		LoadArg("m", "DX").LoadArg("quads", "R8")
	// No step: on to the AVX2 kernel at once, before the first AVX-512
	// instruction, so the wrappers may call this on any CPU with quads = 0.
	b.Raw("TESTQ R8, R8").Raw("JZ none")
	b.Raw("SHLQ $4, DX").
		Raw("LEAQ 16(SI), AX").         // &in[1]
		Raw("LEAQ -64(SI)(DX*1), BX").  // &in[m-4]
		Raw("LEAQ 16(DI), R9").         // &out[1]
		Raw("LEAQ -64(DI)(DX*1), R10"). // &out[m-4]
		Raw("ADDQ $16, CX").            // &tw[1]
		Raw("MOVQ $0xaa, R11").Raw("KMOVB R11, K1").
		Raw("MOVQ $0x55, R11").Raw("KMOVB R11, K2").
		Raw("VBROADCASTF64X4 %d(R14), Z15", c.K.Sign). // imaginary-lane sign mask [0, -0]
		Raw("VPERMILPD $0x55, Z15, Z13").              // real-lane sign mask [-0, 0]
		Raw("VPORQ Z15, Z13, Z12")                     // both lanes
	if inverse {
		b.LoadArg("h", "R15").Raw("VBROADCASTF64X4 (R15), Z14")
	} else {
		b.Raw("VBROADCASTF64X4 %d(R14), Z14", c.K.Half) // 0.5
	}
	adv := func(n int) {
		b.Raw("ADDQ $%d, AX", 64*n).
			Raw("SUBQ $%d, BX", 64*n).
			Raw("ADDQ $%d, R9", 64*n).
			Raw("SUBQ $%d, R10", 64*n).
			Raw("ADDQ $%d, CX", 64*n)
	}
	b.Raw("loop:").
		Raw("CMPQ R8, $%d", smallUntangleSteps).
		Raw("JLT tail")
	seqs := make([][]string, smallUntangleSteps)
	for j := range seqs {
		seqs[j] = clUntangleStep512(j, inverse)
	}
	for i := range seqs[0] {
		for j := range seqs {
			b.Raw("%s", seqs[j][i])
		}
	}
	adv(smallUntangleSteps)
	b.Raw("SUBQ $%d, R8", smallUntangleSteps).
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ R8, R8").
		Raw("JZ done")
	for _, ins := range clUntangleStep512(0, inverse) {
		b.Raw("%s", ins)
	}
	adv(1)
	b.Raw("DECQ R8").
		Raw("JMP tail").
		Raw("done:").
		Raw("VZEROUPPER").
		Raw("none:")
	// The AVX2 kernel's arguments, in this kernel's slots (the same layout,
	// the last one unused): the three slices 4·quads bins (64·quads bytes)
	// in, m - 8·quads, and pairs - 2·quads in the slot it reads its pair
	// count from.
	// No pair left (Round 35): return, as the Go wrappers did when they
	// made the second call themselves.
	b.LoadArg("quads", "R8").LoadArg("pairs", "AX").
		Raw("SUBQ R8, AX").Raw("SUBQ R8, AX").
		Raw("JNZ rest").
		Ret()
	b.Raw("rest:").
		Raw("MOVQ R8, R11").Raw("SHLQ $6, R11")
	for _, p := range names[:3] {
		b.LoadArg(p, "AX").Raw("ADDQ R11, AX").Raw("MOVQ AX, %s+%d(FP)", p, 8*slicesIndex(names, p))
	}
	b.LoadArg("m", "AX").
		Raw("MOVQ R8, R11").Raw("SHLQ $3, R11").
		Raw("SUBQ R11, AX").Raw("MOVQ AX, m+%d(FP)", 8*slicesIndex(names, "m")).
		LoadArg("pairs", "AX").
		Raw("SUBQ R8, AX").Raw("SUBQ R8, AX").Raw("MOVQ AX, quads+%d(FP)", 8*slicesIndex(names, "quads")).
		Raw("JMP ·%s(SB)", tail)
	f.Add(b.Func())
}

// slicesIndex is the position of s in names (Round 37's tail jump in
// Untangle512).
func slicesIndex(names []string, s string) int {
	for i, n := range names {
		if n == s {
			return i
		}
	}
	panic(s)
}
