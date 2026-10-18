package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Split-layout Stockham passes (Round 23).
//
// A power of two keeps its data block-split between passes: four points
// p = 4q .. 4q+3 occupy the 64 bytes they occupy interleaved, as the real
// parts re(4q), re(4q+2), re(4q+1), re(4q+3), then the imaginary parts in the
// same order. That order is what VUNPCKLPD and VUNPCKHPD make of two
// interleaved YMM loads ([r0 i0 r1 i1], [r2 i2 r3 i3] -> [r0 r2 r1 r3] and
// [i0 i2 i1 i3]), one in-lane shuffle per register, and they undo it the
// same way. Inside a pass the points of a block are independent, so the
// order within the block does not matter as long as the twiddles are stored
// in it too (kernels.StockhamSplitTwiddles).
//
// With the parts apart, a YMM register holds one part of four points: the
// ±i rotations are a choice of operands (no shuffle, no sign flip), and a
// twiddle product is four multiplies and two adds per four points with no
// shuffle, where the interleaved kernels spend two shuffles per two points.
//
// skSplit{r}{in}{out}{Fwd|Inv}(cc, ch, tw *complex128, k *float64, ido, l1
// int) runs one whole pass (ido a multiple of four) reading cc interleaved
// (I) or split (S) and writing ch the same way. The direction is compiled in:
// the rotations differ in which operand is added and which subtracted. The
// arithmetic is stockham.go's, operation for operation, separately rounded,
// so the result is the scalar pass's bits: every output is the same IEEE
// operation on the same operands, a − (−b) and a + (−b) being a + b and
// a − b exactly. Point i = 0 of a block (lane 0 of its first group) is
// multiplied by 1 with its group and then put back by a blend, as in the
// interleaved kernels.
//
// Registers as in genStockhamAVX2: AX/SI inputs, BX/DI outputs, CX = S, DX =
// OS, R12 = 3·S, R13 = 3·OS, R8 blocks left, R9 groups left, R10 twiddle
// cursor, R15 twiddle base, R14 constants (kernels.splitConst).

type splitEmit struct {
	c         *Kernels
	b         *asm.Builder
	r         int
	inS, outS bool // input / output split
	inverse   bool
}

func (e splitEmit) raw(format string, a ...any) { e.b.Raw(format, a...) }

// y names a YMM register.
func y(i int) string { return fmt.Sprintf("Y%d", i) }

// at32 is addr 32 bytes further on: "(AX)(CX*1)" -> "32(AX)(CX*1)".
func at32(addr string) string { return "32" + addr }

// load puts input stream j's four points, split, into re and im; t0 and t1
// are scratch for an interleaved input.
func (e splitEmit) load(j, re, im, t0, t1 int) {
	a := skIn(j)
	if e.inS {
		e.raw("VMOVUPD %s, %s", a, y(re))
		e.raw("VMOVUPD %s, %s", at32(a), y(im))
		return
	}
	e.raw("VMOVUPD %s, %s", a, y(t0))
	e.raw("VMOVUPD %s, %s", at32(a), y(t1))
	e.raw("VUNPCKLPD %s, %s, %s", y(t1), y(t0), y(re))
	e.raw("VUNPCKHPD %s, %s, %s", y(t1), y(t0), y(im))
}

// store writes re, im to output stream j; t0 and t1 are scratch for an
// interleaved output.
func (e splitEmit) store(j, re, im, t0, t1 int) {
	a := skOut(j)
	if e.outS {
		e.raw("VMOVUPD %s, %s", y(re), a)
		e.raw("VMOVUPD %s, %s", y(im), at32(a))
		return
	}
	e.raw("VUNPCKLPD %s, %s, %s", y(im), y(re), y(t0))
	e.raw("VUNPCKHPD %s, %s, %s", y(im), y(re), y(t1))
	e.raw("VMOVUPD %s, %s", y(t0), a)
	e.raw("VMOVUPD %s, %s", y(t1), at32(a))
}

func (e splitEmit) add(dst, a, b int) { e.raw("VADDPD %s, %s, %s", y(b), y(a), y(dst)) }
func (e splitEmit) sub(dst, a, b int) { e.raw("VSUBPD %s, %s, %s", y(b), y(a), y(dst)) }

