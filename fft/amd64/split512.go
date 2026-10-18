package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// The split layout at 512 bits (Round 28, AVX-512 Intel). genStockhamSplit's
// kernels with eight points per register: eight points 8q .. 8q+7 occupy the
// 128 bytes they occupy interleaved, as their real parts in the order (0, 4,
// 1, 5, 2, 6, 3, 7), then their imaginary parts in the same order. That is
// what VUNPCKLPD and VUNPCKHPD make of two interleaved ZMM loads
// ([r0 i0 r1 i1 r2 i2 r3 i3], [r4 i4 … r7 i7] → [r0 r4 r1 r5 r2 r6 r3 r7]),
// one in-lane shuffle per register each way, as at 256 bits; the twiddles are
// stored in that order (kernels.intelSplitTwiddles512). The bodies are
// genStockhamSplit's, operation for operation, so the result is the scalar
// pass's bits. The i = 0 point (lane 0 of a block's first group) is put
// back by a blend under opmask K1 = 1. The two constants (√2/2 and −0,
// kernels.splitK's rows) are broadcast from memory by the instructions that
// use them; the sign flip is VPXORQ, which AVX-512F has (VXORPD on ZMM needs
// AVX512DQ). Only Z0..Z15 are written, and the radix-8 body spills four of
// them to its frame as genStockhamSplit's does. VZEROUPPER does not clean
// Z16..Z31: a first build that kept those four values and the constants
// there ran the powers of two 1.07–1.08× faster at 2048 and 4096, but every
// composite transform run after it in the same process 4–5% slower on
// Cascade Lake (Round 28), and zeroing the registers on exit did not undo it.
//
// intelSplit512R{r}{in}{out}{Fwd|Inv}(cc, ch, tw *complex128, k *float64,
// ido, l1 int), ido a positive multiple of eight. Registers as in
// genStockhamSplit.

type intelSplit512Emit struct {
	c         *Kernels
	b         *asm.Builder
	r         int
	inS, outS bool
	inverse   bool
}

func (e intelSplit512Emit) raw(format string, a ...any) { e.b.Raw(format, a...) }

// intelZ names a ZMM register.
func intelZ(i int) string { return fmt.Sprintf("Z%d", i) }

// intelAt64 is addr 64 bytes further on.
func intelAt64(addr string) string { return "64" + addr }

// intelSplit512Frame holds the radix-8 body's four spilled registers.
const intelSplit512Frame = 256

func (e intelSplit512Emit) load(j, re, im, t0, t1 int) {
	a := skIn(j)
	if e.inS {
		e.raw("VMOVUPD %s, %s", a, intelZ(re))
		e.raw("VMOVUPD %s, %s", intelAt64(a), intelZ(im))
		return
	}
	e.raw("VMOVUPD %s, %s", a, intelZ(t0))
	e.raw("VMOVUPD %s, %s", intelAt64(a), intelZ(t1))
	e.raw("VUNPCKLPD %s, %s, %s", intelZ(t1), intelZ(t0), intelZ(re))
	e.raw("VUNPCKHPD %s, %s, %s", intelZ(t1), intelZ(t0), intelZ(im))
}

func (e intelSplit512Emit) store(j, re, im, t0, t1 int) {
	a := skOut(j)
	if e.outS {
		e.raw("VMOVUPD %s, %s", intelZ(re), a)
		e.raw("VMOVUPD %s, %s", intelZ(im), intelAt64(a))
		return
	}
	e.raw("VUNPCKLPD %s, %s, %s", intelZ(im), intelZ(re), intelZ(t0))
	e.raw("VUNPCKHPD %s, %s, %s", intelZ(im), intelZ(re), intelZ(t1))
	e.raw("VMOVUPD %s, %s", intelZ(t0), a)
	e.raw("VMOVUPD %s, %s", intelZ(t1), intelAt64(a))
}

func (e intelSplit512Emit) add(dst, a, b int) {
	e.raw("VADDPD %s, %s, %s", intelZ(b), intelZ(a), intelZ(dst))
}

func (e intelSplit512Emit) sub(dst, a, b int) {
	e.raw("VSUBPD %s, %s, %s", intelZ(b), intelZ(a), intelZ(dst))
}

