package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Radix-10, -15 and -20 pass kernels (Round 26): go-fft's radix5q.go, the prime-factor algorithm for r = 5·q, q = 2, 3, 4.
// Input n = (q·n1 + 5·n2) mod r, output k = (a·k1 + b·k2) mod r (a ≡ 1 mod 5,
// a ≡ 0 mod q, b ≡ 0 mod 5, b ≡ 1 mod q): q radix-5 butterflies (one per n2,
// over n1), then five radix-q butterflies (one per k1, over n2), with no
// twiddle in between. Every operation is the Go pass's (bfly5, then the add
// and subtract of radix 2, bfly3 or bfly4, then the outer twiddle product of
// twStore), separately rounded, so the kernels are bit-identical to it as the
// other AVX2 kernels are to theirs.
//
// Registers: the stage-1 values go to the frame (5·q slots of 32 bytes) and
// are read back for stage 2. Stream j sits 5·⌊j/5⌋ + j mod 5 strides from
// the start, through q bases: AX, SI, R8, R9 for the inputs (strides CX = S,
// R12 = 3S) and BX, DI, R11, R15 for the outputs (DX = OS, R13 = 3OS), each
// base reaching its five streams as +0, +1, +2, +3 (3S) and +4 (CX*4). R10
// is the twiddle cursor, R14 the constants; the blocks and the four-point
// groups left live in the frame.

// comp2Bases are the stream bases of the radix-5q kernels.
var (
	comp2InBase  = [4]string{"AX", "SI", "R8", "R9"}
	comp2OutBase = [4]string{"BX", "DI", "R11", "R15"}
)

// comp2Addr addresses stream j from its base: input streams j·S, output
// streams j·OS. A final pass reads its inputs through skEmit.in, not this.
func comp2Addr(out bool, j int) string {
	base, s, s3 := comp2InBase[j/5], "CX", "R12"
	if out {
		base, s, s3 = comp2OutBase[j/5], "DX", "R13"
	}
	switch j % 5 {
	case 0:
		return "(" + base + ")"
	case 1:
		return fmt.Sprintf("(%s)(%s*1)", base, s)
	case 2:
		return fmt.Sprintf("(%s)(%s*2)", base, s)
	case 3:
		return fmt.Sprintf("(%s)(%s*1)", base, s3)
	}
	return fmt.Sprintf("(%s)(%s*4)", base, s)
}

// comp2Map is radix5q.go's index map for r = 5·q.
type comp2Map struct {
	q   int
	in  [4][5]int
	out [5][4]int
}

func comp2NewMap(q int) comp2Map {
	r := 5 * q
	var a, b int
	for x := range r {
		if x%5 == 1 && x%q == 0 {
			a = x
		}
		if x%5 == 0 && x%q == 1 {
			b = x
		}
	}
	m := comp2Map{q: q}
	for n2 := range q {
		for n1 := range 5 {
			m.in[n2][n1] = (q*n1 + 5*n2) % r
		}
	}
	for k1 := range 5 {
		for k2 := range q {
			m.out[k1][k2] = (a*k1 + b*k2) % r
		}
	}
	return m
}

// comp2Slot is the frame offset of stage-1 value (n2, k1).
func comp2Slot(q, n2, k1 int) string { return fmt.Sprintf("%d(SP)", 32*(q*k1+n2)) }

// comp2Bfly5 emits the Go bfly5 on input streams js (the body of Pass's
// radix 5, its loads included) and leaves y0..y4 in Y7, Y1, Y2, Y3, Y4.
func (e skEmit) comp2Bfly5(js [5]int) [5]int {
	e.in(0, js[0])
	e.in(1, js[1])
	e.in(2, js[4])
	e.add(3, 1, 2) // t1
	e.sub(4, 1, 2) // t2
	e.in(1, js[2])
	e.in(2, js[3])
	e.add(5, 1, 2) // t3
	e.sub(6, 1, 2) // t4
	e.add(7, 0, 3)
	e.add(7, 7, 5) // y0
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
	return [5]int{7, 1, 2, 3, 4}
}

// comp2Body emits one radix-5q butterfly over the points the register width
// holds, with the outer twiddles (twStore) unless this is a final pass.
func (e skEmit) comp2Body(m comp2Map, first bool) {
	q := m.q
	for n2 := range q {
		y := e.comp2Bfly5(m.in[n2])
		for k1, r := range y {
			e.raw("VMOVUPD %s, %s", e.v(r), comp2Slot(q, n2, k1))
		}
	}
	for k1 := range 5 {
		for n2 := range q {
			e.raw("VMOVUPD %s, %s", comp2Slot(q, n2, k1), e.v(n2))
		}
		var z []int
		switch q {
		case 2:
			e.add(2, 0, 1)
			e.sub(3, 0, 1)
			z = []int{2, 3}
		case 3:
			e.comp12Bfly3([3]int{0, 1, 2}, [3]int{3, 4, 5}, [3]int{6, 7, 8})
			z = []int{3, 4, 5}
		default:
			e.bfly4r([4]int{0, 1, 2, 3}, [4]int{4, 5, 6, 7}, [4]int{8, 9, 10, 11})
			z = []int{8, 9, 10, 11}
		}
		for k2, r := range z {
			if j := m.out[k1][k2]; j == 0 {
				e.st(r, e.outAddr(0))
			} else {
				e.twStore(r, j, 12, 13, 14, 15, first)
			}
		}
	}
}