// twStore multiplies (re, im) by twiddle j and stores it to output j:
// (yr·wr − yi·wi, yi·wr + yr·wi), the scalar product with its imaginary sum
// in the other order, which IEEE addition makes exact. t holds four scratch
// registers. In the first group of a block, lane 0 (point i = 0) keeps the
// unmultiplied value.
func (e splitEmit) twStore(re, im, j int, t [4]int, first bool) {
	off := (j - 1) * 64
	e.raw("VMOVUPD %d(R10), %s", off, y(t[0]))    // wr
	e.raw("VMOVUPD %d(R10), %s", off+32, y(t[1])) // wi
	if e.c.FMA {
		// re = fma(yr, wr, −(yi·wi)), im = fma(yi, wr, yr·wi): cmulFMA.
		e.raw("VMULPD %s, %s, %s", y(t[1]), y(im), y(t[2]))
		e.raw("VFMSUB231PD %s, %s, %s", y(t[0]), y(re), y(t[2]))
		e.raw("VMULPD %s, %s, %s", y(t[1]), y(re), y(t[3]))
		e.raw("VFMADD231PD %s, %s, %s", y(t[0]), y(im), y(t[3]))
		if first {
			e.raw("VBLENDPD $1, %s, %s, %s", y(re), y(t[2]), y(t[2]))
			e.raw("VBLENDPD $1, %s, %s, %s", y(im), y(t[3]), y(t[3]))
		}
		e.store(j, t[2], t[3], t[0], t[1])
		return
	}
	e.raw("VMULPD %s, %s, %s", y(t[0]), y(re), y(t[2]))
	e.raw("VMULPD %s, %s, %s", y(t[1]), y(im), y(t[3]))
	e.sub(t[2], t[2], t[3]) // yr·wr − yi·wi
	e.raw("VMULPD %s, %s, %s", y(t[0]), y(im), y(t[0]))
	e.raw("VMULPD %s, %s, %s", y(t[1]), y(re), y(t[1]))
	e.add(t[0], t[0], t[1]) // yi·wr + yr·wi
	if first {
		e.raw("VBLENDPD $1, %s, %s, %s", y(re), y(t[2]), y(t[2]))
		e.raw("VBLENDPD $1, %s, %s, %s", y(im), y(t[0]), y(t[0]))
	}
	e.store(j, t[2], t[0], t[1], t[3])
}