// twStore is splitEmit.twStore at 512 bits: twiddle j of the group is 128
// bytes, eight real parts then eight imaginary parts.
func (e intelSplit512Emit) twStore(re, im, j int, t [4]int, first bool) {
	off := (j - 1) * 128
	e.raw("VMOVUPD %d(R10), %s", off, intelZ(t[0]))    // wr
	e.raw("VMOVUPD %d(R10), %s", off+64, intelZ(t[1])) // wi
	if e.c.FMA {
		// splitEmit.twStore's fused product at 512 bits.
		e.raw("VMULPD %s, %s, %s", intelZ(t[1]), intelZ(im), intelZ(t[2]))
		e.raw("VFMSUB231PD %s, %s, %s", intelZ(t[0]), intelZ(re), intelZ(t[2]))
		e.raw("VMULPD %s, %s, %s", intelZ(t[1]), intelZ(re), intelZ(t[3]))
		e.raw("VFMADD231PD %s, %s, %s", intelZ(t[0]), intelZ(im), intelZ(t[3]))
		if first {
			e.raw("VBLENDMPD %s, %s, K1, %s", intelZ(re), intelZ(t[2]), intelZ(t[2]))
			e.raw("VBLENDMPD %s, %s, K1, %s", intelZ(im), intelZ(t[3]), intelZ(t[3]))
		}
		e.store(j, t[2], t[3], t[0], t[1])
		return
	}
	e.raw("VMULPD %s, %s, %s", intelZ(t[0]), intelZ(re), intelZ(t[2]))
	e.raw("VMULPD %s, %s, %s", intelZ(t[1]), intelZ(im), intelZ(t[3]))
	e.sub(t[2], t[2], t[3]) // yr·wr − yi·wi
	e.raw("VMULPD %s, %s, %s", intelZ(t[0]), intelZ(im), intelZ(t[0]))
	e.raw("VMULPD %s, %s, %s", intelZ(t[1]), intelZ(re), intelZ(t[1]))
	e.add(t[0], t[0], t[1]) // yi·wr + yr·wi
	if first {
		// K1 = lane 0: point i = 0 keeps the unmultiplied value.
		e.raw("VBLENDMPD %s, %s, K1, %s", intelZ(re), intelZ(t[2]), intelZ(t[2]))
		e.raw("VBLENDMPD %s, %s, K1, %s", intelZ(im), intelZ(t[0]), intelZ(t[0]))
	}
	e.store(j, t[2], t[0], t[1], t[3])
}

// pm is a ± h·s, h = √2/2: plus = a + s, a = a − s for the default
// kernels, which scaled s by h before, and splitEmit.hpm's fused products
// for the FMA kernels (e.c.FMA).
func (e intelSplit512Emit) pm(plus, a, s int) {
	if !e.c.FMA {
		e.add(plus, a, s)
		e.sub(a, a, s)
		return
	}
	e.raw("VMOVAPD %s, %s", intelZ(a), intelZ(plus))
	e.raw("VFMADD231PD.BCST %d(R14), %s, %s", e.c.SplitK.H, intelZ(s), intelZ(plus))
	e.raw("VFNMADD231PD.BCST %d(R14), %s, %s", e.c.SplitK.H, intelZ(s), intelZ(a))
}

// rotAdd is splitEmit.rotAdd: pr, pi = a + rotS(b); mr, mi = a − rotS(b).
func (e intelSplit512Emit) rotAdd(pr, pi, mr, mi, ar, ai, br, bi int) {
	if !e.inverse {
		e.add(pr, ar, bi)
		e.sub(pi, ai, br)
		e.sub(mr, ar, bi)
		e.add(mi, ai, br)
		return
	}
	e.sub(pr, ar, bi)
	e.add(pi, ai, br)
	e.add(mr, ar, bi)
	e.sub(mi, ai, br)
}

