package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// float32 (complex64) Stockham pass kernels, into stockham32_amd64.s.
//
// sk32Pass{r}AVX2(cc, ch, tw *complex64, k *float32, ido, l1 int) and
// sk32Last{r}AVX2(cc, ch *complex64, k *float32, l1 int) are the float32
// passes of the fft package's stockham32_passes.go (f32Pass{r},
// f32Pass{r}last) for radix 2, 3, 4, 5 and 8. They are genStockhamAVX2 and
// genStockhamLastAVX2 with single-precision instructions: a complex64 is 8
// bytes, so a YMM register holds four points and an XMM register two. They are
// kept apart from the float64 emitter (f32simdEmit, not skEmit) so a change to
// either family cannot move the other's arithmetic.
//
// Bit identity with the Go passes, which gc compiles for GOAMD64=v1 with
// separately rounded MULSS/ADDSS/SUBSS (no FMA): every operation below is the
// Go pass's, in its order, as VMULPS/VADDPS/VSUBPS; the ±i rotation is a lane
// swap (VPERMILPS) and a sign flip (VXORPS), both exact. The twiddle product
// y·w loads w's real parts duplicated (VMOVSLDUP) and its imaginary parts
// duplicated (VMOVSHDUP), both plain loads, and computes (yr·wr − yi·wi,
// yi·wr + yr·wi) with VADDSUBPS: f32Mul's products, its imaginary sum taken
// in the other order, which IEEE addition makes exact. The point i = 0, which
// the Go pass does not multiply, is multiplied by w = 1 with its group and
// then blended back (VBLENDPS $3): a multiply by one is not exact on signed
// zeros.
//
// Points are walked in twiddle groups (kernels.StockhamTwiddles32 on amd64):
// groups of four points in a YMM register, then one of two in an XMM register
// for ido&2 and one of one for ido&1, loaded and stored with VMOVSD (8 bytes)
// and computed in an XMM register whose upper lanes are zero. A group of g
// points stores its r-1 twiddle runs back to back, g entries each, so a pass
// reads its twiddles as one sequential stream (Round 10).
//
// k points at the float32 constants table (stockham32_amd64.go): 32-byte rows
// at the offsets of the float64 table (e.c.K.Sign … e.c.K.H), each constant rounded once
// to float32 as the Go passes' untyped constants are.
//
// Registers as genStockhamAVX2's, with S = ido·8 and OS = l1·ido·8.

type f32simdEmit struct {
	c    *Kernels
	b    *asm.Builder
	w    string // "Y" (four points), "X" (two) or "S" (one, in an X register)
	last int    // radix of a final-pass kernel (ido == 1), 0 for the others
	g    int    // twiddle group size in points: 4, 2 or 1
	// notw stores every output untwiddled (point i = 0 of a batched pass,
	// Round 25); bcast multiplies output j by one twiddle broadcast to every
	// lane, the complex64 at (j-1)·8(R10) (the other points of a batched
	// pass).
	notw, bcast bool
}

func (e f32simdEmit) v(i int) string {
	if e.w == "Y" {
		return fmt.Sprintf("Y%d", i)
	}
	return fmt.Sprintf("X%d", i)
}

func (e f32simdEmit) raw(format string, a ...any) { e.b.Raw(format, a...) }

func f32simdIn(j int) string {
	return [...]string{"(AX)", "(AX)(CX*1)", "(AX)(CX*2)", "(AX)(R12*1)", "(SI)", "(SI)(CX*1)", "(SI)(CX*2)", "(SI)(R12*1)"}[j]
}

func f32simdOut(j int) string {
	return [...]string{"(BX)", "(BX)(DX*1)", "(BX)(DX*2)", "(BX)(R13*1)", "(DI)", "(DI)(DX*1)", "(DI)(DX*2)", "(DI)(R13*1)"}[j]
}

func (e f32simdEmit) ld(dst int, addr string) {
	if e.w == "S" {
		e.raw("VMOVSD %s, X%d", addr, dst)
		return
	}
	e.raw("VMOVUPS %s, %s", addr, e.v(dst))
}

func (e f32simdEmit) st(src int, addr string) {
	if e.w == "S" {
		e.raw("VMOVSD X%d, %s", src, addr)
		return
	}
	e.raw("VMOVUPS %s, %s", e.v(src), addr)
}