// rotAdd emits the pair (a + rotS(b), a − rotS(b)) on split registers:
// rotS(b) = (−s·bi, s·br), so forward (s = −1) a + rotS(b) = (ar + bi,
// ai − br) and a − rotS(b) = (ar − bi, ai + br); inverse the other way.
// pr, pi receive a + rotS(b), mr, mi a − rotS(b).
func (e splitEmit) rotAdd(pr, pi, mr, mi, ar, ai, br, bi int) {
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

func (e splitEmit) body(first bool) {
	switch e.r {
	case 4:
		// x0 (0,1) x1 (2,3) x2 (4,5) x3 (6,7), scratch 12, 13.
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
	case 8:
		e.body8(first)
	case 3:
		e.body3(first)
	case 5:
		e.body5(first)
	}
}

// mulc multiplies register a by the constant row at off into dst: c·a,
// the product the Go pass forms (a product's operands commute exactly).
func (e splitEmit) mulc(dst, a, off int) {
	e.raw("VMULPD %d(R14), %s, %s", off, y(a), y(dst))
}

// body3 is bfly3 (stockham.go's pass3) on split registers (Round 36): t1 =
// x1 + x2, t2 = x1 − x2, y0 = x0 + t1, ca = x0 − 0.5·t1, cb =
// rotS(sin(2π/3)·t2), y1 = ca + cb, y2 = ca − cb.
func (e splitEmit) body3(first bool) {
	e.load(1, 2, 3, 12, 13)
	e.load(2, 4, 5, 12, 13)
	e.add(6, 2, 4) // t1 = x1 + x2
	e.sub(2, 2, 4) // t2 = x1 − x2
	e.add(7, 3, 5)
	e.sub(3, 3, 5) // t1 (6,7), t2 (2,3)
	e.load(0, 0, 1, 12, 13)
	e.add(8, 0, 6) // y0 = x0 + t1
	e.add(9, 1, 7)
	e.store(0, 8, 9, 12, 13)
	e.mulc(10, 6, e.c.SplitK.Half)
	e.sub(10, 0, 10) // ca = x0 − 0.5·t1
	e.mulc(11, 7, e.c.SplitK.Half)
	e.sub(11, 1, 11)
	e.mulc(2, 2, e.c.SplitK.Sin120)
	e.mulc(3, 3, e.c.SplitK.Sin120)    // sin(2π/3)·t2
	e.rotAdd(4, 5, 6, 7, 10, 11, 2, 3) // y1, y2 = ca ± rotS(·)
	t := [4]int{0, 1, 2, 3}
	e.twStore(4, 5, 1, t, first)
	e.twStore(6, 7, 2, t, first)
}

// body5 is pass5's butterfly (stockham.go) on split registers (Round 36):
// t1, t2 = x1 ± x4, t3, t4 = x2 ± x3, y0 = (x0 + t1) + t3, r1 = x0 +
// (c51·t1 + c52·t3), r2 = x0 + (c52·t1 + c51·t3), i1 = rotS(s51·t2 +
// s52·t4), i2 = rotS(s52·t2 − s51·t4), y1, y4 = r1 ± i1, y2, y3 = r2 ± i2.
func (e splitEmit) body5(first bool) {
	e.load(1, 0, 1, 12, 13)
	e.load(4, 2, 3, 12, 13)
	e.add(4, 0, 2) // t1 = x1 + x4
	e.sub(0, 0, 2) // t2 = x1 − x4
	e.add(5, 1, 3)
	e.sub(1, 1, 3) // t1 (4,5), t2 (0,1)
	e.load(2, 2, 3, 12, 13)
	e.load(3, 6, 7, 12, 13)
	e.add(8, 2, 6) // t3 = x2 + x3
	e.sub(2, 2, 6) // t4 = x2 − x3
	e.add(9, 3, 7)
	e.sub(3, 3, 7)          // t3 (8,9), t4 (2,3)
	e.load(0, 6, 7, 10, 11) // a (6,7)
	e.add(10, 6, 4)
	e.add(10, 10, 8) // y0 = (a + t1) + t3
	e.add(11, 7, 5)
	e.add(11, 11, 9)
	e.store(0, 10, 11, 12, 13)
	// r1 (10,11) = a + (c51·t1 + c52·t3).
	e.mulc(10, 4, e.c.SplitK.C51)
	e.mulc(12, 8, e.c.SplitK.C52)
	e.add(10, 10, 12)
	e.add(10, 6, 10)
	e.mulc(11, 5, e.c.SplitK.C51)
	e.mulc(12, 9, e.c.SplitK.C52)
	e.add(11, 11, 12)
	e.add(11, 7, 11)
	// r2 (12,13) = a + (c52·t1 + c51·t3).
	e.mulc(12, 4, e.c.SplitK.C52)
	e.mulc(14, 8, e.c.SplitK.C51)
	e.add(12, 12, 14)
	e.add(12, 6, 12)
	e.mulc(13, 5, e.c.SplitK.C52)
	e.mulc(14, 9, e.c.SplitK.C51)
	e.add(13, 13, 14)
	e.add(13, 7, 13)
	// z1 (4,5) = s51·t2 + s52·t4, z2 (6,7) = s52·t2 − s51·t4.
	e.mulc(4, 0, e.c.SplitK.S51)
	e.mulc(8, 2, e.c.SplitK.S52)
	e.add(4, 4, 8)
	e.mulc(5, 1, e.c.SplitK.S51)
	e.mulc(9, 3, e.c.SplitK.S52)
	e.add(5, 5, 9)
	e.mulc(6, 0, e.c.SplitK.S52)
	e.mulc(8, 2, e.c.SplitK.S51)
	e.sub(6, 6, 8)
	e.mulc(7, 1, e.c.SplitK.S52)
	e.mulc(9, 3, e.c.SplitK.S51)
	e.sub(7, 7, 9)
	e.rotAdd(0, 1, 2, 3, 10, 11, 4, 5)   // y1 (0,1), y4 (2,3) = r1 ± rotS(z1)
	e.rotAdd(8, 9, 14, 15, 12, 13, 6, 7) // y2 (8,9), y3 (14,15) = r2 ± rotS(z2)
	t := [4]int{4, 5, 6, 7}
	e.twStore(0, 1, 1, t, first)
	e.twStore(8, 9, 2, t, first)
	e.twStore(14, 15, 3, t, first)
	e.twStore(2, 3, 4, t, first)
}

// The radix-8 body spills a1 and a3 (the odd half's sums) to its frame
// while the even half runs: sixteen registers hold the odd half's four
// results and the even half's eight inputs only without them.
const splitFrame8 = 128

// h8 multiplies a register by √2/2 in place.
func (e splitEmit) h8(r int) { e.raw("VMULPD %d(R14), %s, %s", e.c.SplitK.H, y(r), y(r)) }

// hpm is the FMA kernels' (e.c.FMA) a ± h·s, h = √2/2, with the product
// fused: plus = fma(h, s, a), then a = fma(−h, s, a) in place (VFNMADD).
func (e splitEmit) hpm(plus, a, s int) {
	e.raw("VMOVAPD %s, %s", y(a), y(plus))
	e.raw("VFMADD231PD %d(R14), %s, %s", e.c.SplitK.H, y(s), y(plus))
	e.raw("VFNMADD231PD %d(R14), %s, %s", e.c.SplitK.H, y(s), y(a))
}

// neg flips a register's signs in place: −x, exactly.
func (e splitEmit) neg(r int) { e.raw("VXORPD %d(R14), %s, %s", e.c.SplitK.Neg, y(r), y(r)) }

// body8 is bfly8 (stockham.go's pass8) on split registers.
func (e splitEmit) body8(first bool) {
	// Odd half. x1 (0,1), x5 (2,3), then x3 (3,5), x7 (6,7).
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
	e.raw("VMOVUPD %s, 0(SP)", y(7))
	e.raw("VMOVUPD %s, 32(SP)", y(8))
	e.raw("VMOVUPD %s, 64(SP)", y(4))
	e.raw("VMOVUPD %s, 96(SP)", y(2))
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
		e.raw("VMOVAPD %s, %s", y(2), y(0))
	}
	if !e.c.FMA {
		e.h8(3)
		e.h8(5)
		e.h8(6)
		e.h8(0) // a5 (3,5), a7 (6,0)
	}
	// Even half. x0 (1,2), x4 (4,7), then x2 (7,9), x6 (10,11).
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
	if e.c.FMA {
		e.hpm(7, 10, 3)
		e.hpm(9, 13, 5)
	} else {
		e.add(7, 10, 3)
		e.sub(10, 10, 3)
		e.add(9, 13, 5)
		e.sub(13, 13, 5)
	}
	e.twStore(7, 9, 1, t, first)
	e.twStore(10, 13, 5, t, first)
	// y3, y7 = a6 ± a7.
	t = [4]int{3, 5, 10, 13}
	if e.c.FMA {
		e.hpm(7, 1, 6)
		e.hpm(9, 2, 0)
	} else {
		e.add(7, 1, 6)
		e.sub(1, 1, 6)
		e.add(9, 2, 0)
		e.sub(2, 2, 0)
	}
	e.twStore(7, 9, 3, t, first)
	e.twStore(1, 2, 7, t, first)
	// y0, y4 = a0 ± a1.
	t = [4]int{0, 1, 2, 3}
	e.raw("VADDPD 0(SP), %s, %s", y(11), y(7))
	e.raw("VSUBPD 0(SP), %s, %s", y(11), y(11))
	e.raw("VADDPD 32(SP), %s, %s", y(12), y(9))
	e.raw("VSUBPD 32(SP), %s, %s", y(12), y(12))
	e.store(0, 7, 9, 5, 6)
	e.twStore(11, 12, 4, t, first)
	// y2, y6 = a2 ± rotS(a3).
	e.raw("VMOVUPD 64(SP), %s", y(6))
	e.raw("VMOVUPD 96(SP), %s", y(0))
	e.rotAdd(7, 9, 8, 4, 8, 4, 6, 0)
	t = [4]int{1, 2, 3, 5}
	e.twStore(7, 9, 2, t, first)
	e.twStore(8, 4, 6, t, first)
}

