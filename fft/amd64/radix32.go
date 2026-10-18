package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Radix-32 passes with fused products (go-fft Round 42).
//
// The butterfly is the 4×8 split of go-fft's bfly32FMA: four radix-8
// butterflies (bfly8FMA) over the inputs n2 + 4·n1 (stage 1), the internal
// twiddles W32^(n2·k1) as one product and one fused multiply-add per part
// (radix16RotFMA: re = fma(c, ar, −(ss·ai)), im = fma(c, ai, ss·ar), with
// W32^e = c + i·ss), W32^8 = s·i as the exact rotation rotS, then eight
// radix-4 butterflies (bfly4) over V[0..3][k1] (stage 2), whose outputs
// k1 + 8·k2 take the pass's twiddles (cmulFMA) unless the pass is a final
// one. Sixteen vector registers hold one radix-8 butterfly and its scratch,
// not the 32 values between the stages, so they go through memory once
// more, through the OUTPUT (go-fft Round 38's layout): V[n2][k1] is stored
// to output slot k1 + 8·n2, and stage 2's butterfly k1 reads slots k1,
// k1+8, k1+16, k1+24 and writes its outputs to those same four slots. A
// Stockham pass never runs in place, so no input still to be read is
// overwritten. Only registers 0 to 15 are written at either width: at 512
// bits, values left in Z16..Z31 slowed the code that ran after the kernel
// (Split512).
//
// The internal twiddles' constants are the cosines C_k = cos(kπ/16), k = 1
// .. 7, one row each from SplitConsts.R32 (split kernels) or Consts.R32's
// per-exponent rows (interleaved). In the split kernels the direction is
// compiled in: W32^e's cosine and sine are ±C_p and ±C_q, and the signs,
// the direction's included, choose among VFMADD, VFMSUB, VFNMADD and
// VFNMSUB, which form exactly the fused value with the signed constant
// (−(a·b) is (−a)·b in IEEE 754).

// r32Es are the exponents e = n2·k1 of the internal twiddles that are
// neither 1 nor s·i, in the order of their rows in the interleaved
// constants table (Consts.R32: a row of cos θ then a row [−ss, ss, …],
// ss = s·sin θ, θ = 2π·e/32, per exponent).
var r32Es = []int{1, 2, 3, 4, 5, 6, 7, 9, 10, 12, 14, 15, 18, 21}

// r32Trig gives W32^e = cos θ + i·sin θ (forward sign excluded), θ = π·e/16,
// as ±C_p and ±C_q (C_k = cos(kπ/16), 1 <= k <= 7): cos θ = cs·C_p, sin θ =
// ss·C_q. e is one of r32Es.
func r32Trig(e int) (p, cs, q, ss int) {
	switch {
	case e < 8:
		return e, 1, 8 - e, 1
	case e < 16:
		return 16 - e, -1, e - 8, 1
	}
	return e - 16, -1, 24 - e, -1
}

// r32SplitConst is the offset of the row of C_k in the split table.
func (c *Kernels) r32SplitConst(k int) int { return c.SplitK.R32 + 32*(k-1) }

// r32KRows returns the offsets (32-byte rows) of the cosine and ss rows of
// W32^e in the interleaved table.
func (c *Kernels) r32KRows(e int) (kc, ks int) {
	for q, x := range r32Es {
		if x == e {
			return c.K.R32 + 64*q, c.K.R32 + 64*q + 32
		}
	}
	panic(fmt.Sprintf("fft/amd64: no radix-32 row for W32^%d", e))
}

// sxEmit emits split-layout code at 256 bits (four points per register) or
// 512 bits (eight).
type sxEmit struct {
	c         *Kernels
	b         *asm.Builder
	wide      bool
	inS, outS bool
	inverse   bool
	spill     int // frame offset of the radix-8 body's four spilled registers
}

func (e sxEmit) raw(format string, a ...any) { e.b.Raw(format, a...) }

func (e sxEmit) v(i int) string {
	if e.wide {
		return fmt.Sprintf("Z%d", i)
	}
	return fmt.Sprintf("Y%d", i)
}

