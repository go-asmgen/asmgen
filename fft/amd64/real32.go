package amd64

import (
	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// float32 real-FFT untangle and its inverse (Round 25).
//
// Untangle32's name(dst, z, tw *complex64, k *float32, m, quads int) is
// go-fft's f32Untangle for bins k = 1 .. 4·quads, and Retangle32's
// name(z, x, tw *complex64, k, h *float32, m, quads int) its
// f32Retangle loop (f32rRetangle) for the same k: the float64 untangle
// and retangle (smallUntangleStep) with single-precision instructions, four consecutive k per
// YMM register. The mirrored bins m-k-3 .. m-k are one 32-byte load whose
// four complex64 VPERMPD $0x1B reverses (and the same instruction reverses
// them back before the store), so lane q of both registers holds the pair
// (k+q, m-k-q). Every operation is the Go code's, in its order, separately
// rounded (gc does not fuse on GOAMD64=v1): the even/odd split as a blend
// (VBLENDPS) of the sum and the difference, the ±0.5 and the conjugations as
// sign flips (VXORPS with k's row 0, [0, -0, ...], the imaginary-lane mask),
// and W·xo as VMOVSLDUP/VMOVSHDUP and VADDSUBPS: (wr·xor − wi·xoi,
// wr·xoi + wi·xor), the Go products in the Go order. k is the forward float32
// constants table: row Sign the mask, row Half 0.5.

// Batch32 emits name(cc, ch, tw *complex64, k *float32, ido, l1, quads, rem,
// jin, jout, adjin, adjout int) (Round 25), r = 2, 3, 4, 5 or 8: one float32
// Stockham pass of radix r over a batch of w = 4·quads + rem transforms laid
// side by side, the strips of a non-contiguous N-D axis (go-fft's
// skStage32.passBatch); Batch with single-precision registers.
// Point p of the batch is w neighbouring complex64 values, so four lines at
// one point are one YMM register (then two in an XMM register for rem&2 and
// one, VMOVSD, for rem&1), and every lane runs exactly the arithmetic of the
// 1-D float32 kernels (Pass32) on one line: the same butterflies,
// the same twiddle product. A point's twiddle is the same for every line,
// broadcast with VBROADCASTSS from the batched table (for i = 1 .. ido-1, the
// r-1 twiddles of point i); point i = 0 is not multiplied, as in the Go pass.
//
// Registers as Pass32's, with CX = jin = 8·ido·sIn (one input
// stream), DX = jout = 8·l1·ido·sOut; each row (one point) advances the
// streams by 8·w, then by adjin = 8·(sIn-w) and adjout = 8·(sOut-w).
func (c *Kernels) Batch32(f *emit.File, name string, r int) {
	check("Batch32", r, 2, 3, 4, 5, 8)
	names := []string{"cc", "ch", "tw", "k", "ido", "l1", "quads", "rem", "jin", "jout", "adjin", "adjout"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr}
	for range names[4:] {
		types = append(types, asm.Int64)
	}
	sig := asm.Layout(names, types, nil, nil)
	arg := func(n string) string { return fp(names, n) }
	b := asm.NewFunc(name, sig, 0)
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
	row := func(label string, notw bool) {
		y := f32simdEmit{c: c, b: b, w: "Y", notw: notw, bcast: !notw}
		x := f32simdEmit{c: c, b: b, w: "X", notw: notw, bcast: !notw}
		s := f32simdEmit{c: c, b: b, w: "S", notw: notw, bcast: !notw}
		b.Raw("MOVQ %s, R11", arg("quads")).
			Raw("%sq:", label).
			Raw("TESTQ R11, R11").
			Raw("JZ %sp", label)
		y.body(r, false)
		y.advance(32)
		b.Raw("DECQ R11").
			Raw("JMP %sq", label).
			Raw("%sp:", label).
			Raw("MOVQ %s, R11", arg("rem")).
			Raw("TESTQ $2, R11").
			Raw("JZ %ss", label)
		x.body(r, false)
		x.advance(16)
		b.Raw("%ss:", label).
			Raw("TESTQ $1, R11").
			Raw("JZ %se", label)
		s.body(r, false)
		s.advance(8)
		b.Raw("%se:", label).
			Raw("ADDQ %s, AX", arg("adjin")).
			Raw("ADDQ %s, SI", arg("adjin")).
			Raw("ADDQ %s, BX", arg("adjout")).
			Raw("ADDQ %s, DI", arg("adjout"))
	}
	row("zero", true) // point i = 0: no twiddle
	b.Raw("MOVQ %s, R9", arg("ido")).
		Raw("DECQ R9").
		Raw("iloop:").
		Raw("TESTQ R9, R9").
		Raw("JZ knext")
	row("tw", false)
	b.Raw("ADDQ $%d, R10", (r-1)*8).
		Raw("DECQ R9").
		Raw("JMP iloop").
		Raw("knext:")
	f32simdNextBlock(b, r)
	b.Raw("DECQ R8").
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// f32rMirrorSetup loads the four cursors of an untangle kernel: AX = &in[1],
// BX = &in[m-4], R9 = &out[1], R10 = &out[m-4], CX = &tw[1], with DX = m.
func f32rMirrorSetup(b *asm.Builder, in, out string) {
	b.Raw("SHLQ $3, DX"). // m·8
				Raw("LEAQ 8(%s), AX", in).
				Raw("LEAQ -32(%s)(DX*1), BX", in).
				Raw("LEAQ 8(%s), R9", out).
				Raw("LEAQ -32(%s)(DX*1), R10", out).
				Raw("ADDQ $8, CX")
}

// f32rMirrorAdvance moves the cursors to the next four bins and loops.
func f32rMirrorAdvance(b *asm.Builder) {
	b.Raw("ADDQ $32, AX").
		Raw("SUBQ $32, BX").
		Raw("ADDQ $32, R9").
		Raw("SUBQ $32, R10").
		Raw("ADDQ $32, CX").
		Raw("DECQ R8").
		Raw("JMP loop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
}

// Untangle32 emits the float32 untangle kernel (see above).
func (c *Kernels) Untangle32(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"dst", "z", "tw", "k", "m", "quads"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("dst", "DI").LoadArg("z", "SI").LoadArg("tw", "CX").LoadArg("k", "R14").
		LoadArg("m", "DX").LoadArg("quads", "R8")
	f32rMirrorSetup(b, "SI", "DI")
	b.Raw("VMOVUPS %d(R14), Y12", c.K.Sign). // imaginary-lane sign mask
							Raw("VMOVUPS %d(R14), Y13", c.K.Half). // 0.5
							Raw("loop:").
							Raw("TESTQ R8, R8").
							Raw("JZ done").
							Raw("VMOVUPS (AX), Y0").           // a = Z[k .. k+3]
							Raw("VMOVUPS (BX), Y1").           // Z[m-k-3 .. m-k]
							Raw("VPERMPD $0x1B, Y1, Y1").      // b = Z[m-k], Z[m-k-1], Z[m-k-2], Z[m-k-3]
							Raw("VADDPS Y1, Y0, Y2").          // s
							Raw("VSUBPS Y1, Y0, Y3").          // d
							Raw("VBLENDPS $0xAA, Y3, Y2, Y4"). // [s.re, d.im]
							Raw("VMULPS Y13, Y4, Y4").         // xe
							Raw("VBLENDPS $0x55, Y3, Y2, Y5"). // [d.re, s.im]
							Raw("VPERMILPS $0xB1, Y5, Y5").    // [s.im, d.re]
							Raw("VXORPS Y12, Y5, Y5").         // [s.im, -d.re]
							Raw("VMULPS Y13, Y5, Y5").         // xo
							Raw("VMOVSLDUP (CX), Y7").         // [wr, wr]
							Raw("VMOVSHDUP (CX), Y8").         // [wi, wi]
							Raw("VPERMILPS $0xB1, Y5, Y9").    // [xo.im, xo.re]
							Raw("VMULPS Y5, Y7, Y7").          // [wr·xo.re, wr·xo.im]
							Raw("VMULPS Y9, Y8, Y8").          // [wi·xo.im, wi·xo.re]
							Raw("VADDSUBPS Y8, Y7, Y7").       // t = w·xo
							Raw("VADDPS Y7, Y4, Y10").         // dst[k .. k+3]
							Raw("VSUBPS Y7, Y4, Y11").         // xe - t
							Raw("VXORPS Y12, Y11, Y11").       // conj: dst[m-k], ..., dst[m-k-3]
							Raw("VPERMPD $0x1B, Y11, Y11").    // dst[m-k-3 .. m-k]
							Raw("VMOVUPS Y10, (R9)").
							Raw("VMOVUPS Y11, (R10)")
	f32rMirrorAdvance(b)
	f.Add(b.Func())
}

// Retangle32 emits the float32 retangle kernel (see above).
func (c *Kernels) Retangle32(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"z", "x", "tw", "k", "h", "m", "quads"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("z", "DI").LoadArg("x", "SI").LoadArg("tw", "CX").LoadArg("k", "R14").
		LoadArg("h", "R15").LoadArg("m", "DX").LoadArg("quads", "R8")
	f32rMirrorSetup(b, "SI", "DI")
	b.Raw("VMOVUPS (R15), Y12"). // h
					Raw("VMOVUPS %d(R14), Y13", c.K.Sign). // imaginary-lane sign mask
					Raw("loop:").
					Raw("TESTQ R8, R8").
					Raw("JZ done").
					Raw("VMOVUPS (AX), Y0"). // a = X[k .. k+3]
					Raw("VMOVUPS (BX), Y1").
					Raw("VPERMPD $0x1B, Y1, Y1").      // b = X[m-k], ..., X[m-k-3]
					Raw("VADDPS Y1, Y0, Y2").          // s
					Raw("VSUBPS Y1, Y0, Y3").          // d
					Raw("VBLENDPS $0xAA, Y3, Y2, Y4"). // [s.re, d.im]
					Raw("VMULPS Y12, Y4, Y4").         // xe
					Raw("VBLENDPS $0x55, Y3, Y2, Y5"). // [d.re, s.im]
					Raw("VMULPS Y12, Y5, Y5").         // dd = [dr, di]
					Raw("VMOVUPS (CX), Y6").
					Raw("VXORPS Y13, Y6, Y6").      // conj(w) = [wr, -wi]
					Raw("VMOVSLDUP Y6, Y7").        // [wr, wr]
					Raw("VMOVSHDUP Y6, Y8").        // [-wi, -wi]
					Raw("VPERMILPS $0xB1, Y5, Y9"). // [di, dr]
					Raw("VMULPS Y5, Y7, Y7").       // [wr·dr, wr·di]
					Raw("VMULPS Y9, Y8, Y8").       // [-wi·di, -wi·dr]
					Raw("VADDSUBPS Y8, Y7, Y7").    // xo = [wr·dr + wi·di, wr·di - wi·dr]
					Raw("VPERMILPS $0xB1, Y7, Y8"). // [xoi, xor]
					Raw("VXORPS Y13, Y8, Y8").      // [xoi, -xor]
					Raw("VSUBPS Y8, Y4, Y10").      // [xer - xoi, xei + xor] = Z[k]
					Raw("VADDPS Y8, Y4, Y11").      // [xer + xoi, xei - xor]
					Raw("VXORPS Y13, Y11, Y11").    // Z[m-k] = [xer + xoi, -(xei - xor)]
					Raw("VPERMPD $0x1B, Y11, Y11").
					Raw("VMOVUPS Y10, (R9)").
					Raw("VMOVUPS Y11, (R10)")
	f32rMirrorAdvance(b)
	f.Add(b.Func())
}