// in loads input j. In a final pass (ido == 1) a register holds blocks k ..
// k+3 (or k, k+1, or k alone) instead of points: input j of block k+q is
// 8·(q·r+j) bytes into the blocks, gathered two by two (VMOVSD, VMOVHPS) and
// joined with VINSERTF128 through X15, which no final-pass body uses.
func (e f32simdEmit) in(dst, j int) {
	if e.last == 0 {
		e.ld(dst, f32simdIn(j))
		return
	}
	r := e.last
	e.raw("VMOVSD %d(AX), X%d", 8*j, dst)
	if e.w == "S" {
		return
	}
	e.raw("VMOVHPS %d(AX), X%d, X%d", 8*(r+j), dst, dst)
	if e.w == "X" {
		return
	}
	e.raw("VMOVSD %d(AX), X15", 8*(2*r+j))
	e.raw("VMOVHPS %d(AX), X15, X15", 8*(3*r+j))
	e.raw("VINSERTF128 $1, X15, Y%d, Y%d", dst, dst)
}

func (e f32simdEmit) add(dst, a, b int) { e.raw("VADDPS %s, %s, %s", e.v(b), e.v(a), e.v(dst)) }
func (e f32simdEmit) sub(dst, a, b int) { e.raw("VSUBPS %s, %s, %s", e.v(b), e.v(a), e.v(dst)) }
func (e f32simdEmit) mulk(dst, a int, off int) {
	e.raw("VMULPS %d(R14), %s, %s", off, e.v(a), e.v(dst))
}

// swap exchanges the real and imaginary part of every point.
func (e f32simdEmit) swap(src, dst int) { e.raw("VPERMILPS $0xB1, %s, %s", e.v(src), e.v(dst)) }

// rot: dst = rotS(src), a swap and the sign flip of the direction's row.
func (e f32simdEmit) rot(dst, src int) {
	e.swap(src, dst)
	e.raw("VXORPS %d(R14), %s, %s", e.c.K.Sign, e.v(dst), e.v(dst))
}

func (e f32simdEmit) tw(j int) string { return fmt.Sprintf("%d(R10)", (j-1)*8*e.g) }

func (e f32simdEmit) twNext(r int) { e.raw("ADDQ $%d, R10", (r-1)*8*e.g) }

func (e f32simdEmit) advance(bytes int) {
	for _, r := range []string{"AX", "SI", "BX", "DI"} {
		e.raw("ADDQ $%d, %s", bytes, r)
	}
}

// twStore multiplies y by twiddle j and stores it to output j; see the
// section comment. A lone point loads its twiddle with VMOVSD (a 16-byte
// VMOVSLDUP would read past the table) and duplicates it in registers.
func (e f32simdEmit) twStore(y, j, t1, t2, t3, dst int, first bool) {
	if e.last != 0 || e.notw {
		e.st(y, f32simdOut(j))
		return
	}
	if e.bcast {
		e.raw("VBROADCASTSS %d(R10), %s", (j-1)*8, e.v(t1))
		e.raw("VBROADCASTSS %d(R10), %s", (j-1)*8+4, e.v(t2))
	} else if e.w == "S" {
		e.raw("VMOVSD %s, X%d", e.tw(j), t1)
		e.raw("VMOVSHDUP X%d, X%d", t1, t2)
		e.raw("VMOVSLDUP X%d, X%d", t1, t1)
	} else {
		e.raw("VMOVSLDUP %s, %s", e.tw(j), e.v(t1))
		e.raw("VMOVSHDUP %s, %s", e.tw(j), e.v(t2))
	}
	e.swap(y, t3)
	e.raw("VMULPS %s, %s, %s", e.v(y), e.v(t1), e.v(t1))
	e.raw("VMULPS %s, %s, %s", e.v(t3), e.v(t2), e.v(t2))
	e.raw("VADDSUBPS %s, %s, %s", e.v(t2), e.v(t1), e.v(dst))
	if first {
		e.raw("VBLENDPS $3, %s, %s, %s", e.v(y), e.v(dst), e.v(dst))
	}
	e.st(dst, f32simdOut(j))
}

// body is skEmit.body's butterflies, operation for operation, which are also
// stockham32_passes.go's.
func (e f32simdEmit) body(r int, first bool) {
	switch r {
	case 2:
		e.in(0, 0)
		e.in(1, 1)
		e.add(2, 0, 1)
		e.st(2, f32simdOut(0))
		e.sub(3, 0, 1)
		e.twStore(3, 1, 5, 6, 7, 4, first)
	case 3:
		e.in(0, 0)
		e.in(1, 1)
		e.in(2, 2)
		e.add(3, 1, 2) // t1
		e.sub(4, 1, 2) // t2
		e.add(5, 0, 3)
		e.st(5, f32simdOut(0))
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
		e.st(0, f32simdOut(0))
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
		e.st(7, f32simdOut(0))
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
		e.mulk(8, 8, e.c.K.H) // a5 = h·(a5 + rotS(a5))
		e.rot(11, 9)
		e.sub(9, 11, 9)
		e.mulk(9, 9, e.c.K.H) // a7 = h·(rotS(a7) - a7)
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
		e.st(4, f32simdOut(0))
		e.sub(0, 0, 12)  // y4
		e.add(12, 1, 10) // y2
		e.sub(1, 1, 10)  // y6
		e.add(10, 3, 8)  // y1
		e.sub(3, 3, 8)   // y5
		e.add(8, 2, 9)   // y3
		e.sub(2, 2, 9)   // y7
		for _, yj := range [][2]int{{10, 1}, {12, 2}, {8, 3}, {0, 4}, {3, 5}, {1, 6}, {2, 7}} {
			e.twStore(yj[0], yj[1], 13, 14, 15, 4, first)
		}
	}
}

