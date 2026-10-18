package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Radix-12 pass kernels (Round 24): go-fft's radix12.go, the
// prime-factor algorithm for 12 = 3·4. Input n = (4·n1 + 3·n2) mod 12 and
// output k = (4·k1 + 9·k2) mod 12: four radix-3 butterflies (one per n2, over
// n1), then three radix-4 butterflies (one per k1, over n2), with no twiddle
// between them. Every operation is the Go pass's (bfly3, bfly4, then the outer
// twiddle product of twStore), separately rounded, so the kernels are
// bit-identical to it as the other AVX2 kernels are to theirs.
//
// Registers: the stage-1 values of k1 = 0 stay in Y8..Y11; those of k1 = 1
// and 2 (eight registers) are spilled to the frame and read back for stage 2.
// Twelve input streams through AX, SI, R8 and twelve output streams through
// BX, DI, R11 (skIn, skOut), R10 the twiddle cursor, R15 the twiddle base, R9
// the groups left; the blocks left live in the frame.

const (
	comp12Spill  = 256 // eight spilled stage-1 values, 32 bytes each
	comp12Blocks = 256 // blocks left (pass kernel)
	comp12Frame  = 264
)

// comp12In and comp12Out are radix12.go's index maps.
var (
	comp12In  = [4][3]int{{0, 4, 8}, {3, 7, 11}, {6, 10, 2}, {9, 1, 5}}
	comp12Out = [3][4]int{{0, 9, 6, 3}, {4, 1, 10, 7}, {8, 5, 2, 11}}
)

// comp12Slot is the frame offset of the spilled stage-1 value (n2, k1), k1 =
// 1 or 2.
func comp12Slot(n2, k1 int) string { return fmt.Sprintf("%d(SP)", 32*(4*(k1-1)+n2)) }

// comp12Bfly3 emits the Go bfly3 on registers x into y, through t (three
// scratch registers): t1 = x1 + x2, t2 = x1 − x2, y0 = x0 + t1, ca = x0 −
// 0.5·t1, cb = rotS(sin(2π/3)·t2), y1 = ca + cb, y2 = ca − cb.
func (e skEmit) comp12Bfly3(x, y, t [3]int) {
	e.add(t[0], x[1], x[2]) // t1
	e.sub(t[1], x[1], x[2]) // t2
	e.add(y[0], x[0], t[0])
	e.mulk(t[2], t[0], e.c.K.Half)
	e.sub(t[2], x[0], t[2]) // ca
	e.mulk(t[1], t[1], e.c.K.Sin120)
	e.rot(t[1], t[1]) // cb
	e.add(y[1], t[2], t[1])
	e.sub(y[2], t[2], t[1])
}

// comp12Body emits one radix-12 butterfly over the points the register width
// holds, with the outer twiddles (twStore) unless this is a final pass.
func (e skEmit) comp12Body(first bool) {
	for n2 := 0; n2 < 4; n2++ {
		in := comp12In[n2]
		for q := 0; q < 3; q++ {
			e.in(q, in[q])
		}
		e.comp12Bfly3([3]int{0, 1, 2}, [3]int{8 + n2, 3, 4}, [3]int{5, 6, 7})
		e.raw("VMOVUPD %s, %s", e.v(3), comp12Slot(n2, 1))
		e.raw("VMOVUPD %s, %s", e.v(4), comp12Slot(n2, 2))
	}
	for k1 := 0; k1 < 3; k1++ {
		in, t, z := [4]int{8, 9, 10, 11}, [4]int{0, 1, 2, 3}, [4]int{4, 5, 6, 7}
		if k1 > 0 {
			in, t, z = [4]int{0, 1, 2, 3}, [4]int{4, 5, 6, 7}, [4]int{8, 9, 10, 11}
			for n2 := 0; n2 < 4; n2++ {
				e.raw("VMOVUPD %s, %s", comp12Slot(n2, k1), e.v(n2))
			}
		}
		e.bfly4r(in, t, z)
		for k2 := 0; k2 < 4; k2++ {
			if j := comp12Out[k1][k2]; j == 0 {
				e.st(z[k2], skOut(0))
			} else {
				e.twStore(z[k2], j, 12, 13, 14, 15, first)
			}
		}
	}
}

// comp12Advance moves the six stream bases of the radix-12 pass kernel.
func (e skEmit) comp12Advance(bytes int) {
	for _, r := range []string{"AX", "SI", "R8", "BX", "DI", "R11"} {
		e.raw("ADDQ $%d, %s", bytes, r)
	}
}

// pass12 emits Pass's radix-12 pass, the loop of pass around comp12Body.
func (c *Kernels) pass12(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, comp12Frame)
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
		Raw("MOVQ R8, %d(SP)", comp12Blocks).
		Raw("kloop:").
		Raw("CMPQ %d(SP), $0", comp12Blocks).
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
	q0.comp12Body(true)
	q0.comp12Advance(32)
	q1.comp12Body(false)
	q1.comp12Advance(32)
	q0.twNext(12)
	b.Raw("DECQ R9").
		Raw("qloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ pair")
	q0.comp12Body(false)
	q0.comp12Advance(32)
	q1.comp12Body(false)
	q1.comp12Advance(32)
	q0.twNext(12)
	b.Raw("DECQ R9").
		Raw("JMP qloop").
		Raw("pair:").
		Raw("TESTQ $32, CX"). // ido & 2
		Raw("JZ single")
	p.comp12Body(false)
	p.comp12Advance(32)
	p.twNext(12)
	b.Raw("JMP single").
		Raw("firstpair:") // ido is 2 or 3: the first pair is the two-point group
	p.comp12Body(true)
	p.comp12Advance(32)
	p.twNext(12)
	b.Raw("single:").
		Raw("TESTQ $16, CX"). // ido & 1
		Raw("JZ knext")
	x.comp12Body(false)
	x.comp12Advance(16)
	// P advanced by S over the block; the next block starts 12·S after this
	// one. O advanced by S, which is the next block's output start.
	b.Raw("knext:").
		Raw("LEAQ (AX)(CX*8), AX").
		Raw("ADDQ R12, AX").
		Raw("DECQ %d(SP)", comp12Blocks).
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// last12 emits Last's radix-12 final pass (ido == 1), two
// blocks per register as in Last, an odd l1 finished by one
// 128-bit step.
func (c *Kernels) last12(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, comp12Spill)
	y := skEmit{c: c, b: b, w: "Y", last: 12}
	x := skEmit{c: c, b: b, w: "X", last: 12}
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
	y.comp12Body(false)
	b.Raw("ADDQ $%d, AX", 32*12)
	for _, r := range []string{"BX", "DI", "R11"} {
		b.Raw("ADDQ $32, %s", r)
	}
	b.Raw("DECQ R9").
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ $16, DX"). // l1 odd
		Raw("JZ done")
	x.comp12Body(false)
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
