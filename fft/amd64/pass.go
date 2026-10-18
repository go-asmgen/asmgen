package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Stockham pass kernels (go-fft's stockham.go, radices 2/3/4/5/8).
//
// A pass kernel name(cc, ch, tw *complex128, k *float64, ido, l1 int) runs one whole
// Stockham pass: for every block k < l1 and every point i < ido it reads the r
// inputs cc[i+ido·(j+r·k)], applies the size-r butterfly, multiplies output j
// >= 1 by its twiddle and writes ch[i+ido·(k+l1·j)]. Two points (i, i+1) share
// each YMM register; an odd ido finishes with one 128-bit step. ido >= 2.
//
// tw is the pass's twiddle table extended to i = 0, laid out group by group
// (see StockhamTwiddles and skEmit.tw): the points are walked in groups of
// four (two pairs), then one pair and one single point for ido mod 4, and each
// group's r-1 twiddle runs are stored together. The scalar oracle does NOT multiply at i = 0, and a
// multiply by 1 is not exact on signed zeros, so the first pair of every block
// multiplies both lanes and then takes lane pair 0 back from the unmultiplied
// value (VBLENDPD $3) — bit-identical, not merely equal.
//
// k points at the constants table (32-byte rows, laid out as Kernels.K says):
// row Sign is the ±i rotation sign mask, which carries the transform
// direction, so one kernel serves both. Every operation mirrors stockham.go's scalar arithmetic
// in the same order with separately rounded VMULPD/VADDPD/VADDSUBPD (no FMA),
// matching the GOAMD64=v1 oracle bit for bit; the rotations are exact lane
// swaps and sign flips.
//
// Registers: AX = input block + i (P), SI = P + 4·S, BX = output + i (O), DI =
// O + 4·OS, R10 = twiddle group cursor, CX = S = ido·16, DX = OS = l1·ido·16,
// R12 = 3·S, R13 = 3·OS, R8 = blocks left, R9 = groups left, R14 = constants,
// R15 = twiddle table base.

type skEmit struct {
	c    *Kernels
	b    *asm.Builder
	w    string // "Z" (four points), "Y" (two) or "X" (one, the odd tail)
	last int    // radix of a final-pass kernel (ido == 1), 0 for the others
	wide bool   // inside an AVX-512 kernel: 64-byte constant rows
	// g is the size, in points, of the twiddle group the body reads (see
	// StockhamTwiddles): 4, 2 or 1. half is 1 for the second pair of a
	// four-point group walked as two pairs, 0 otherwise.
	g, half int
	// notw stores every output without a twiddle (the i = 0 point of a
	// batched pass); bcast multiplies by one twiddle per output broadcast to
	// every lane (the other points of a batched pass), read from (j-1)·16(R10).
	notw, bcast bool
	// dup reads the radix-16 twiddle table, whose twiddles are stored
	// pre-duplicated: (wr, wr) and (wi, wi) (see radix16Twiddles).
	dup bool
	// addr, when set, addresses input (out false) or output stream j in
	// place of skIn and skOut: the radix-10/15/20 kernels (Round 26) have
	// more streams than those reach.
	addr func(out bool, j int) string
}

// inAddr and outAddr address input and output stream j.
func (e skEmit) inAddr(j int) string {
	if e.addr != nil {
		return e.addr(false, j)
	}
	return skIn(j)
}

func (e skEmit) outAddr(j int) string {
	if e.addr != nil {
		return e.addr(true, j)
	}
	return skOut(j)
}

// k scales a constants-table offset (given for 32-byte rows) to the table the
// kernel reads: the AVX-512 table's rows are 64 bytes.
func (e skEmit) k(off int) int {
	if e.wide {
		return 2 * off
	}
	return off
}

// perm emits a lane permute within each complex: "lo" broadcasts the real
// part, "hi" the imaginary part, "swap" exchanges them. The immediates differ
// with the register width (one bit per lane).
func (e skEmit) perm(kind string, src, dst int) {
	imm := map[string]string{"lo": "$0", "hi": "$15", "swap": "$5"}[kind]
	if e.w == "Z" {
		imm = map[string]string{"lo": "$0x00", "hi": "$0xFF", "swap": "$0x55"}[kind]
	}
	e.raw("VPERMILPD %s, %s, %s", imm, e.v(src), e.v(dst))
}

// xork flips signs by a constants row: VPXORQ for ZMM (VXORPD on ZMM would
// need AVX512DQ), VXORPD otherwise.
func (e skEmit) xork(off, r int) {
	op := "VXORPD"
	if e.w == "Z" {
		op = "VPXORQ"
	}
	e.raw("%s %d(R14), %s, %s", op, e.k(off), e.v(r), e.v(r))
}

func (e skEmit) v(i int) string { return fmt.Sprintf("%s%d", e.w, i) }

func (e skEmit) raw(format string, a ...any) { e.b.Raw(format, a...) }

// skIn and skOut address input and output stream j. Streams 8..15 exist only
// in the radix-16 kernels, whose third and fourth base registers (R8, R9 in,
// R11, R15 out) the other kernels use for something else.
func skIn(j int) string {
	return [...]string{"(AX)", "(AX)(CX*1)", "(AX)(CX*2)", "(AX)(R12*1)", "(SI)", "(SI)(CX*1)", "(SI)(CX*2)", "(SI)(R12*1)",
		"(R8)", "(R8)(CX*1)", "(R8)(CX*2)", "(R8)(R12*1)", "(R9)", "(R9)(CX*1)", "(R9)(CX*2)", "(R9)(R12*1)"}[j]
}

func skOut(j int) string {
	return [...]string{"(BX)", "(BX)(DX*1)", "(BX)(DX*2)", "(BX)(R13*1)", "(DI)", "(DI)(DX*1)", "(DI)(DX*2)", "(DI)(R13*1)",
		"(R11)", "(R11)(DX*1)", "(R11)(DX*2)", "(R11)(R13*1)", "(R15)", "(R15)(DX*1)", "(R15)(DX*2)", "(R15)(R13*1)"}[j]
}

// tw addresses twiddle j (j >= 1) of the points the body runs on. The table
// holds the twiddles group by group, in the order the kernel walks the points:
// a group of g points stores its r-1 twiddle runs one after the other, g
// entries each, so twiddle j sits (j-1)·16·g bytes into the group (plus 32 for
// the second pair of a four-point group). R10 points at the current group. A
// pass then reads its twiddles as one sequential stream instead of r-1 runs
// ido entries apart.
func (e skEmit) tw(j int) string {
	return fmt.Sprintf("%d(R10)", (j-1)*16*e.g+32*e.half)
}

// twNext moves R10 past the current group.
func (e skEmit) twNext(r int) {
	if e.dup {
		e.raw("ADDQ $%d, R10", (r-1)*32*e.g)
		return
	}
	e.raw("ADDQ $%d, R10", (r-1)*16*e.g)
}

// twDup addresses the pre-duplicated halves of twiddle j: a group of g points
// stores, per j, the g pairs (wr, wr) then the g pairs (wi, wi).
func (e skEmit) twDup(j int) (wr, wi string) {
	at := (j-1)*32*e.g + 32*e.half
	return fmt.Sprintf("%d(R10)", at), fmt.Sprintf("%d(R10)", at+16*e.g)
}

func (e skEmit) ld(dst int, addr string) { e.raw("VMOVUPD %s, %s", addr, e.v(dst)) }

// in loads input j. In a final pass (ido == 1) a register holds blocks k and
// k+1 instead of points i and i+1: input j of block k is 16·j bytes into the
// block pair, and of block k+1 16·(r+j).
func (e skEmit) in(dst, j int) {
	if e.last == 0 {
		e.ld(dst, e.inAddr(j))
		return
	}
	e.raw("VMOVUPD %d(AX), X%d", 16*j, dst)
	switch e.w {
	case "Y":
		e.raw("VINSERTF128 $1, %d(AX), Y%d, Y%d", 16*(e.last+j), dst, dst)
	case "Z":
		// Four blocks: VINSERTF32X4 is AVX512F (its 64x2 twin needs DQ) and
		// moves the same 128 bits.
		for q := 1; q < 4; q++ {
			e.raw("VINSERTF32X4 $%d, %d(AX), Z%d, Z%d", q, 16*(q*e.last+j), dst, dst)
		}
	}
}
func (e skEmit) st(src int, addr string) { e.raw("VMOVUPD %s, %s", e.v(src), addr) }
func (e skEmit) add(dst, a, b int)       { e.raw("VADDPD %s, %s, %s", e.v(b), e.v(a), e.v(dst)) }
func (e skEmit) sub(dst, a, b int)       { e.raw("VSUBPD %s, %s, %s", e.v(b), e.v(a), e.v(dst)) }
func (e skEmit) mulk(dst, a int, off int) {
	e.raw("VMULPD %d(R14), %s, %s", e.k(off), e.v(a), e.v(dst))
}

// hpm is radix 8's a ± h·s, h = √2/2: plus = a + s, a = a − s for the
// default kernels, which scaled s by h before, and the fused products for
// the FMA kernels (e.c.FMA): plus = fma(h, s, a), a = fma(−h, s, a).
func (e skEmit) hpm(plus, a, s int) {
	if !e.c.FMA {
		e.add(plus, a, s)
		e.sub(a, a, s)
		return
	}
	e.raw("VMOVAPD %s, %s", e.v(a), e.v(plus))
	e.raw("VFMADD231PD %d(R14), %s, %s", e.k(e.c.K.H), e.v(s), e.v(plus))
	e.raw("VFNMADD231PD %d(R14), %s, %s", e.k(e.c.K.H), e.v(s), e.v(a))
}

// rot: dst = rotS(src) — swap re/im, then flip the sign the direction selects.
func (e skEmit) rot(dst, src int) {
	e.perm("swap", src, dst)
	e.xork(e.c.K.Sign, dst)
}

// twStore multiplies y by twiddle j and stores it to output j. In the first
// pair of a block the i = 0 lane pair keeps the unmultiplied y. A final pass
// has no twiddles: it stores y.
//
// The twiddle's halves are taken straight from memory: VMOVDDUP gives
// (wr, wr) per point as a plain load, and VPERMILPD with a memory operand
// gives (wi, wi), so the multiply takes two shuffles (that one and swapping
// y) where broadcasting y's halves and swapping w took three, all on the one
// shuffle port Intel cores have. It computes (yr·wr − yi·wi, yi·wr + yr·wi):
// the same products as the scalar oracle's (yr·wr − yi·wi, yr·wi + yi·wr),
// its imaginary sum taken in the other order, which IEEE addition makes
// exact. Loading (wi, wi) with a second VMOVDDUP at w+8 saves the other
// shuffle but straddles a cache line every other pair: it measured 8% slower
// at 2048 and 4096 on Zen 3, where this form is within 2% of the old one or
// faster at every size (2026-10-04).
func (e skEmit) twStore(y, j, t1, t2, t3, dst int, first bool) {
	if e.last != 0 || e.notw {
		e.st(y, e.outAddr(j))
		return
	}
	if e.bcast {
		// One twiddle for the whole register: its halves broadcast straight
		// from memory, VBROADCASTSD (VMOVDDUP at 128 bits), which are plain
		// loads, so the product costs one shuffle, the swap of y.
		ld := "VBROADCASTSD"
		if e.w == "X" {
			ld = "VMOVDDUP"
		}
		e.raw("%s %d(R10), %s", ld, (j-1)*16, e.v(t1))
		e.raw("%s %d(R10), %s", ld, (j-1)*16+8, e.v(t2))
		e.perm("swap", y, t3)
		if e.c.FMA {
			e.fmaProduct(y, e.v(t1), e.v(t2), t3, dst)
			e.st(dst, e.outAddr(j))
			return
		}
		e.raw("VMULPD %s, %s, %s", e.v(y), e.v(t1), e.v(t1))
		e.raw("VMULPD %s, %s, %s", e.v(t3), e.v(t2), e.v(t2))
		e.raw("VADDSUBPD %s, %s, %s", e.v(t2), e.v(t1), e.v(dst))
		e.st(dst, e.outAddr(j))
		return
	}
	switch {
	case e.c.FMA && e.dup:
		wr, wi := e.twDup(j)
		e.perm("swap", y, t3)
		e.fmaProduct(y, wr, wi, t3, dst)
	case e.c.FMA:
		hi := map[string]string{"X": "$3", "Y": "$15", "Z": "$0xFF"}[e.w]
		e.raw("VMOVDDUP %s, %s", e.tw(j), e.v(t1))
		e.raw("VPERMILPD %s, %s, %s", hi, e.tw(j), e.v(t2))
		e.perm("swap", y, t3)
		e.fmaProduct(y, e.v(t1), e.v(t2), t3, dst)
	case e.dup:
		// The halves come pre-duplicated: each product takes its twiddle
		// half as a memory operand, and swapping y is the only shuffle.
		wr, wi := e.twDup(j)
		e.perm("swap", y, t3)
		e.raw("VMULPD %s, %s, %s", wr, e.v(y), e.v(t1))
		e.raw("VMULPD %s, %s, %s", wi, e.v(t3), e.v(t2))
	default:
		hi := map[string]string{"X": "$3", "Y": "$15", "Z": "$0xFF"}[e.w]
		e.raw("VMOVDDUP %s, %s", e.tw(j), e.v(t1))
		e.raw("VPERMILPD %s, %s, %s", hi, e.tw(j), e.v(t2))
		e.perm("swap", y, t3)
		e.raw("VMULPD %s, %s, %s", e.v(y), e.v(t1), e.v(t1))
		e.raw("VMULPD %s, %s, %s", e.v(t3), e.v(t2), e.v(t2))
	}
	switch {
	case e.c.FMA:
		// fmaProduct left the product in dst.
	case e.w == "Z":
		// No 512-bit VADDSUBPD: flip the real lanes' sign, then add.
		// x + (-y) is x - y exactly, so the rounding is VADDSUBPD's.
		e.xork(e.c.K.AddSub, t2)
		e.raw("VADDPD %s, %s, %s", e.v(t2), e.v(t1), e.v(dst))
	default:
		e.raw("VADDSUBPD %s, %s, %s", e.v(t2), e.v(t1), e.v(dst))
	}
	if first {
		if e.w == "Z" {
			// K1 = lanes 0–1: point i = 0 keeps the unmultiplied y.
			e.raw("VBLENDMPD %s, %s, K1, %s", e.v(y), e.v(dst), e.v(dst))
		} else {
			e.raw("VBLENDPD $3, %s, %s, %s", e.v(y), e.v(dst), e.v(dst))
		}
	}
	e.st(dst, e.outAddr(j))
}

// fmaProduct is the fused twiddle product of an FMA kernel (e.c.FMA) on
// interleaved registers: dst = y·w with w's real part duplicated in wr and its
// imaginary part in wi (registers or memory operands), sw holding y with its
// halves swapped. One multiply, (yi·wi, yr·wi), then VFMADDSUB231PD, which
// subtracts it from yr·wr in the real lane and adds it to yi·wr in the
// imaginary lane, each with one rounding:
//
//	re = fma(yr, wr, −(yi·wi)),  im = fma(yi, wr, yr·wi)
//
// go-fft's cmulFMA, the product every FMA kernel and its Go
// reference compute. VFMADDSUB231PD exists at 512 bits (AVX512F), so the
// AVX-512 kernels need no sign flip where VADDSUBPD has no 512-bit form.
func (e skEmit) fmaProduct(y int, wr, wi string, sw, dst int) {
	e.raw("VMULPD %s, %s, %s", wi, e.v(sw), e.v(dst))
	e.raw("VFMADDSUB231PD %s, %s, %s", wr, e.v(y), e.v(dst))
}

func (e skEmit) body(r int, first bool) {
	switch r {
	case 2:
		e.in(0, 0)
		e.in(1, 1)
		e.add(2, 0, 1)
		e.st(2, skOut(0))
		e.sub(3, 0, 1)
		e.twStore(3, 1, 5, 6, 7, 4, first)
	case 3:
		e.in(0, 0)
		e.in(1, 1)
		e.in(2, 2)
		e.add(3, 1, 2) // t1
		e.sub(4, 1, 2) // t2
		e.add(5, 0, 3)
		e.st(5, skOut(0))
		e.mulk(6, 3, e.c.K.Half)
		e.sub(6, 0, 6) // ca = x0 - 0.5·t1
		e.mulk(7, 4, e.c.K.Sin120)
		e.rot(7, 7) // cb
		e.add(1, 6, 7)
		e.sub(2, 6, 7)
		e.twStore(1, 1, 9, 10, 11, 8, first)
		e.twStore(2, 2, 9, 10, 11, 8, first)
	case 4:
		for j := 0; j < 4; j++ {
			e.in(j, j)
		}
		e.add(4, 0, 2) // t2
		e.sub(5, 0, 2) // t1
		e.add(6, 1, 3) // t3
		e.sub(7, 1, 3) // t4
		e.rot(7, 7)
		e.add(0, 4, 6)
		e.st(0, skOut(0))
		e.add(1, 5, 7)
		e.sub(2, 4, 6)
		e.sub(3, 5, 7)
		e.twStore(1, 1, 9, 10, 11, 8, first)
		e.twStore(2, 2, 9, 10, 11, 8, first)
		e.twStore(3, 3, 9, 10, 11, 8, first)
	case 5:
		e.in(0, 0)
		e.in(1, 1)
		e.in(2, 4)
		e.add(3, 1, 2) // t1
		e.sub(4, 1, 2) // t2
		e.in(1, 2)
		e.in(2, 3)
		e.add(5, 1, 2) // t3
		e.sub(6, 1, 2) // t4
		e.add(7, 0, 3)
		e.add(7, 7, 5)
		e.st(7, skOut(0))
		e.mulk(8, 3, e.c.K.C51)
		e.mulk(9, 5, e.c.K.C52)
		e.add(8, 8, 9)
		e.add(8, 0, 8) // r1
		e.mulk(9, 3, e.c.K.C52)
		e.mulk(10, 5, e.c.K.C51)
		e.add(9, 9, 10)
		e.add(9, 0, 9) // r2
		e.mulk(10, 4, e.c.K.S51)
		e.mulk(11, 6, e.c.K.S52)
		e.add(10, 10, 11)
		e.rot(10, 10) // i1
		e.mulk(11, 4, e.c.K.S52)
		e.mulk(12, 6, e.c.K.S51)
		e.sub(11, 11, 12)
		e.rot(11, 11)   // i2
		e.add(1, 8, 10) // y1
		e.sub(4, 8, 10) // y4
		e.add(2, 9, 11) // y2
		e.sub(3, 9, 11) // y3
		e.twStore(1, 1, 12, 13, 14, 15, first)
		e.twStore(2, 2, 12, 13, 14, 15, first)
		e.twStore(3, 3, 12, 13, 14, 15, first)
		e.twStore(4, 4, 12, 13, 14, 15, first)
	case 8:
		e.in(1, 1)
		e.in(5, 5)
		e.add(8, 1, 5) // a1
		e.sub(9, 1, 5) // a5
		e.in(3, 3)
		e.in(7, 7)
		e.add(10, 3, 7) // a3
		e.sub(11, 3, 7) // a7
		e.add(12, 8, 10)
		e.sub(10, 8, 10) // a1, a3 = a1+a3, a1-a3
		e.rot(10, 10)
		e.rot(11, 11)
		e.add(8, 9, 11)
		e.sub(9, 9, 11) // a5, a7 = a5+a7, a5-a7
		e.rot(11, 8)
		e.add(8, 8, 11)
		if !e.c.FMA {
			e.mulk(8, 8, e.c.K.H) // a5 = h·(a5 + rotS(a5))
		}
		e.rot(11, 9)
		e.sub(9, 11, 9)
		if !e.c.FMA {
			e.mulk(9, 9, e.c.K.H) // a7 = h·(rotS(a7) - a7)
		}
		e.in(0, 0)
		e.in(4, 4)
		e.add(1, 0, 4) // a0
		e.sub(2, 0, 4) // a4
		e.in(0, 2)
		e.in(4, 6)
		e.add(3, 0, 4) // a2
		e.sub(5, 0, 4) // a6
		e.add(0, 1, 3)
		e.sub(1, 1, 3) // a0, a2 = a0+a2, a0-a2
		e.rot(5, 5)
		e.add(3, 2, 5)
		e.sub(2, 2, 5) // a4, a6 = a4+a6, a4-a6
		// a0=0 a2=1 a4=3 a6=2 a1=12 a3=10 a5=8 a7=9
		e.add(4, 0, 12)
		e.st(4, skOut(0))
		e.sub(0, 0, 12)  // y4
		e.add(12, 1, 10) // y2
		e.sub(1, 1, 10)  // y6
		e.hpm(10, 3, 8)  // y1, y5
		e.hpm(8, 2, 9)   // y3, y7
		for _, yj := range [][2]int{{10, 1}, {12, 2}, {8, 3}, {0, 4}, {3, 5}, {1, 6}, {2, 7}} {
			e.twStore(yj[0], yj[1], 13, 14, 15, 4, first)
		}
	}
}

func (e skEmit) advance(bytes int) {
	for _, r := range []string{"AX", "SI", "BX", "DI"} {
		e.raw("ADDQ $%d, %s", bytes, r)
	}
}

// pass emits the radix-2, 3, 4, 5 and 8 AVX2 pass (see Pass).
func (c *Kernels) pass(f *emit.File, name string, r int) {
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	// A four-point twiddle group is walked as two pairs (q0, q1); then come a
	// two-point group (p) and a one-point group (x) for ido mod 4.
	q0 := skEmit{c: c, b: b, w: "Y", g: 4}
	q1 := skEmit{c: c, b: b, w: "Y", g: 4, half: 1}
	p := skEmit{c: c, b: b, w: "Y", g: 2}
	x := skEmit{c: c, b: b, w: "X", g: 1}
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
		Raw("JZ done").
		Raw("LEAQ (AX)(CX*4), SI").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("MOVQ R15, R10").
		Raw("MOVQ CX, R9").
		Raw("SHRQ $6, R9"). // four-point groups = ido/4
		Raw("TESTQ R9, R9").
		Raw("JZ firstpair")
	q0.body(r, true)
	q0.advance(32)
	q1.body(r, false)
	q1.advance(32)
	q0.twNext(r)
	b.Raw("DECQ R9").
		Raw("qloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ pair")
	q0.body(r, false)
	q0.advance(32)
	q1.body(r, false)
	q1.advance(32)
	q0.twNext(r)
	b.Raw("DECQ R9").
		Raw("JMP qloop").
		Raw("pair:").
		Raw("TESTQ $32, CX"). // ido & 2
		Raw("JZ single")
	p.body(r, false)
	p.advance(32)
	p.twNext(r)
	b.Raw("JMP single").
		Raw("firstpair:") // ido is 2 or 3: the first pair is the two-point group
	p.body(r, true)
	p.advance(32)
	p.twNext(r)
	b.Raw("single:").
		Raw("TESTQ $16, CX"). // ido & 1
		Raw("JZ knext")
	x.body(r, false)
	x.advance(16)
	b.Raw("knext:")
	// P advanced by S over the block; the next block starts r·S after this one.
	skNextBlock(b, r)
	// O advanced by S, which is exactly the next block's output start.
	b.Raw("DECQ R8").
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// last emits the radix-2, 3, 4, 5 and 8 AVX2 final pass (see Last): the final Stockham
// pass (ido == 1), where every block is a single point and there are no
// twiddles. It vectorizes across blocks instead of points — blocks k and k+1
// share each YMM register, their inputs gathered from the two adjacent r-point
// blocks, their outputs landing contiguous in each of the r output runs — and
// an odd l1 finishes with one 128-bit step. The butterflies are the ones the
// other passes use, so the result is again bit-identical to go-fft's
// pass{r}last.
func (c *Kernels) last(f *emit.File, name string, r int) {
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	y, x := skEmit{c: c, b: b, w: "Y", last: r}, skEmit{c: c, b: b, w: "X", last: r}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").LoadArg("l1", "DX")
	b.Raw("MOVQ DX, R9").
		Raw("SHRQ $1, R9"). // block pairs
		Raw("SHLQ $4, DX"). // OS
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("loop:").
		Raw("TESTQ R9, R9").
		Raw("JZ tail")
	y.body(r, false)
	b.Raw("ADDQ $%d, AX", 32*r).
		Raw("ADDQ $32, BX").
		Raw("ADDQ $32, DI").
		Raw("DECQ R9").
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ $16, DX"). // l1 odd
		Raw("JZ done")
	x.body(r, false)
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// knext emits the advance from one Stockham block to the next: P has moved
// by S over the block, and the next block starts r·S after this one.
func skNextBlock(b *asm.Builder, r int) {
	switch r {
	case 2:
		b.Raw("ADDQ CX, AX")
	case 3:
		b.Raw("LEAQ (AX)(CX*2), AX")
	case 4:
		b.Raw("ADDQ R12, AX")
	case 5:
		b.Raw("LEAQ (AX)(CX*4), AX")
	case 8:
		b.Raw("LEAQ (AX)(CX*4), AX").Raw("ADDQ R12, AX")
	}
}

// Pass512 emits name(cc, ch, tw *complex128, k *float64, ido, l1 int): Pass's
// radix-r pass (r = 2, 4 or 8) with four points per ZMM register. It needs ido >= 4; ido mod 4 is finished with one
// 256-bit and/or one 128-bit step, the same bodies the AVX2 kernel runs. The
// constants table has 64-byte rows (the offsets of Kernels.K doubled), and it
// holds the AddSub row. Every substitution for an
// instruction AVX-512 lacks at this width is exact: VADDSUBPD becomes a sign
// flip and an add, VXORPD becomes VPXORQ, the i = 0 blend uses opmask K1.
func (c *Kernels) Pass512(f *emit.File, name string, r int) {
	check("Pass512", r, 2, 4, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	z := skEmit{c: c, b: b, w: "Z", wide: true, g: 4}
	y := skEmit{c: c, b: b, w: "Y", wide: true, g: 2}
	x := skEmit{c: c, b: b, w: "X", wide: true, g: 1}
	b.Raw("MOVQ $3, R9").Raw("KMOVB R9, K1") // lanes 0–1: the i = 0 point
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("tw", "R15").LoadArg("k", "R14").
		LoadArg("ido", "CX").LoadArg("l1", "R8")
	b.Raw("MOVQ CX, DX").
		Raw("IMULQ R8, DX").
		Raw("SHLQ $4, CX").
		Raw("SHLQ $4, DX").
		Raw("LEAQ (CX)(CX*2), R12").
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("kloop:").
		Raw("TESTQ R8, R8").
		Raw("JZ done").
		Raw("LEAQ (AX)(CX*4), SI").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("MOVQ R15, R10")
	z.body(r, true)
	z.advance(64)
	z.twNext(r)
	b.Raw("MOVQ CX, R9").
		Raw("SHRQ $6, R9"). // quads = ido/4
		Raw("DECQ R9").
		Raw("iloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ pair")
	z.body(r, false)
	z.advance(64)
	z.twNext(r)
	b.Raw("DECQ R9").
		Raw("JMP iloop").
		Raw("pair:").
		Raw("TESTQ $32, CX"). // ido & 2
		Raw("JZ single")
	y.body(r, false)
	y.advance(32)
	y.twNext(r)
	b.Raw("single:").
		Raw("TESTQ $16, CX"). // ido & 1
		Raw("JZ knext")
	x.body(r, false)
	x.advance(16)
	b.Raw("knext:")
	skNextBlock(b, r)
	b.Raw("DECQ R8").
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// Last512 emits name(cc, ch *complex128, k *float64, l1 int): Last's final
// pass (r = 2, 4 or 8) with four blocks per ZMM register (VINSERTF32X4 gathers blocks k..k+3); l1 mod 4
// is finished with one 256-bit and/or one 128-bit step. The constants table
// is Pass512's.
func (c *Kernels) Last512(f *emit.File, name string, r int) {
	check("Last512", r, 2, 4, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	z, y, x := skEmit{c: c, b: b, w: "Z", last: r, wide: true}, skEmit{c: c, b: b, w: "Y", last: r, wide: true}, skEmit{c: c, b: b, w: "X", last: r, wide: true}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").LoadArg("l1", "DX")
	b.Raw("MOVQ DX, R9").
		Raw("SHRQ $2, R9"). // block quads
		Raw("SHLQ $4, DX"). // OS
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("loop:").
		Raw("TESTQ R9, R9").
		Raw("JZ pair")
	z.body(r, false)
	b.Raw("ADDQ $%d, AX", 64*r).
		Raw("ADDQ $64, BX").
		Raw("ADDQ $64, DI").
		Raw("DECQ R9").
		Raw("JMP loop").
		Raw("pair:").
		Raw("TESTQ $32, DX"). // l1 & 2
		Raw("JZ single")
	y.body(r, false)
	b.Raw("ADDQ $%d, AX", 32*r).
		Raw("ADDQ $32, BX").
		Raw("ADDQ $32, DI").
		Raw("single:").
		Raw("TESTQ $16, DX"). // l1 & 1
		Raw("JZ done")
	x.body(r, false)
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// lastRun emits LastRun's and LastRun512's kernel: the final pass (ido == 1)
// of go-fft's blocked schedule (stockham.go, cascadeBlocked), which writes
// straight into the transform's output instead of a local buffer. It runs
// runs·run blocks, the bodies the final-pass kernels run (Last and Last512,
// so the arithmetic is the
// same, operation for operation), reading the blocks contiguously; its
// outputs land in runs of run consecutive points, gap points apart, and
// output j sits os points after output j-1. run is a multiple of 4. The
// plain final-pass kernel is the case runs = 1, run = gap = os.
//
// Registers: AX = input, BX = output run cursor, DI = BX + 4·OS, DX = OS =
// os·16, R13 = 3·OS, R14 = constants, R8 = runs left, R9 = steps left in the
// run, R10 = steps per run, R11 = (gap - run)·16, the jump to the next run.
func (c *Kernels) lastRun(f *emit.File, name string, r int, wide bool) {
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "os", "runs", "run", "gap"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64, asm.Int64, asm.Int64}, nil, nil,
	)
	step, w := 2, "Y"
	if wide {
		step, w = 4, "Z"
	}
	b := asm.NewFunc(name, sig, 0)
	e := skEmit{c: c, b: b, w: w, last: r, wide: wide}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").LoadArg("os", "DX").
		LoadArg("runs", "R8").LoadArg("run", "R10").LoadArg("gap", "R11")
	b.Raw("SHLQ $4, DX"). // OS
				Raw("LEAQ (DX)(DX*2), R13").
				Raw("SUBQ R10, R11").
				Raw("SHLQ $4, R11"). // (gap - run)·16
				Raw("SHRQ $%d, R10", map[int]int{2: 1, 4: 2}[step]).
				Raw("rloop:").
				Raw("TESTQ R8, R8").
				Raw("JZ done").
				Raw("LEAQ (BX)(DX*4), DI").
				Raw("MOVQ R10, R9").
				Raw("sloop:").
				Raw("TESTQ R9, R9").
				Raw("JZ rnext")
	e.body(r, false)
	b.Raw("ADDQ $%d, AX", 16*step*r).
		Raw("ADDQ $%d, BX", 16*step).
		Raw("ADDQ $%d, DI", 16*step).
		Raw("DECQ R9").
		Raw("JMP sloop").
		Raw("rnext:").
		Raw("ADDQ R11, BX").
		Raw("DECQ R8").
		Raw("JMP rloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// strided emits Strided's and Strided512's kernel: Pass's (or Pass512's) pass
// over nb blocks of cnt points (cnt a positive multiple of 4), with every
// stride a parameter instead of a function of ido and l1. Point i of block b
// reads its r inputs sin points apart from cc + b·bin + i and writes its r
// outputs sout points apart from ch + b·bout + i, where the caller passes the
// block strides as din = (bin - cnt)·16 and dout = (bout - cnt)·16, the jump
// left after the block's points. Block b's twiddles start dtw bytes after
// block b-1's (StockhamTwiddles' layout, so a slice of a pass's table that
// starts at a point i0 with i0 mod 4 = 0 serves points i0, i0+1, ...). bf and
// br say whether the first point of block 0, and of the other blocks, is the
// pass's untwiddled point i = 0, which the kernel then keeps unmultiplied as
// the plain pass does (the blend); any other point is multiplied. The
// butterflies and the products are the plain pass's, so a pass split into
// strided pieces is bit-identical to the pass. go-fft runs two
// passes chunk by chunk with it (cascadePair), one reading the array and the
// other writing it, through a buffer that stays in cache.
//
// Registers as Pass, plus R11 = whether the block's first point is
// i = 0; R8 = blocks left, R9 = four-point groups left.
func (c *Kernels) strided(f *emit.File, name string, r int, wide bool) {
	names := []string{"cc", "ch", "tw", "k", "cnt", "nb", "sin", "sout", "din", "dout", "dtw", "bf", "br"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr}
	for range names[4:] {
		types = append(types, asm.Int64)
	}
	sig := asm.Layout(names, types, nil, nil)
	arg := func(n string) string { return fp(names, n) }
	b := asm.NewFunc(name, sig, 0)
	// One four-point group: a ZMM register, or two YMM pairs.
	group := func(first bool) {
		if wide {
			z := skEmit{c: c, b: b, w: "Z", wide: true, g: 4}
			z.body(r, first)
			z.advance(64)
			z.twNext(r)
			return
		}
		q0 := skEmit{c: c, b: b, w: "Y", g: 4}
		q1 := skEmit{c: c, b: b, w: "Y", g: 4, half: 1}
		q0.body(r, first)
		q0.advance(32)
		q1.body(r, false)
		q1.advance(32)
		q0.twNext(r)
	}
	if wide {
		b.Raw("MOVQ $3, R9").Raw("KMOVB R9, K1") // lanes 0–1: the i = 0 point
	}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("tw", "R15").LoadArg("k", "R14").
		LoadArg("sin", "CX").LoadArg("sout", "DX").LoadArg("nb", "R8").LoadArg("bf", "R11")
	b.Raw("SHLQ $4, CX").
		Raw("SHLQ $4, DX").
		Raw("LEAQ (CX)(CX*2), R12").
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("kloop:").
		Raw("TESTQ R8, R8").
		Raw("JZ done").
		Raw("LEAQ (AX)(CX*4), SI").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("MOVQ R15, R10").
		Raw("MOVQ %s, R9", arg("cnt")).
		Raw("SHRQ $2, R9").
		Raw("TESTQ R11, R11").
		Raw("JZ plain")
	group(true)
	b.Raw("JMP rest").
		Raw("plain:")
	group(false)
	b.Raw("rest:").
		Raw("DECQ R9").
		Raw("gloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ knext")
	group(false)
	b.Raw("DECQ R9").
		Raw("JMP gloop").
		Raw("knext:").
		Raw("ADDQ %s, AX", arg("din")).
		Raw("ADDQ %s, BX", arg("dout")).
		Raw("ADDQ %s, R15", arg("dtw")).
		Raw("MOVQ %s, R11", arg("br")).
		Raw("DECQ R8").
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// Batch emits name(cc, ch, tw *complex128, k *float64, ido, l1, pairs, odd,
// jin, jout, adjin, adjout int): one Stockham pass of radix r (2, 3, 4, 5 or
// 8) over
// a batch of w neighbouring transforms, the lines of a non-contiguous axis of
// an N-D array. Point p of the batch's transforms is w consecutive complex
// values ("a row" of the strip), sIn bytes after point p-1 on input and sOut
// on output, so the pass reads its strip straight out of the array (first
// pass) or writes it straight back (last pass) where the per-line path
// gathers and scatters. The butterflies are the ones every other pass kernel
// runs; point i of a block multiplies output j by one twiddle for the whole
// row, so the twiddle is broadcast from memory and the product costs one
// shuffle. Point i = 0 is not multiplied at all, as in the scalar pass, so no
// blend is needed. tw holds, for i = 1 .. ido-1, the r-1 twiddles of point i
// one after the other (StockhamBatchTwiddles).
//
// Arguments, in bytes where they are strides: jin = ido·sIn (input j stride),
// jout = l1·ido·sOut (output j stride), adjin = sIn - 16·w, adjout = sOut -
// 16·w; pairs = w/2 and odd = w&1 split the row into YMM pairs and one XMM
// point. Registers: AX/SI input (SI = AX + 4·jin), BX/DI output, CX = jin, DX
// = jout, R12 = 3·CX, R13 = 3·DX, R14 = constants, R10 = twiddle cursor, R15 =
// twiddle base, R8 = blocks left, R9 = points left, R11 = pairs left.
func (c *Kernels) Batch(f *emit.File, name string, r int) {
	check("Batch", r, 2, 3, 4, 5, 8)
	names := []string{"cc", "ch", "tw", "k", "ido", "l1", "pairs", "odd", "jin", "jout", "adjin", "adjout"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr}
	for range names[4:] {
		types = append(types, asm.Int64)
	}
	sig := asm.Layout(names, types, nil, nil)
	arg := func(n string) string { return fp(names, n) }
	b := asm.NewFunc(name, sig, 0)
	y0, x0 := skEmit{c: c, b: b, w: "Y", notw: true}, skEmit{c: c, b: b, w: "X", notw: true}
	yt, xt := skEmit{c: c, b: b, w: "Y", bcast: true}, skEmit{c: c, b: b, w: "X", bcast: true}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("tw", "R15").LoadArg("k", "R14").
		LoadArg("jin", "CX").LoadArg("jout", "DX").LoadArg("l1", "R8")
	b.Raw("LEAQ (CX)(CX*2), R12").
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("kloop:").
		Raw("TESTQ R8, R8").
		Raw("JZ done").
		Raw("LEAQ (AX)(CX*4), SI").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("MOVQ R15, R10")
	// row emits one point's row: the YMM pairs, then the odd XMM point, then
	// the step to the next point's row.
	row := func(label string, y, x skEmit) {
		b.Raw("MOVQ %s, R11", arg("pairs")).
			Raw("%sp:", label).
			Raw("TESTQ R11, R11").
			Raw("JZ %so", label)
		y.body(r, false)
		y.advance(32)
		b.Raw("DECQ R11").
			Raw("JMP %sp", label).
			Raw("%so:", label).
			Raw("CMPQ %s, $0", arg("odd")).
			Raw("JEQ %se", label)
		x.body(r, false)
		x.advance(16)
		b.Raw("%se:", label).
			Raw("ADDQ %s, AX", arg("adjin")).
			Raw("ADDQ %s, SI", arg("adjin")).
			Raw("ADDQ %s, BX", arg("adjout")).
			Raw("ADDQ %s, DI", arg("adjout"))
	}
	row("zero", y0, x0) // point i = 0: no twiddle
	b.Raw("MOVQ %s, R9", arg("ido")).
		Raw("DECQ R9").
		Raw("iloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ knext")
	row("tw", yt, xt)
	b.Raw("ADDQ $%d, R10", (r-1)*16).
		Raw("DECQ R9").
		Raw("JMP iloop").
		Raw("knext:")
	// P advanced by jin over the block; the next block starts r·jin after it.
	skNextBlock(b, r)
	// O advanced by ido·sOut, which is exactly the next block's output start.
	b.Raw("DECQ R8").
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// Pass emits name(cc, ch, tw *complex128, k *float64, ido, l1 int), the AVX2
// Stockham pass of radix r for ido >= 2: r = 2, 3, 4, 5 and 8 (the butterflies
// of go-fft's stockham.go), 9 (the 3×3 split), 10, 15 and 20 (the
// prime-factor algorithm for 5·q), 12 (prime-factor, 3·4) and 16 (the 4×4
// split). Every radix reads its twiddles group by group (skEmit.tw); radix
// 16's are stored pre-duplicated, (wr, wr) then (wi, wi) per group. The
// constants table is Kernels.K's.
func (c *Kernels) Pass(f *emit.File, name string, r int) {
	check("Pass", r, 2, 3, 4, 5, 8, 9, 10, 12, 15, 16, 20)
	switch r {
	case 9:
		c.pass9(f, name)
	case 10, 15, 20:
		c.passPFA(f, name, r/5)
	case 12:
		c.pass12(f, name)
	case 16:
		c.pass16(f, name)
	default:
		c.pass(f, name, r)
	}
}

// Last emits name(cc, ch *complex128, k *float64, l1 int), Pass's final pass
// (ido == 1) for the same radices: no twiddles, two blocks per YMM register,
// an odd l1 finished by one 128-bit step.
func (c *Kernels) Last(f *emit.File, name string, r int) {
	check("Last", r, 2, 3, 4, 5, 8, 9, 10, 12, 15, 16, 20)
	switch r {
	case 9:
		c.last9(f, name)
	case 10, 15, 20:
		c.lastPFA(f, name, r/5)
	case 12:
		c.last12(f, name)
	case 16:
		c.last16(f, name)
	default:
		c.last(f, name, r)
	}
}

// LastRun emits name(cc, ch *complex128, k *float64, os, runs, run, gap int),
// the AVX2 final pass of radix r (2, 4 or 8) that writes its outputs in runs
// (see lastRun). run is a multiple of 2.
func (c *Kernels) LastRun(f *emit.File, name string, r int) {
	check("LastRun", r, 2, 4, 8)
	c.lastRun(f, name, r, false)
}

// LastRun512 is LastRun at 512 bits, with Pass512's constants table. run is a
// multiple of 4.
func (c *Kernels) LastRun512(f *emit.File, name string, r int) {
	check("LastRun512", r, 2, 4, 8)
	c.lastRun(f, name, r, true)
}

// Strided emits name(cc, ch, tw *complex128, k *float64, cnt, nb, sin, sout,
// din, dout, dtw, bf, br int), Pass's radix-r pass (r = 4 or 8) with every
// stride a parameter (see strided).
func (c *Kernels) Strided(f *emit.File, name string, r int) {
	check("Strided", r, 4, 8)
	c.strided(f, name, r, false)
}

// Strided512 is Strided at 512 bits, with Pass512's constants table.
func (c *Kernels) Strided512(f *emit.File, name string, r int) {
	check("Strided512", r, 4, 8)
	c.strided(f, name, r, true)
}