// f32simdNextBlock advances P from the end of one block (it moved by S) to
// the start of the next, r·S after this one's.
func f32simdNextBlock(b *asm.Builder, r int) {
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

// Pass32 emits name(cc, ch, tw *complex64, k *float32, ido, l1 int), the
// float32 pass of radix r (2, 3, 4, 5 or 8) for ido >= 2 (see above). k is
// the float32 constants table, laid out as Kernels.K says.
func (c *Kernels) Pass32(f *emit.File, name string, r int) {
	check("Pass32", r, 2, 3, 4, 5, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	y := f32simdEmit{c: c, b: b, w: "Y", g: 4}
	x := f32simdEmit{c: c, b: b, w: "X", g: 2}
	s := f32simdEmit{c: c, b: b, w: "S", g: 1}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("tw", "R15").LoadArg("k", "R14").
		LoadArg("ido", "CX").LoadArg("l1", "R8")
	b.Raw("MOVQ CX, DX").
		Raw("IMULQ R8, DX").
		Raw("SHLQ $3, CX"). // S
		Raw("SHLQ $3, DX"). // OS
		Raw("LEAQ (CX)(CX*2), R12").
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("kloop:").
		Raw("TESTQ R8, R8").
		Raw("JZ done").
		Raw("LEAQ (AX)(CX*4), SI").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("MOVQ R15, R10").
		Raw("MOVQ CX, R9").
		Raw("SHRQ $5, R9"). // four-point groups = ido/4
		Raw("TESTQ R9, R9").
		Raw("JZ firstpair")
	y.body(r, true)
	y.advance(32)
	y.twNext(r)
	b.Raw("DECQ R9").
		Raw("qloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ pair")
	y.body(r, false)
	y.advance(32)
	y.twNext(r)
	b.Raw("DECQ R9").
		Raw("JMP qloop").
		Raw("pair:").
		Raw("TESTQ $16, CX"). // ido & 2
		Raw("JZ single")
	x.body(r, false)
	x.advance(16)
	x.twNext(r)
	b.Raw("JMP single").
		Raw("firstpair:") // ido is 2 or 3: the first group is the pair
	x.body(r, true)
	x.advance(16)
	x.twNext(r)
	b.Raw("single:").
		Raw("TESTQ $8, CX"). // ido & 1
		Raw("JZ knext")
	s.body(r, false)
	s.advance(8)
	b.Raw("knext:")
	f32simdNextBlock(b, r)
	b.Raw("DECQ R8").
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// Last32 emits name(cc, ch *complex64, k *float32, l1 int): the final pass (ido == 1),
// four blocks per YMM register, then two and one for l1 mod 4. Output j of
// blocks k .. k+3 is contiguous, one VMOVUPS.
func (c *Kernels) Last32(f *emit.File, name string, r int) {
	check("Last32", r, 2, 3, 4, 5, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	y := f32simdEmit{c: c, b: b, w: "Y", last: r}
	x := f32simdEmit{c: c, b: b, w: "X", last: r}
	s := f32simdEmit{c: c, b: b, w: "S", last: r}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").LoadArg("l1", "DX")
	b.Raw("MOVQ DX, R9").
		Raw("SHRQ $2, R9"). // groups of four blocks
		Raw("SHLQ $3, DX"). // OS
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("loop:").
		Raw("TESTQ R9, R9").
		Raw("JZ pair")
	y.body(r, false)
	b.Raw("ADDQ $%d, AX", 32*r).
		Raw("ADDQ $32, BX").
		Raw("ADDQ $32, DI").
		Raw("DECQ R9").
		Raw("JMP loop").
		Raw("pair:").
		Raw("TESTQ $16, DX"). // l1 & 2
		Raw("JZ single")
	x.body(r, false)
	b.Raw("ADDQ $%d, AX", 16*r).
		Raw("ADDQ $16, BX").
		Raw("ADDQ $16, DI").
		Raw("single:").
		Raw("TESTQ $8, DX"). // l1 & 1
		Raw("JZ done")
	s.body(r, false)
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
