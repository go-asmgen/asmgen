package amd64

import (
	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Radix-10 pass at 512 bits (prototype): Pass512's four points per ZMM
// register around the radix-10 prime-factor butterfly of pfa.go (two radix-5
// butterflies, then five radix-2 butterflies, no twiddle in between). FFTW's
// t1fv_10.c (dft/simd/common, fftw-3.3.10) is the same 2x5 split with the
// same 5-point constants (KP559016994, KP951056516, KP250000000, lines
// 39-42), but a decimation-in-time codelet: it multiplies the ten inputs by
// their twiddles first (BYTWJ, lines 52-86) and uses FMA forms of the 5-point
// butterfly (lines 99-127). Ours is a Stockham pass with the twiddle on the
// outputs, so only the split and the constants carry over; the operation
// order is the AVX2 kernel's, not FFTW's.
//
// With 32 ZMM registers nothing is spilled: the radix-5 butterfly works in
// Z0..Z12 (as the YMM kernel does), the first butterfly's five outputs are
// moved to Z16..Z20, the second's stay where the butterfly leaves them
// (Z7, Z1..Z4), the radix-2 outputs go to Z8, Z9 and the twiddle product to
// Z26..Z29. Every operation is the AVX2 kernel's, so the result is
// bit-identical to it. ido >= 4; ido mod 4 is finished by the AVX2 kernel's
// own pair and single bodies, on the same frame.

// comp10ZBody emits one radix-10 butterfly over four points.
func (e skEmit) comp10ZBody(m comp2Map, first bool) {
	y := e.comp2Bfly5(m.in[0])
	for k1, r := range y {
		e.raw("VMOVAPD %s, %s", e.v(r), e.v(16+k1))
	}
	y = e.comp2Bfly5(m.in[1])
	for k1 := range 5 {
		e.add(8, 16+k1, y[k1])
		e.sub(9, 16+k1, y[k1])
		for k2, r := range [2]int{8, 9} {
			if j := m.out[k1][k2]; j == 0 {
				e.st(r, e.outAddr(0))
			} else {
				e.twStore(r, j, 26, 27, 28, 29, first)
			}
		}
	}
}

// pass10z512 emits Pass512's radix-10 pass.
func (c *Kernels) pass10z512(f *emit.File, name string) {
	const q, r = 2, 10
	m := comp2NewMap(q)
	spill := 32 * r
	blocks, groups := spill, spill+8
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, spill+16)
	z := skEmit{c: c, b: b, w: "Z", wide: true, g: 4, addr: comp2Addr}
	p := skEmit{c: c, b: b, w: "Y", wide: true, g: 2, addr: comp2Addr}
	x := skEmit{c: c, b: b, w: "X", wide: true, g: 1, addr: comp2Addr}
	b.Raw("MOVQ $3, R9").Raw("KMOVB R9, K1") // lanes 0-1: the i = 0 point
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
	b.Raw("LEAQ (%s)(CX*4), %s", comp2InBase[0], comp2InBase[1]).
		Raw("ADDQ CX, %s", comp2InBase[1]).
		Raw("LEAQ (%s)(DX*4), %s", comp2OutBase[0], comp2OutBase[1]).
		Raw("ADDQ DX, %s", comp2OutBase[1])
	b.LoadArg("tw", "R10")
	z.comp10ZBody(m, true)
	comp2Advance(b, q, 64)
	z.twNext(r)
	b.Raw("DECQ %d(SP)", groups).
		Raw("qloop:").
		Raw("CMPQ %d(SP), $0", groups).
		Raw("JEQ pair")
	z.comp10ZBody(m, false)
	comp2Advance(b, q, 64)
	z.twNext(r)
	b.Raw("DECQ %d(SP)", groups).
		Raw("JMP qloop").
		Raw("pair:").
		Raw("TESTQ $32, CX"). // ido & 2
		Raw("JZ single")
	p.comp2Body(m, false)
	comp2Advance(b, q, 32)
	p.twNext(r)
	b.Raw("single:").
		Raw("TESTQ $16, CX"). // ido & 1
		Raw("JZ knext")
	x.comp2Body(m, false)
	comp2Advance(b, q, 16)
	b.Raw("knext:").
		Raw("LEAQ (%s)(CX*4), AX", comp2InBase[q-1]).
		Raw("DECQ %d(SP)", blocks).
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
