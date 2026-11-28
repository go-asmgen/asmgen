package amd64

import (
	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// zeroHighZ zeroes Z16..Z31 before VZEROUPPER, which leaves them as they were.
// The radix-10 and radix-12 512-bit passes use Z16 and above, and on a Zen 4
// the dirty registers slow the plain AVX2 code that runs afterwards in the same
// process by 1.3 to 1.9 % (a 1,440-point plan after a 1,080-point one: 1.583
// then 1.604 µs per call); zeroing them on exit removes it. VPXORQ of a
// register with itself is a zeroing idiom, and AVX-512F has it.
func zeroHighZ(b *asm.Builder) {
	for i := 16; i < 32; i++ {
		b.Raw("VPXORQ Z%d, Z%d, Z%d", i, i, i)
	}
}

// ---------------------------------------------------------------------------
// Radix-12 pass at 512 bits: Pass512's four points per ZMM register around the
// radix-12 prime-factor butterfly of radix12.go (four radix-3 butterflies, then
// three radix-4 butterflies). FFTW's t1fv_12.c (dft/simd/common, fftw-3.3.10)
// has the same 3x4 shape and the same constants (KP866025403, KP500000000,
// lines 39-40; the radix-3 groups are lines 48-94, the radix-4 stage starts at
// line 95), but is a decimation-in-time codelet that multiplies the inputs by
// their twiddles first (BYTWJ). Ours is a Stockham pass with the twiddle on
// the outputs, so only the split and the constants carry over.
//
// With 32 ZMM registers the whole butterfly fits in registers: the twelve
// stage-1 values live in Z8..Z19, the radix-3 inputs and temporaries in
// Z0..Z7, the twiddle product in Z20..Z23, so nothing is spilled (the YMM
// kernel spills eight values per point group). Every operation is the AVX2
// kernel's, including the twiddle product (fmaProduct for the FMA kernels), so
// the result is bit-identical to it. ido >= 4; ido mod 4 is finished by the
// AVX2 kernel's own pair and single bodies, on the same frame.

// comp12ZReg is the register of the stage-1 value (n2, k1) in the 512-bit
// body.
func comp12ZReg(n2, k1 int) int { return 8 + 3*n2 + k1 }

// comp12ZBody emits one radix-12 butterfly over four points.
func (e skEmit) comp12ZBody(first bool) {
	for n2 := 0; n2 < 4; n2++ {
		in := comp12In[n2]
		for q := 0; q < 3; q++ {
			e.in(q, in[q])
		}
		e.comp12Bfly3([3]int{0, 1, 2},
			[3]int{comp12ZReg(n2, 0), comp12ZReg(n2, 1), comp12ZReg(n2, 2)}, [3]int{3, 4, 5})
	}
	for k1 := 0; k1 < 3; k1++ {
		in := [4]int{comp12ZReg(0, k1), comp12ZReg(1, k1), comp12ZReg(2, k1), comp12ZReg(3, k1)}
		t, z := [4]int{0, 1, 2, 3}, [4]int{4, 5, 6, 7}
		e.bfly4r(in, t, z)
		for k2 := 0; k2 < 4; k2++ {
			if j := comp12Out[k1][k2]; j == 0 {
				e.st(z[k2], skOut(0))
			} else {
				e.twStore(z[k2], j, 20, 21, 22, 23, first)
			}
		}
	}
}

// pass12z512 emits Pass512's radix-12 pass.
func (c *Kernels) pass12z512(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, comp12Frame)
	z := skEmit{c: c, b: b, w: "Z", wide: true, g: 4}
	y := skEmit{c: c, b: b, w: "Y", wide: true, g: 2}
	x := skEmit{c: c, b: b, w: "X", wide: true, g: 1}
	b.Raw("MOVQ $3, R9").Raw("KMOVB R9, K1") // lanes 0-1: the i = 0 point
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
		Raw("MOVQ R15, R10")
	z.comp12ZBody(true)
	z.comp12Advance(64)
	z.twNext(12)
	b.Raw("MOVQ CX, R9").
		Raw("SHRQ $6, R9"). // quads = ido/4
		Raw("DECQ R9").
		Raw("iloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ pair")
	z.comp12ZBody(false)
	z.comp12Advance(64)
	z.twNext(12)
	b.Raw("DECQ R9").
		Raw("JMP iloop").
		Raw("pair:").
		Raw("TESTQ $32, CX"). // ido & 2
		Raw("JZ single")
	y.comp12Body(false)
	y.comp12Advance(32)
	y.twNext(12)
	b.Raw("single:").
		Raw("TESTQ $16, CX"). // ido & 1
		Raw("JZ knext")
	x.comp12Body(false)
	x.comp12Advance(16)
	b.Raw("knext:").
		Raw("LEAQ (AX)(CX*8), AX").
		Raw("ADDQ R12, AX").
		Raw("DECQ %d(SP)", comp12Blocks).
		Raw("JMP kloop").
		Raw("done:")
	zeroHighZ(b)
	b.Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