// body4 is splitEmit.body's radix-4 case.
func (e intelSplit512Emit) body4(first bool) {
	for j := 0; j < 4; j++ {
		e.load(j, 2*j, 2*j+1, 12, 13)
	}
	e.add(8, 0, 4) // t2 = x0 + x2
	e.sub(0, 0, 4) // t1 = x0 − x2
	e.add(9, 1, 5)
	e.sub(1, 1, 5)
	e.add(4, 2, 6) // t3 = x1 + x3
	e.sub(2, 2, 6) // t4 = x1 − x3
	e.add(5, 3, 7)
	e.sub(3, 3, 7)
	e.add(6, 8, 4) // y0 = t2 + t3
	e.add(7, 9, 5)
	e.store(0, 6, 7, 10, 11)
	e.sub(8, 8, 4) // y2 = t2 − t3
	e.sub(9, 9, 5)
	e.rotAdd(4, 5, 0, 1, 0, 1, 2, 3) // y1, y3 = t1 ± rotS(t4)
	t := [4]int{10, 11, 12, 13}
	e.twStore(4, 5, 1, t, first)
	e.twStore(8, 9, 2, t, first)
	e.twStore(0, 1, 3, t, first)
}

// neg flips a register's signs in place: −x, exactly (VPXORQ with −0
// broadcast from the constants).
func (e intelSplit512Emit) neg(r int) {
	e.raw("VPXORQ.BCST %d(R14), %s, %s", e.c.SplitK.Neg, intelZ(r), intelZ(r))
}

// body8 is splitEmit.body8 at 512 bits, spilling the same four registers.
func (e intelSplit512Emit) body8(first bool) {
	// Odd half.
	e.load(1, 0, 1, 12, 13)
	e.load(5, 2, 3, 12, 13)
	e.add(4, 0, 2) // a1 = x1 + x5
	e.sub(0, 0, 2) // a5 = x1 − x5
	e.add(2, 1, 3)
	e.sub(1, 1, 3) // a1 (4,2), a5 (0,1)
	e.load(3, 3, 5, 12, 13)
	e.load(7, 6, 7, 12, 13)
	e.add(8, 3, 6) // a3 = x3 + x7
	e.sub(3, 3, 6) // a7 = x3 − x7
	e.add(6, 5, 7)
	e.sub(5, 5, 7) // a3 (8,6), a7 (3,5)
	e.add(7, 4, 8) // a1 + a3
	e.sub(4, 4, 8) // a1 − a3
	e.add(8, 2, 6)
	e.sub(2, 2, 6) // a1 (7,8), a3 (4,2), before rotS(a3)
	e.raw("VMOVUPD %s, 0(SP)", intelZ(7))
	e.raw("VMOVUPD %s, 64(SP)", intelZ(8))
	e.raw("VMOVUPD %s, 128(SP)", intelZ(4))
	e.raw("VMOVUPD %s, 192(SP)", intelZ(2))
	e.rotAdd(6, 9, 0, 1, 0, 1, 3, 5) // a5, a7 = a5 ± rotS(a7): a5 (6,9), a7 (0,1)
	if !e.inverse {
		// a5 = h·(a5r + a5i, a5i − a5r); a7 = h·(a7i − a7r, −a7r − a7i).
		e.add(3, 6, 9)
		e.sub(5, 9, 6)
		e.sub(6, 1, 0)
		e.neg(0)
		e.sub(0, 0, 1)
	} else {
		// a5 = h·(a5r − a5i, a5i + a5r); a7 = h·(−a7i − a7r, a7r − a7i).
		e.sub(3, 6, 9)
		e.add(5, 9, 6)
		e.sub(2, 0, 1)
		e.neg(1)
		e.sub(6, 1, 0)
		e.raw("VMOVAPD %s, %s", intelZ(2), intelZ(0))
	}
	if !e.c.FMA {
		for _, r := range []int{3, 5, 6, 0} {
			e.raw("VMULPD.BCST %d(R14), %s, %s", e.c.SplitK.H, intelZ(r), intelZ(r))
		} // a5 (3,5), a7 (6,0)
	}
	// Even half.
	e.load(0, 1, 2, 13, 14)
	e.load(4, 4, 7, 13, 14)
	e.add(8, 1, 4) // a0 = x0 + x4
	e.sub(1, 1, 4) // a4 = x0 − x4
	e.add(4, 2, 7)
	e.sub(2, 2, 7) // a0 (8,4), a4 (1,2)
	e.load(2, 7, 9, 13, 14)
	e.load(6, 10, 11, 13, 14)
	e.add(12, 7, 10) // a2 = x2 + x6
	e.sub(7, 7, 10)  // a6 = x2 − x6
	e.add(10, 9, 11)
	e.sub(9, 9, 11) // a2 (12,10), a6 (7,9)
	e.add(11, 8, 12)
	e.sub(8, 8, 12)
	e.add(12, 4, 10)
	e.sub(4, 4, 10)                    // a0 (11,12), a2 (8,4)
	e.rotAdd(10, 13, 1, 2, 1, 2, 7, 9) // a4, a6 = a4 ± rotS(a6): a4 (10,13), a6 (1,2)
	t := [4]int{3, 5, 14, 15}
	// y1, y5 = a4 ± a5.
	e.pm(7, 10, 3)
	e.pm(9, 13, 5)
	e.twStore(7, 9, 1, t, first)
	e.twStore(10, 13, 5, t, first)
	// y3, y7 = a6 ± a7.
	t = [4]int{3, 5, 10, 13}
	e.pm(7, 1, 6)
	e.pm(9, 2, 0)
	e.twStore(7, 9, 3, t, first)
	e.twStore(1, 2, 7, t, first)
	// y0, y4 = a0 ± a1.
	t = [4]int{0, 1, 2, 3}
	e.raw("VADDPD 0(SP), %s, %s", intelZ(11), intelZ(7))
	e.raw("VSUBPD 0(SP), %s, %s", intelZ(11), intelZ(11))
	e.raw("VADDPD 64(SP), %s, %s", intelZ(12), intelZ(9))
	e.raw("VSUBPD 64(SP), %s, %s", intelZ(12), intelZ(12))
	e.store(0, 7, 9, 5, 6)
	e.twStore(11, 12, 4, t, first)
	// y2, y6 = a2 ± rotS(a3).
	e.raw("VMOVUPD 128(SP), %s", intelZ(6))
	e.raw("VMOVUPD 192(SP), %s", intelZ(0))
	e.rotAdd(7, 9, 8, 4, 8, 4, 6, 0)
	t = [4]int{1, 2, 3, 5}
	e.twStore(7, 9, 2, t, first)
	e.twStore(8, 4, 6, t, first)
}