// vb is a register's size in bytes; a group of points is 2·vb bytes (the
// real parts, then the imaginary parts).
func (e sxEmit) vb() int {
	if e.wide {
		return 64
	}
	return 32
}

func (e sxEmit) im(addr string) string { return fmt.Sprintf("%d%s", e.vb(), addr) }

// sp addresses spill slot q (0..3) of the radix-8 body.
func (e sxEmit) sp(q int) string { return fmt.Sprintf("%d(SP)", e.spill+q*e.vb()) }

// kop emits op with the constants row at off as its memory operand,
// broadcast from the row's first value at 512 bits.
func (e sxEmit) kop(op string, off, src, dst int) {
	if e.wide {
		op += ".BCST"
	}
	e.raw("%s %d(R14), %s, %s", op, off, e.v(src), e.v(dst))
}

func (e sxEmit) add(dst, a, b int) { e.raw("VADDPD %s, %s, %s", e.v(b), e.v(a), e.v(dst)) }
func (e sxEmit) sub(dst, a, b int) { e.raw("VSUBPD %s, %s, %s", e.v(b), e.v(a), e.v(dst)) }

// neg flips a register's signs in place, exactly.
func (e sxEmit) neg(r int) {
	if e.wide {
		e.kop("VPXORQ", e.c.SplitK.Neg, r, r)
		return
	}
	e.kop("VXORPD", e.c.SplitK.Neg, r, r)
}

// loadAt puts the points at addr, split (inS) or interleaved, into re, im.
func (e sxEmit) loadAt(addr string, inS bool, re, im, t0, t1 int) {
	if inS {
		e.raw("VMOVUPD %s, %s", addr, e.v(re))
		e.raw("VMOVUPD %s, %s", e.im(addr), e.v(im))
		return
	}
	e.raw("VMOVUPD %s, %s", addr, e.v(t0))
	e.raw("VMOVUPD %s, %s", e.im(addr), e.v(t1))
	e.raw("VUNPCKLPD %s, %s, %s", e.v(t1), e.v(t0), e.v(re))
	e.raw("VUNPCKHPD %s, %s, %s", e.v(t1), e.v(t0), e.v(im))
}

// storeAt writes re, im to addr, split (outS) or interleaved.
func (e sxEmit) storeAt(addr string, outS bool, re, im, t0, t1 int) {
	if outS {
		e.raw("VMOVUPD %s, %s", e.v(re), addr)
		e.raw("VMOVUPD %s, %s", e.v(im), e.im(addr))
		return
	}
	e.raw("VUNPCKLPD %s, %s, %s", e.v(im), e.v(re), e.v(t0))
	e.raw("VUNPCKHPD %s, %s, %s", e.v(im), e.v(re), e.v(t1))
	e.raw("VMOVUPD %s, %s", e.v(t0), addr)
	e.raw("VMOVUPD %s, %s", e.v(t1), e.im(addr))
}