func (e splitEmit) advance(bytes int) {
	regs := []string{"AX", "BX"}
	if e.r > 4 {
		regs = append(regs, "SI", "DI")
	}
	for _, r := range regs {
		e.raw("ADDQ $%d, %s", bytes, r)
	}
}

// Split emits name(cc, ch, tw *complex128, k *float64, ido, l1 int), one
// split-layout pass kernel of radix r (3, 4, 5 or 8, see above) reading cc
// split when inS and interleaved otherwise, writing ch split when outS, in
// the direction inverse selects. k is the split constants table
// (Kernels.SplitK). ido is a multiple of four (the caller checks), so every
// block is whole groups of four points and there is no tail.
func (c *Kernels) Split(f *emit.File, name string, r int, inS, outS, inverse bool) {
	check("Split", r, 3, 4, 5, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	frame := 0
	if r == 8 {
		frame = splitFrame8
	}
	b := asm.NewFunc(name, sig, frame)
	e := splitEmit{c: c, b: b, r: r, inS: inS, outS: outS, inverse: inverse}
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
		Raw("SHRQ $6, R9") // groups = ido/4
	e.body(true)
	e.advance(64)
	b.Raw("ADDQ $%d, R10", (r-1)*64).
		Raw("DECQ R9").
		Raw("gloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ knext")
	e.body(false)
	e.advance(64)
	b.Raw("ADDQ $%d, R10", (r-1)*64).
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
