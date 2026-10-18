package amd64

import (
	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Radix 9 (Round 35): Pass and Last for r = 9, go-fft's pass9 and pass9last
// operation for operation.
// The butterfly is the 3×3 split (Cooley–Tukey with n = n2 + 3·n1, k = k1 +
// 3·k2, as FFTW's n1_9 and t1_9 codelets): three bfly3 over n1, the internal
// twiddles W9^(n2·k1) (e = 1, 2, 2, 4) as radix16Rot's four packed
// operations, then three bfly3 over n2. Nine values stay in Y0..Y8 between
// the stages and Y9..Y15 are scratch, so nothing spills. The loop is
// pass12's (stream 8 through R8 and R11); the block counter lives in the
// frame. 256 bits only: go-fft runs composites with the AVX2 kernels.

const (
	cs9Blocks = 0 // blocks left (pass kernel), frame offset
	cs9Frame  = 8
)

// csRot9 emits dst = src·W9^e for e in {1, 2, 4}: c·src + swap(src)·[−ss,
// ss], radix16Rot's four operations. ta and tb are scratch.
func (e skEmit) csRot9(dst, src, ex, ta, tb int) {
	k := map[int][2]int{1: {e.c.K.R9C1, e.c.K.R9S1}, 2: {e.c.K.R9C2, e.c.K.R9S2}, 4: {e.c.K.R9C4, e.c.K.R9S4}}[ex]
	e.rotC(dst, src, k[0], k[1], ta, tb)
}

// csBody9 emits one radix-9 butterfly over the points the register width
// holds, with the outer twiddles (twStore) unless this is a final pass.
// Stage-1 value (n2, k1) is kept in register 3·n2 + k1.
func (e skEmit) csBody9(first bool) {
	for n2 := 0; n2 < 3; n2++ {
		d := 3 * n2
		e.in(9, n2)
		e.in(10, n2+3)
		e.in(11, n2+6)
		e.add(12, 10, 11) // t1
		e.sub(13, 10, 11) // t2
		e.add(d, 9, 12)   // y0
		e.mulk(14, 12, e.c.K.Half)
		e.sub(14, 9, 14) // ca
		e.mulk(15, 13, e.c.K.Sin120)
		e.rot(15, 15) // cb
		if n2 == 0 {
			e.add(d+1, 14, 15)
			e.sub(d+2, 14, 15)
			continue
		}
		e.add(10, 14, 15)
		e.sub(11, 14, 15)
		e.csRot9(d+1, 10, n2, 12, 13)
		e.csRot9(d+2, 11, 2*n2, 12, 13)
	}
	for k1 := 0; k1 < 3; k1++ {
		e.add(12, 3+k1, 6+k1) // t1
		e.sub(13, 3+k1, 6+k1) // t2
		e.add(9, k1, 12)      // y0
		e.mulk(14, 12, e.c.K.Half)
		e.sub(14, k1, 14) // ca
		e.mulk(15, 13, e.c.K.Sin120)
		e.rot(15, 15) // cb
		e.add(10, 14, 15)
		e.sub(11, 14, 15)
		if k1 == 0 {
			e.st(9, skOut(0))
		} else {
			e.twStore(9, k1, 12, 13, 14, 15, first)
		}
		e.twStore(10, k1+3, 12, 13, 14, 15, first)
		e.twStore(11, k1+6, 12, 13, 14, 15, first)
	}
}

// pass9 emits Pass's radix-9 pass, pass12's loop around csBody9.
func (c *Kernels) pass9(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, cs9Frame)
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
		Raw("MOVQ R8, %d(SP)", cs9Blocks).
		Raw("kloop:").
		Raw("CMPQ %d(SP), $0", cs9Blocks).
		Raw("JEQ done").
		Raw("LEAQ (AX)(CX*4), SI").
		Raw("LEAQ (AX)(CX*8), R8").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("LEAQ (BX)(DX*8), R11").
		Raw("MOVQ R15, R10").
		Raw("MOVQ CX, R9").
		Raw("SHRQ $6, R9"). // four-point groups = ido/4
		Raw("TESTQ R9, R9").
		Raw("JZ firstpair")
	q0.csBody9(true)
	q0.comp12Advance(32)
	q1.csBody9(false)
	q1.comp12Advance(32)
	q0.twNext(9)
	b.Raw("DECQ R9").
		Raw("qloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ pair")
	q0.csBody9(false)
	q0.comp12Advance(32)
	q1.csBody9(false)
	q1.comp12Advance(32)
	q0.twNext(9)
	b.Raw("DECQ R9").
		Raw("JMP qloop").
		Raw("pair:").
		Raw("TESTQ $32, CX"). // ido & 2
		Raw("JZ single")
	p.csBody9(false)
	p.comp12Advance(32)
	p.twNext(9)
	b.Raw("JMP single").
		Raw("firstpair:") // ido is 2 or 3: the first pair is the two-point group
	p.csBody9(true)
	p.comp12Advance(32)
	p.twNext(9)
	b.Raw("single:").
		Raw("TESTQ $16, CX"). // ido & 1
		Raw("JZ knext")
	x.csBody9(false)
	x.comp12Advance(16)
	// P advanced by S over the block; the next block starts 9·S after this
	// one. O advanced by S, which is the next block's output start.
	b.Raw("knext:").
		Raw("LEAQ (AX)(CX*8), AX").
		Raw("DECQ %d(SP)", cs9Blocks).
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// last9 emits Last's radix-9 final pass (ido == 1), two
// blocks per register as in Last, an odd l1 finished by one
// 128-bit step.
func (c *Kernels) last9(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	y := skEmit{c: c, b: b, w: "Y", last: 9}
	x := skEmit{c: c, b: b, w: "X", last: 9}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").LoadArg("l1", "DX")
	b.Raw("MOVQ DX, R9").
		Raw("SHRQ $1, R9"). // block pairs
		Raw("SHLQ $4, DX"). // OS
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("LEAQ (BX)(DX*8), R11").
		Raw("loop:").
		Raw("TESTQ R9, R9").
		Raw("JZ tail")
	y.csBody9(false)
	b.Raw("ADDQ $%d, AX", 32*9)
	for _, r := range []string{"BX", "DI", "R11"} {
		b.Raw("ADDQ $32, %s", r)
	}
	b.Raw("DECQ R9").
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ $16, DX"). // l1 odd
		Raw("JZ done")
	x.csBody9(false)
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