// comp2Advance moves the stream bases of a radix-5q pass kernel.
func comp2Advance(b *asm.Builder, q, bytes int) {
	for _, r := range comp2InBase[:q] {
		b.Raw("ADDQ $%d, %s", bytes, r)
	}
	for _, r := range comp2OutBase[:q] {
		b.Raw("ADDQ $%d, %s", bytes, r)
	}
}

// passPFA emits Pass's radix-r pass for r = 5·q: the loop of pass16 around
// comp2Body.
func (c *Kernels) passPFA(f *emit.File, name string, q int) {
	r := 5 * q
	m := comp2NewMap(q)
	spill := 32 * r
	blocks, groups := spill, spill+8
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, spill+16)
	q0 := skEmit{c: c, b: b, w: "Y", g: 4, addr: comp2Addr}
	q1 := skEmit{c: c, b: b, w: "Y", g: 4, half: 1, addr: comp2Addr}
	p := skEmit{c: c, b: b, w: "Y", g: 2, addr: comp2Addr}
	x := skEmit{c: c, b: b, w: "X", g: 1, addr: comp2Addr}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").
		LoadArg("ido", "CX").LoadArg("l1", "R8")
	b.Raw("MOVQ CX, DX").
		Raw("IMULQ R8, DX").
		Raw("SHLQ $4, CX"). // S
		Raw("SHLQ $4, DX"). // OS
		Raw("LEAQ (CX)(CX*2), R12").
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("MOVQ R8, %d(SP)", blocks).
		Raw("kloop:").
		Raw("CMPQ %d(SP), $0", blocks).
		Raw("JEQ done").
		Raw("MOVQ CX, R9").
		Raw("SHRQ $6, R9"). // four-point groups = ido/4
		Raw("MOVQ R9, %d(SP)", groups)
	// Base t+1 is base t plus five strides.
	for t := 1; t < q; t++ {
		b.Raw("LEAQ (%s)(CX*4), %s", comp2InBase[t-1], comp2InBase[t]).
			Raw("ADDQ CX, %s", comp2InBase[t]).
			Raw("LEAQ (%s)(DX*4), %s", comp2OutBase[t-1], comp2OutBase[t]).
			Raw("ADDQ DX, %s", comp2OutBase[t])
	}
	b.LoadArg("tw", "R10")
	b.Raw("CMPQ %d(SP), $0", groups).
		Raw("JEQ firstpair")
	q0.comp2Body(m, true)
	comp2Advance(b, q, 32)
	q1.comp2Body(m, false)
	comp2Advance(b, q, 32)
	q0.twNext(r)
	b.Raw("DECQ %d(SP)", groups).
		Raw("qloop:").
		Raw("CMPQ %d(SP), $0", groups).
		Raw("JEQ pair")
	q0.comp2Body(m, false)
	comp2Advance(b, q, 32)
	q1.comp2Body(m, false)
	comp2Advance(b, q, 32)
	q0.twNext(r)
	b.Raw("DECQ %d(SP)", groups).
		Raw("JMP qloop").
		Raw("pair:").
		Raw("TESTQ $32, CX"). // ido & 2
		Raw("JZ single")
	p.comp2Body(m, false)
	comp2Advance(b, q, 32)
	p.twNext(r)
	b.Raw("JMP single").
		Raw("firstpair:") // ido is 2 or 3: the first pair is the two-point group
	p.comp2Body(m, true)
	comp2Advance(b, q, 32)
	p.twNext(r)
	b.Raw("single:").
		Raw("TESTQ $16, CX"). // ido & 1
		Raw("JZ knext")
	x.comp2Body(m, false)
	comp2Advance(b, q, 16)
	// The last base advanced by S over the block, from 5·(q-1)·S past the
	// block's start; the next block starts r·S after it, four strides on.
	// O advanced by S, which is the next block's output start.
	b.Raw("knext:").
		Raw("LEAQ (%s)(CX*4), AX", comp2InBase[q-1]).
		Raw("DECQ %d(SP)", blocks).
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// lastPFA emits Last's radix-r final pass for r = 5·q (ido == 1),
// two blocks per register as in Last, an odd l1 finished by
// one 128-bit step.
func (c *Kernels) lastPFA(f *emit.File, name string, q int) {
	r := 5 * q
	m := comp2NewMap(q)
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 32*r)
	y := skEmit{c: c, b: b, w: "Y", last: r, addr: comp2Addr}
	x := skEmit{c: c, b: b, w: "X", last: r, addr: comp2Addr}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").LoadArg("l1", "DX")
	b.Raw("MOVQ DX, R9").
		Raw("SHRQ $1, R9"). // block pairs
		Raw("SHLQ $4, DX"). // OS
		Raw("LEAQ (DX)(DX*2), R13")
	for t := 1; t < q; t++ {
		b.Raw("LEAQ (%s)(DX*4), %s", comp2OutBase[t-1], comp2OutBase[t]).
			Raw("ADDQ DX, %s", comp2OutBase[t])
	}
	b.Raw("loop:").
		Raw("TESTQ R9, R9").
		Raw("JZ tail")
	y.comp2Body(m, false)
	b.Raw("ADDQ $%d, AX", 32*r)
	for _, o := range comp2OutBase[:q] {
		b.Raw("ADDQ $32, %s", o)
	}
	b.Raw("DECQ R9").
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ $16, DX"). // l1 odd
		Raw("JZ done")
	x.comp2Body(m, false)
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