func (e intelSplit512Emit) body(first bool) {
	if e.r == 4 {
		e.body4(first)
		return
	}
	e.body8(first)
}

func (e intelSplit512Emit) advance(bytes int) {
	regs := []string{"AX", "BX"}
	if e.r > 4 {
		regs = append(regs, "SI", "DI")
	}
	for _, r := range regs {
		e.raw("ADDQ $%d, %s", bytes, r)
	}
}

// Split512 emits name(cc, ch, tw *complex128, k *float64, ido, l1 int), one
// 512-bit split-layout pass kernel of radix r (4 or 8, see above), with
// Split's layouts and direction and its constants table. ido is a positive
// multiple of eight.
func (c *Kernels) Split512(f *emit.File, name string, r int, inS, outS, inverse bool) {
	check("Split512", r, 4, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	frame := 0
	if r == 8 {
		frame = intelSplit512Frame
	}
	b := asm.NewFunc(name, sig, frame)
	e := intelSplit512Emit{c: c, b: b, r: r, inS: inS, outS: outS, inverse: inverse}
	b.Raw("MOVQ $1, R9").Raw("KMOVB R9, K1") // lane 0: the i = 0 point
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("tw", "R15").LoadArg("k", "R14").
		LoadArg("ido", "CX").LoadArg("l1", "R8")
	b.Raw("MOVQ CX, DX").
		Raw("IMULQ R8, DX").
		Raw("SHLQ $4, CX"). // S
		Raw("SHLQ $4, DX"). // OS
		Raw("LEAQ (CX)(CX*2), R12").
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("kloop:").
		Raw("TESTQ R8, R8").
		Raw("JZ done")
	if r > 4 {
		b.Raw("LEAQ (AX)(CX*4), SI").
			Raw("LEAQ (BX)(DX*4), DI")
	}
	b.Raw("MOVQ R15, R10").
		Raw("MOVQ CX, R9").
		Raw("SHRQ $7, R9") // groups = ido/8
	e.body(true)
	e.advance(128)
	b.Raw("ADDQ $%d, R10", (r-1)*128).
		Raw("DECQ R9").
		Raw("gloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ knext")
	e.body(false)
	e.advance(128)
	b.Raw("ADDQ $%d, R10", (r-1)*128).
		Raw("DECQ R9").
		Raw("JMP gloop").
		Raw("knext:")
	skNextBlock(b, r)
	b.Raw("DECQ R8").
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
