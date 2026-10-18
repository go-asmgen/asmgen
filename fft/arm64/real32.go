package arm64

import (
	asm "github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// float32 real-FFT untangle and its inverse (Round 25), into
// untangle32_arm64.s.
//
// f32rUntangleNEON(dst, z, tw *complex64, m, quads int) is the fft package's
// f32Untangle for bins k = 1 .. 4·quads, and f32rRetangleNEON(z, x, tw
// *complex64, h *float32, m, quads int) its f32Retangle loop for the same k,
// four consecutive k per .4S register pair: VLD2 splits Z[k .. k+3] into real
// and imaginary parts, and the mirrored Z[m-k-3 .. m-k] the same way, then
// VREV64 and VEXT $8 reverse the four lanes so lane q holds bin m-k-q (and
// the same two reverse the mirrored outputs before their VST2).
//
// Bit identity. gc fuses these loops heavily on arm64 (go build
// -gcflags=-S, Go 1.27.1, realplan32.go); with s = a+b, d = a-b of the pair
// (a, b) = (bin k, bin m-k):
//
//	untangle: xor = s.im·0.5 and xoi = -(d.re·0.5) rounded (FMULS, FNMULS);
//	  tr = round(wr·xor) − wi·xoi and ti = round(wr·xoi) + wi·xor fused
//	  (FMSUBS, FMADDS); dst[k] = (tr + s.re·0.5, ti + d.im·0.5) fused (FMADDS:
//	  xe's halving is never rounded on its own); dst[m-k] = (s.re·0.5 − tr,
//	  −(d.im·0.5 − ti)) fused (FNMSUBS, then FNEGS).
//	retangle: dr = d.re·h, di = s.im·h rounded; xor = round(wr·dr) + wi·di,
//	  xoi = round(wr·di) − wi·dr fused; z[k] = (h·s.re − xoi, xor + h·d.im),
//	  z[m-k] = (xoi + h·s.re, −(h·d.im − xor)) fused, s.re and d.im never
//	  multiplied by h on their own.
//
// FNMSUBS computes p − c as (−c) + p with one rounding; NEON has no vector
// form, so the kernels negate c (exact) and VFMLA, which is the same
// operation, signed zeros included (VFMLS would give −(c − p), the other
// zero). The tests compare kernel and Go loop bit for bit.

// genF32rBatchNEON emits sk32Batch<r>NEON / sk32Batch<r>NEONInv(cc, ch, tw
// *complex64, ido, l1, quads, rem, jin, jout, adjin, adjout int) (Round 25):
// genStockhamBatchNEON in single precision, one float32 Stockham pass of
// radix r over w = 4·quads + rem transforms side by side. Four lines at one
// point are one VLD2 on .4S (two on .2S for rem&2, one FLDPS for rem&1), and
// every lane runs the arithmetic of the 1-D float32 kernels
// (f32simdPassStep), fused products included. The point's twiddle, the same
// for every line, is loaded already broadcast and split by VLD2R from the
// batched table (for i = 1 .. ido-1, the r-1 twiddles of point i); point
// i = 0 is not multiplied. Streams, counters and row steps as
// genStockhamBatchNEON's, with 8-byte values: jin = 8·ido·sIn, jout =
// 8·l1·ido·sOut, adjin = 8·(sIn-w), adjout = 8·(sOut-w).
func Batch32(f *emit.File, name string, r int, inverse bool) {
	check("Batch32", r, 2, 3, 4, 5, 8)
	names := []string{"cc", "ch", "tw", "ido", "l1", "quads", "rem", "jin", "jout", "adjin", "adjout"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr}
	for range names[3:] {
		types = append(types, asm.Int64)
	}
	sig := asm.Layout(names, types, nil, nil)
	in, out := skStreams[r][0], skStreams[r][1]
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("cc", in[0]).LoadArg("ch", out[0]).LoadArg("tw", "R2").
		LoadArg("l1", "R5").LoadArg("jin", "R16").LoadArg("jout", "R17")
	f32simdConsts(b, r, 18)
	row := func(label string, twiddled bool) {
		b.LoadArg("quads", "R15").
			Raw("CBZ R15, %sp", label)
		b.Label(label + "q")
		f32rBatchStepNEON(b, r, in, out, inverse, twiddled, 4)
		b.Raw("SUBS $1, R15, R15").
			Raw("BNE %sq", label)
		b.Label(label+"p").
			LoadArg("rem", "R19").
			Raw("TBZ $1, R19, %ss", label)
		f32rBatchStepNEON(b, r, in, out, inverse, twiddled, 2)
		b.Label(label+"s").
			Raw("TBZ $0, R19, %se", label)
		f32rBatchStepNEON(b, r, in, out, inverse, twiddled, 1)
		for j := 0; j < r; j++ { // FLDPS/FSTPS do not post-increment
			b.Raw("ADD $8, %s, %s", in[j], in[j]).
				Raw("ADD $8, %s, %s", out[j], out[j])
		}
		b.Label(label+"e").
			LoadArg("adjin", "R19").
			LoadArg("adjout", "R20")
		for j := 0; j < r; j++ {
			b.Raw("ADD R19, %s, %s", in[j], in[j]).
				Raw("ADD R20, %s, %s", out[j], out[j])
		}
	}
	b.Label("kloop")
	for j := 1; j < r; j++ {
		b.Raw("ADD R16, %s, %s", in[j-1], in[j]).
			Raw("ADD R17, %s, %s", out[j-1], out[j])
	}
	row("zero", false) // point i = 0: no twiddle
	b.LoadArg("ido", "R3").
		Raw("SUBS $1, R3, R3").
		Raw("BEQ knext").
		Raw("MOVD R2, R26")
	b.Label("iloop")
	row("tw", true)
	b.Raw("ADD $%d, R26, R26", (r-1)*8).
		Raw("SUBS $1, R3, R3").
		Raw("BNE iloop")
	b.Label("knext")
	switch r { // input stream 0: r-1 more streams to the next block
	case 2:
		b.Raw("ADD R16, %s, %s", in[0], in[0])
	case 3:
		b.Raw("ADD R16<<1, %s, %s", in[0], in[0])
	case 4:
		b.Raw("ADD R16<<1, %s, %s", in[0], in[0]).Raw("ADD R16, %s, %s", in[0], in[0])
	case 5:
		b.Raw("ADD R16<<2, %s, %s", in[0], in[0])
	case 8:
		b.Raw("ADD R16<<3, %s, %s", in[0], in[0]).Raw("SUB R16, %s, %s", in[0], in[0])
	}
	b.Raw("SUBS $1, R5, R5").
		Raw("BNE kloop").
		Ret()
	f.Add(b.Func())
}

// f32rBatchStepNEON emits one step of a float32 batched pass: g lines (4, 2
// or 1) at one point: load, butterfly, and, when twiddled, outputs 1..r-1
// multiplied by the point's twiddle with f32simdPassStep's fused product,
// then store.
func f32rBatchStepNEON(b *asm.Builder, r int, in, out []string, inverse, twiddled bool, g int) {
	st := f32simdStep{g: g}
	for j := 0; j < r; j++ {
		st.load(b, in[j], 2*j)
	}
	if twiddled {
		b.Raw("MOVD R26, R14")
	}
	f32simdBfly(b, r, inverse, func(j, yr, yi int) {
		if j == 0 || !twiddled {
			st.store(b, yr, out[j])
			return
		}
		b.Raw("VLD2R.P 8(R14), [V28.S4, V29.S4]")
		b.VFMUL4S(30, yr, 28).VFMLS4S(30, yi, 29) // re = yr·wr - yi·wi, yi·wi fused
		if r == 2 {
			b.VFMUL4S(31, yr, 29).VFMLA4S(31, yi, 28) // im = yr·wi + yi·wr, yi·wr fused
		} else {
			b.VFMUL4S(31, yi, 28).VFMLA4S(31, yr, 29) // im = yi·wr + yr·wi, yr·wi fused
		}
		st.store(b, 30, out[j])
	})
}

// f32rRev reverses the four .4S lanes of V(v).
func f32rRev(b *asm.Builder, v int) {
	b.Raw("VREV64 V%d.S4, V%d.S4", v, v).
		Raw("VEXT $8, V%d.B16, V%d.B16, V%d.B16", v, v, v)
}

// genF32rUntangle emits f32rUntangleNEON, or f32rRetangleNEON when inverse.
// R5 = &in[k], R6 = &in[m-k-3], R7 = &out[k], R8 = &out[m-k-3], R9 = &tw[k],
// R4 = groups left; V31 = 0.5 (or h).
func Untangle32(f *emit.File, name string, inverse bool) {
	args := []string{"dst", "z", "tw", "m", "quads"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}
	if inverse {
		args = []string{"z", "x", "tw", "h", "m", "quads"}
		types = []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}
	}
	b := asm.NewFunc(name, asm.Layout(args, types, nil, nil), 0)
	b.LoadArg(args[0], "R0").LoadArg(args[1], "R1").LoadArg("tw", "R2").
		LoadArg("m", "R3").LoadArg("quads", "R4")
	if inverse {
		b.LoadArg("h", "R10").Raw("FMOVS (R10), F31")
	} else {
		b.Raw("FMOVS $(0.5), F31")
	}
	b.Raw("VDUP V31.S[0], V31.S4").
		Raw("LSL $3, R3, R3"). // m·8
		Raw("ADD $8, R1, R5").
		Raw("ADD R3, R1, R6").
		Raw("SUB $32, R6, R6").
		Raw("ADD $8, R0, R7").
		Raw("ADD R3, R0, R8").
		Raw("SUB $32, R8, R8").
		Raw("ADD $8, R2, R9").
		Label("loop").
		Raw("CBZ R4, done").
		Raw("VLD2.P 32(R5), [V0.S4, V1.S4]"). // a: re, im
		Raw("VLD2 (R6), [V2.S4, V3.S4]").     // b, reversed below
		Raw("VLD2.P 32(R9), [V4.S4, V5.S4]")  // wr, wi
	f32rRev(b, 2)
	f32rRev(b, 3)
	b.VFADD4S(6, 0, 2). // s.re
				VFSUB4S(7, 1, 3) // d.im
	if !inverse {
		b.VFADD4S(8, 1, 3). // s.im
					VFSUB4S(9, 0, 2).   // d.re
					VFMUL4S(10, 8, 31). // xor = s.im·0.5
					VFMUL4S(11, 9, 31). // d.re·0.5
					VFNEG4S(11, 11).    // xoi
					VFMUL4S(12, 10, 4). // wr·xor
					VFMLS4S(12, 5, 11). // tr = wr·xor − wi·xoi
					VFMUL4S(13, 11, 4). // wr·xoi
					VFMLA4S(13, 5, 10). // ti = wr·xoi + wi·xor
					VFNEG4S(14, 12).
					VFMLA4S(14, 6, 31). // dst[m-k].re = s.re·0.5 − tr
					VFMLA4S(12, 6, 31). // dst[k].re = tr + s.re·0.5
					VFNEG4S(15, 13).
					VFMLA4S(15, 7, 31). // d.im·0.5 − ti
					VFNEG4S(15, 15).    // dst[m-k].im
					VFMLA4S(13, 7, 31)  // dst[k].im = ti + d.im·0.5
	} else {
		b.VFSUB4S(8, 0, 2). // d.re
					VFMUL4S(8, 8, 31).  // dr = d.re·h
					VFADD4S(9, 1, 3).   // s.im
					VFMUL4S(9, 9, 31).  // di = s.im·h
					VFMUL4S(13, 8, 4).  // wr·dr
					VFMLA4S(13, 9, 5).  // xor = wr·dr + wi·di
					VFMUL4S(14, 9, 4).  // wr·di
					VFMLS4S(14, 8, 5).  // xoi = wr·di − wi·dr
					VFNEG4S(12, 14).    //
					VFMLA4S(12, 6, 31). // z[k].re = h·s.re − xoi
					VFMLA4S(14, 6, 31). // z[m-k].re = xoi + h·s.re
					VFNEG4S(15, 13).
					VFMLA4S(15, 7, 31). // h·d.im − xor
					VFNEG4S(15, 15).    // z[m-k].im
					VFMLA4S(13, 7, 31)  // z[k].im = xor + h·d.im
	}
	f32rRev(b, 14)
	f32rRev(b, 15)
	b.Raw("VST2.P [V12.S4, V13.S4], 32(R7)").
		Raw("VST2 [V14.S4, V15.S4], (R8)").
		Raw("SUB $32, R6, R6").
		Raw("SUB $32, R8, R8").
		Raw("SUB $1, R4, R4").
		Raw("B loop").
		Label("done").
		Ret()
	f.Add(b.Func())
}