// rotAdd is splitEmit.rotAdd: pr, pi = a + rotS(b); mr, mi = a − rotS(b).
func (e sxEmit) rotAdd(pr, pi, mr, mi, ar, ai, br, bi int) {
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

// hpm is a ± h·s with the products fused: plus = fma(h, s, a), a = fma(−h,
// s, a).
func (e sxEmit) hpm(plus, a, s int) {
	e.raw("VMOVAPD %s, %s", e.v(a), e.v(plus))
	e.kop("VFMADD231PD", e.c.SplitK.H, s, plus)
	e.kop("VFNMADD231PD", e.c.SplitK.H, s, a)
}

// bfly8 is Split's FMA radix-8 body (bfly8FMA) with its inputs read by load
// (input j into re, im with scratch t0, t1) and every output handed to out
// (output j in re, im, with scratch registers t) instead of stored.
func (e sxEmit) bfly8(load func(j, re, im, t0, t1 int), out func(re, im, j int, t [4]int)) {
	// Odd half. x1 (0,1), x5 (2,3), then x3 (3,5), x7 (6,7).
	load(1, 0, 1, 12, 13)
	load(5, 2, 3, 12, 13)
	e.add(4, 0, 2) // a1 = x1 + x5
	e.sub(0, 0, 2) // a5 = x1 − x5
	e.add(2, 1, 3)
	e.sub(1, 1, 3) // a1 (4,2), a5 (0,1)
	load(3, 3, 5, 12, 13)
	load(7, 6, 7, 12, 13)
	e.add(8, 3, 6) // a3 = x3 + x7
	e.sub(3, 3, 6) // a7 = x3 − x7
	e.add(6, 5, 7)
	e.sub(5, 5, 7) // a3 (8,6), a7 (3,5)
	e.add(7, 4, 8) // a1 + a3
	e.sub(4, 4, 8) // a1 − a3
	e.add(8, 2, 6)
	e.sub(2, 2, 6) // a1 (7,8), a3 (4,2), before rotS(a3)
	e.raw("VMOVUPD %s, %s", e.v(7), e.sp(0))
	e.raw("VMOVUPD %s, %s", e.v(8), e.sp(1))
	e.raw("VMOVUPD %s, %s", e.v(4), e.sp(2))
	e.raw("VMOVUPD %s, %s", e.v(2), e.sp(3))
	e.rotAdd(6, 9, 0, 1, 0, 1, 3, 5) // a5, a7 = a5 ± rotS(a7): a5 (6,9), a7 (0,1)
	if !e.inverse {
		// a5 = (a5r + a5i, a5i − a5r); a7 = (a7i − a7r, −a7r − a7i).
		e.add(3, 6, 9)
		e.sub(5, 9, 6)
		e.sub(6, 1, 0)
		e.neg(0)
		e.sub(0, 0, 1)
	} else {
		// a5 = (a5r − a5i, a5i + a5r); a7 = (−a7i − a7r, a7r − a7i).
		e.sub(3, 6, 9)
		e.add(5, 9, 6)
		e.sub(2, 0, 1)
		e.neg(1)
		e.sub(6, 1, 0)
		e.raw("VMOVAPD %s, %s", e.v(2), e.v(0))
	}
	// Even half. x0 (1,2), x4 (4,7), then x2 (7,9), x6 (10,11).
	load(0, 1, 2, 13, 14)
	load(4, 4, 7, 13, 14)
	e.add(8, 1, 4) // a0 = x0 + x4
	e.sub(1, 1, 4) // a4 = x0 − x4
	e.add(4, 2, 7)
	e.sub(2, 2, 7) // a0 (8,4), a4 (1,2)
	load(2, 7, 9, 13, 14)
	load(6, 10, 11, 13, 14)
	e.add(12, 7, 10) // a2 = x2 + x6
	e.sub(7, 7, 10)  // a6 = x2 − x6
	e.add(10, 9, 11)
	e.sub(9, 9, 11) // a2 (12,10), a6 (7,9)
	e.add(11, 8, 12)
	e.sub(8, 8, 12)
	e.add(12, 4, 10)
	e.sub(4, 4, 10)                    // a0 (11,12), a2 (8,4)
	e.rotAdd(10, 13, 1, 2, 1, 2, 7, 9) // a4, a6 = a4 ± rotS(a6): a4 (10,13), a6 (1,2)
	// y1, y5 = a4 ± h·a5.
	e.hpm(7, 10, 3)
	e.hpm(9, 13, 5)
	out(7, 9, 1, [4]int{3, 5, 14, 15})
	out(10, 13, 5, [4]int{3, 5, 14, 15})
	// y3, y7 = a6 ± h·a7.
	e.hpm(7, 1, 6)
	e.hpm(9, 2, 0)
	out(7, 9, 3, [4]int{3, 5, 10, 13})
	out(1, 2, 7, [4]int{3, 5, 10, 13})
	// y0, y4 = a0 ± a1.
	e.raw("VADDPD %s, %s, %s", e.sp(0), e.v(11), e.v(7))
	e.raw("VSUBPD %s, %s, %s", e.sp(0), e.v(11), e.v(11))
	e.raw("VADDPD %s, %s, %s", e.sp(1), e.v(12), e.v(9))
	e.raw("VSUBPD %s, %s, %s", e.sp(1), e.v(12), e.v(12))
	out(7, 9, 0, [4]int{0, 1, 2, 3})
	out(11, 12, 4, [4]int{0, 1, 2, 3})
	// y2, y6 = a2 ± rotS(a3).
	e.raw("VMOVUPD %s, %s", e.sp(2), e.v(6))
	e.raw("VMOVUPD %s, %s", e.sp(3), e.v(0))
	e.rotAdd(7, 9, 8, 4, 8, 4, 6, 0)
	out(7, 9, 2, [4]int{1, 2, 3, 5})
	out(8, 4, 6, [4]int{1, 2, 3, 5})
}

// twiddle32 multiplies (re, im) by W32^e in place, or into (t[0], t[1]),
// and returns the registers that hold the product: rotS for e = 8 (a sign
// flip, and the parts exchanged), else radix16RotFMA's product and fused
// multiply-add per part.
func (e sxEmit) twiddle32(re, im, ex int, t [4]int) (int, int) {
	if ex == 8 {
		// rotS(z) = (−s·zi, s·zr): forward (zi, −zr), inverse (−zi, zr).
		if !e.inverse {
			e.neg(re)
		} else {
			e.neg(im)
		}
		return im, re
	}
	p, cs, q, ss := r32Trig(ex)
	if !e.inverse {
		ss = -ss // the forward transform's W32^e = cos θ − i·sin θ
	}
	kp, kq := e.c.r32SplitConst(p), e.c.r32SplitConst(q)
	// re' = cs·Cp·ar − ss·(Cq·ai), im' = cs·Cp·ai + ss·(Cq·ar).
	reOp := map[[2]int]string{{1, 1}: "VFMSUB231PD", {1, -1}: "VFMADD231PD", {-1, 1}: "VFNMSUB231PD", {-1, -1}: "VFNMADD231PD"}[[2]int{cs, ss}]
	imOp := map[[2]int]string{{1, 1}: "VFMADD231PD", {1, -1}: "VFMSUB231PD", {-1, 1}: "VFNMADD231PD", {-1, -1}: "VFNMSUB231PD"}[[2]int{cs, ss}]
	e.kop("VMULPD", kq, im, t[0])
	e.kop(reOp, kp, re, t[0])
	e.kop("VMULPD", kq, re, t[1])
	e.kop(imOp, kp, im, t[1])
	return t[0], t[1]
}

// twStore multiplies (re, im) by the pass's twiddle j (split layout, 2·vb
// bytes per twiddle from R10) with cmulFMA's products and stores it to
// addr in the output layout; in the first group of a block lane 0 (point i
// = 0) keeps the unmultiplied value.
func (e sxEmit) twStore(re, im, j int, addr string, t [4]int, first bool) {
	off := (j - 1) * 2 * e.vb()
	e.raw("VMOVUPD %d(R10), %s", off, e.v(t[0]))        // wr
	e.raw("VMOVUPD %d(R10), %s", off+e.vb(), e.v(t[1])) // wi
	e.raw("VMULPD %s, %s, %s", e.v(t[1]), e.v(im), e.v(t[2]))
	e.raw("VFMSUB231PD %s, %s, %s", e.v(t[0]), e.v(re), e.v(t[2]))
	e.raw("VMULPD %s, %s, %s", e.v(t[1]), e.v(re), e.v(t[3]))
	e.raw("VFMADD231PD %s, %s, %s", e.v(t[0]), e.v(im), e.v(t[3]))
	if first {
		for _, p := range [][2]int{{re, t[2]}, {im, t[3]}} {
			if e.wide {
				e.raw("VBLENDMPD %s, %s, K1, %s", e.v(p[0]), e.v(p[1]), e.v(p[1]))
			} else {
				e.raw("VBLENDPD $1, %s, %s, %s", e.v(p[0]), e.v(p[1]), e.v(p[1]))
			}
		}
	}
	e.storeAt(addr, e.outS, t[2], t[3], t[0], t[1])
}

// r32In addresses stage 1's input n2 + 4·n1 from SI = P + n2·S and DI = SI
// + 16·S (CX = S, R12 = 3·S).
func r32In(n1 int) string {
	return [...]string{"(SI)", "(SI)(CX*4)", "(SI)(CX*8)", "(SI)(R12*4)", "(DI)", "(DI)(CX*4)", "(SI)(R12*8)", "(DI)(R12*4)"}[n1]
}

// r32Slot1 addresses stage 1's output slot k1 + 8·n2 from R8 = O + 8·n2·D
// and R9 = R8 + 4·D (DX = D, R13 = 3·D).
func r32Slot1(k1 int) string {
	return [...]string{"(R8)", "(R8)(DX*1)", "(R8)(DX*2)", "(R8)(R13*1)", "(R9)", "(R9)(DX*1)", "(R8)(R13*2)", "(R9)(R13*1)"}[k1]
}

// r32Slot2 addresses stage 2's slot k1 + 8·k2 from R8 = O + k1·D (R11 =
// 16·D).
func r32Slot2(k2 int) string {
	return [...]string{"(R8)", "(R8)(DX*8)", "(R8)(R11*1)", "(R8)(R13*8)"}[k2]
}

// r32Bases emits stage 1's base registers for n2: SI and DI (when the
// inputs are read from AX and CX), R8 and R9.
func r32Bases(b *asm.Builder, n2 int, inputs bool) {
	if inputs {
		b.Raw("%s", [...]string{"MOVQ AX, SI", "LEAQ (AX)(CX*1), SI", "LEAQ (AX)(CX*2), SI", "LEAQ (AX)(R12*1), SI"}[n2])
		b.Raw("LEAQ (SI)(CX*8), DI").Raw("LEAQ (DI)(CX*8), DI")
	}
	b.Raw("%s", [...]string{"MOVQ BX, R8", "LEAQ (BX)(DX*8), R8", "LEAQ (BX)(R11*1), R8", "LEAQ (BX)(R13*8), R8"}[n2])
	b.Raw("LEAQ (R8)(DX*4), R9")
}

// body emits one radix-32 butterfly over a group of points.
func (e sxEmit) body(first bool) {
	for n2 := 0; n2 < 4; n2++ {
		r32Bases(e.b, n2, true)
		load := func(n1, re, im, t0, t1 int) { e.loadAt(r32In(n1), e.inS, re, im, t0, t1) }
		e.bfly8(load, func(re, im, k1 int, t [4]int) {
			if n2 > 0 && k1 > 0 {
				re, im = e.twiddle32(re, im, n2*k1, t)
			}
			e.storeAt(r32Slot1(k1), true, re, im, 0, 0)
		})
	}
	e.raw("MOVQ BX, R8")
	for k1 := 0; k1 < 8; k1++ {
		if k1 > 0 {
			e.raw("ADDQ DX, R8")
		}
		// V0 (0,1) V1 (2,3) V2 (4,5) V3 (6,7), stored split by stage 1.
		for n2 := 0; n2 < 4; n2++ {
			e.loadAt(r32Slot2(n2), true, 2*n2, 2*n2+1, 0, 0)
		}
		e.add(8, 0, 4) // t2 = V0 + V2
		e.sub(0, 0, 4) // t1 = V0 − V2
		e.add(9, 1, 5)
		e.sub(1, 1, 5)
		e.add(4, 2, 6) // t3 = V1 + V3
		e.sub(2, 2, 6) // t4 = V1 − V3
		e.add(5, 3, 7)
		e.sub(3, 3, 7)
		e.add(6, 8, 4) // y0 = t2 + t3
		e.add(7, 9, 5)
		e.sub(8, 8, 4) // y2 = t2 − t3
		e.sub(9, 9, 5)
		e.rotAdd(4, 5, 0, 1, 0, 1, 2, 3) // y1, y3 = t1 ± rotS(t4)
		t := [4]int{10, 11, 12, 13}
		for k2, y := range [4][2]int{{6, 7}, {4, 5}, {8, 9}, {0, 1}} {
			if j := k1 + 8*k2; j > 0 {
				e.twStore(y[0], y[1], j, r32Slot2(k2), t, first)
			} else {
				e.storeAt(r32Slot2(k2), e.outS, y[0], y[1], t[0], t[1])
			}
		}
	}
}

// SplitR32 emits name(cc, ch, tw *complex128, k *float64, ido, l1 int), the
// split-layout radix-32 pass kernel with fused products (see above) at 256
// bits, or 512 when wide: Split's layouts (cc read split when inS, ch
// written split when outS), its direction compiled in, its twiddle tables
// (go-fft's StockhamSplitTwiddles, 256- or 512-bit), and the split
// constants table, which must hold the rows of SplitConsts.R32. ido is a
// positive multiple of four (eight when wide). The frame holds the block
// and group counters and the radix-8 body's four spilled registers. The
// kernels are FMA kernels only: Kernels.FMA must be set.
//
// Registers: AX = input block + i (P), BX = output + i (O), CX = S =
// ido·16, DX = D = l1·ido·16, R12 = 3·S, R13 = 3·D, R11 = 16·D, R10 =
// twiddle group cursor, R15 = twiddle base, R14 = constants; SI, DI, R8 and
// R9 address the inputs and slots of each stage (r32In, r32Slot1,
// r32Slot2).
func (c *Kernels) SplitR32(f *emit.File, name string, wide, inS, outS, inverse bool) {
	if !c.FMA {
		panic("fft/amd64: SplitR32 builds FMA kernels only")
	}
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	e := sxEmit{c: c, wide: wide, inS: inS, outS: outS, inverse: inverse, spill: 16}
	b := asm.NewFunc(name, sig, 16+4*e.vb())
	e.b = b
	group, shift := 2*e.vb(), 6
	if wide {
		shift = 7
		b.Raw("MOVQ $1, R9").Raw("KMOVB R9, K1") // lane 0: the i = 0 point
	}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("tw", "R15").LoadArg("k", "R14").
		LoadArg("ido", "CX").LoadArg("l1", "R8")
	b.Raw("MOVQ CX, DX").
		Raw("IMULQ R8, DX").
		Raw("SHLQ $4, CX"). // S
		Raw("SHLQ $4, DX"). // D
		Raw("LEAQ (CX)(CX*2), R12").
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("MOVQ DX, R11").
		Raw("SHLQ $4, R11"). // 16·D
		Raw("MOVQ R8, 0(SP)").
		Raw("kloop:").
		Raw("CMPQ 0(SP), $0").
		Raw("JEQ done").
		Raw("MOVQ R15, R10").
		Raw("MOVQ CX, R9").
		Raw("SHRQ $%d, R9", shift). // groups = ido/4 (ido/8)
		Raw("MOVQ R9, 8(SP)")
	step := func(first bool) {
		e.body(first)
		b.Raw("ADDQ $%d, AX", group).
			Raw("ADDQ $%d, BX", group).
			Raw("ADDQ $%d, R10", 31*group).
			Raw("DECQ 8(SP)")
	}
	step(true)
	b.Raw("gloop:").
		Raw("CMPQ 8(SP), $0").
		Raw("JEQ knext")
	step(false)
	b.Raw("JMP gloop").
		// P advanced by S over the block; the next block starts 32·S after
		// this one. O advanced by S, the next block's output start.
		Raw("knext:").
		Raw("MOVQ CX, SI").
		Raw("SHLQ $5, SI").
		Raw("SUBQ CX, SI").
		Raw("ADDQ SI, AX"). // 31·S
		Raw("DECQ 0(SP)").
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// ---------------------------------------------------------------------------
// The final radix-32 pass (ido == 1), interleaved, with fused products.

// r32Bfly8 emits stage 1's radix-8 butterfly over the inputs n2 + 4·n1 of
// the block pair (or block) the register holds: skEmit.body's radix-8
// sequence with the FMA kernels' fused √2/2 products (bfly8FMA), its outputs
// y0..y7 left in registers 4, 10, 12, 8, 0, 3, 1, 2.
func (e skEmit) r32Bfly8(n2 int) [8]int {
	in := func(dst, n1 int) { e.in(dst, n2+4*n1) }
	in(1, 1)
	in(5, 5)
	e.add(8, 1, 5) // a1
	e.sub(9, 1, 5) // a5
	in(3, 3)
	in(7, 7)
	e.add(10, 3, 7) // a3
	e.sub(11, 3, 7) // a7
	e.add(12, 8, 10)
	e.sub(10, 8, 10) // a1, a3 = a1+a3, a1-a3
	e.rot(10, 10)
	e.rot(11, 11)
	e.add(8, 9, 11)
	e.sub(9, 9, 11) // a5, a7 = a5+a7, a5-a7
	e.rot(11, 8)
	e.add(8, 8, 11) // a5 + rotS(a5)
	e.rot(11, 9)
	e.sub(9, 11, 9) // rotS(a7) − a7
	in(0, 0)
	in(4, 4)
	e.add(1, 0, 4) // a0
	e.sub(2, 0, 4) // a4
	in(0, 2)
	in(4, 6)
	e.add(3, 0, 4) // a2
	e.sub(5, 0, 4) // a6
	e.add(0, 1, 3)
	e.sub(1, 1, 3) // a0, a2 = a0+a2, a0-a2
	e.rot(5, 5)
	e.add(3, 2, 5)
	e.sub(2, 2, 5) // a4, a6 = a4+a6, a4-a6
	// a0=0 a2=1 a4=3 a6=2 a1=12 a3=10 a5=8 a7=9
	e.add(4, 0, 12)  // y0
	e.sub(0, 0, 12)  // y4
	e.add(12, 1, 10) // y2
	e.sub(1, 1, 10)  // y6
	e.hpm(10, 3, 8)  // y1, y5
	e.hpm(8, 2, 9)   // y3, y7
	return [8]int{4, 10, 12, 8, 0, 3, 1, 2}
}

// r32Last emits one final radix-32 butterfly over the blocks the register
// width holds: stage 1 into the output slots, the internal twiddles
// (rotS for W32^8, else rotC's fused form), stage 2 from the slots.
func (e skEmit) r32Last() {
	for n2 := 0; n2 < 4; n2++ {
		r32Bases(e.b, n2, false)
		y := e.r32Bfly8(n2)
		for k1 := 0; k1 < 8; k1++ {
			r := y[k1]
			switch ex := n2 * k1; {
			case ex == 8:
				e.rot(r, r)
			case ex > 0:
				kc, ks := e.c.r32KRows(ex)
				e.rotC(r, r, kc, ks, 13, 14)
			}
			e.st(r, r32Slot1(k1))
		}
	}
	e.raw("MOVQ BX, R8")
	for k1 := 0; k1 < 8; k1++ {
		if k1 > 0 {
			e.raw("ADDQ DX, R8")
		}
		in := [4]int{0, 1, 2, 3}
		for n2 := 0; n2 < 4; n2++ {
			e.ld(in[n2], r32Slot2(n2))
		}
		z := [4]int{8, 9, 10, 11}
		e.bfly4r(in, [4]int{4, 5, 6, 7}, z)
		for k2 := 0; k2 < 4; k2++ {
			e.st(z[k2], r32Slot2(k2))
		}
	}
}

// LastR32 emits name(cc, ch *complex128, k *float64, l1 int), the final
// radix-32 pass (ido == 1) with fused products: two blocks per YMM register
// as Last runs the other radices, an odd l1 finished by one 128-bit step.
// The constants table is the interleaved one (Kernels.K), which must hold
// the rows of Consts.R32. FMA kernels only: Kernels.FMA must be set.
//
// Registers: AX = the block pair's inputs, BX = its first output, DX = D =
// l1·16, R13 = 3·D, R11 = 16·D, R14 = constants, R10 = pairs left; R8 and
// R9 address the slots (r32Slot1, r32Slot2).
func (c *Kernels) LastR32(f *emit.File, name string) {
	if !c.FMA {
		panic("fft/amd64: LastR32 builds FMA kernels only")
	}
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	y := skEmit{c: c, b: b, w: "Y", last: 32}
	x := skEmit{c: c, b: b, w: "X", last: 32}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").LoadArg("l1", "DX")
	b.Raw("MOVQ DX, R10").
		Raw("SHRQ $1, R10"). // block pairs
		Raw("SHLQ $4, DX").  // D
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("MOVQ DX, R11").
		Raw("SHLQ $4, R11"). // 16·D
		Raw("loop:").
		Raw("TESTQ R10, R10").
		Raw("JZ tail")
	y.r32Last()
	b.Raw("ADDQ $%d, AX", 32*32).
		Raw("ADDQ $32, BX").
		Raw("DECQ R10").
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ $16, DX"). // l1 odd
		Raw("JZ done")
	x.r32Last()
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
