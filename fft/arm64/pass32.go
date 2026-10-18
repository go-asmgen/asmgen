package arm64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// float32 (complex64) Stockham pass kernels.
//
// Pass32's name(cc, ch, tw *complex64, ido, l1 int) and Last32's
// name(cc, ch *complex64, l1 int) run go-fft's
// float32 passes (stockham32_passes.go, f32Pass<r> and f32Pass<r>last) for
// radix 2, 3, 4, 5 and 8, four points per register: VLD2 splits four
// complex64 into a register of real parts and one of imaginary parts (.4S),
// every lane then runs the Go pass's operation for one point, and VST2
// interleaves on store. They are the float64 kernels above with
// single-precision registers, in their own functions (f32simd*) so a change
// to one family cannot move the other's arithmetic.
//
// Bit identity. gc compiles the float32 passes with the same fused
// multiply-adds as the float64 ones (FMADDS/FMSUBS where those have
// FMADDD/FMSUBD, go build -gcflags=-S, Go 1.27.1: pass2, pass3, pass4 and
// every final pass emit the same sequence of operations; pass5, pass7 and
// pass8 the same operations scheduled differently), so the kernels fuse the
// products the float64 kernels fuse: see the comment above step. The tests
// compare kernel and Go pass bit for bit.
//
// Points are walked in groups of four (one .4S register pair), then a group
// of two (VLD2/VST2 on .2S, the upper lanes zero) for ido&2 and a lone point
// (FLDPS/FSTPS) for ido&1. Twiddles come in the same groups, one sequential
// stream already split (go-fft's StockhamTwiddles32 on arm64): for a group of
// g points and each j, the g real parts then the g imaginary parts, loaded
// with VLD1 (FLDPS for a lone point). The point i = 0 is multiplied by w = 1
// with its group and lane 0 of the untwiddled value put back (VMOV element).
//
// Registers as in the float64 kernels, with S = ido·8 and OS = l1·ido·8.

// f32simdStep is one call of a float32 pass body: g points (4, 2 or 1), the
// first group of a block (whose lane 0 is restored) or another.
type f32simdStep struct {
	g     int
	first bool
}

func (st f32simdStep) load(b *asm.Builder, ptr string, v int) {
	switch st.g {
	case 4:
		b.Raw("VLD2.P 32(%s), [V%d.S4, V%d.S4]", ptr, v, v+1)
	case 2:
		b.Raw("VLD2.P 16(%s), [V%d.S2, V%d.S2]", ptr, v, v+1)
	default:
		b.Raw("FLDPS (%s), (F%d, F%d)", ptr, v, v+1)
	}
}

func (st f32simdStep) store(b *asm.Builder, v int, ptr string) {
	switch st.g {
	case 4:
		b.Raw("VST2.P [V%d.S4, V%d.S4], 32(%s)", v, v+1, ptr)
	case 2:
		b.Raw("VST2.P [V%d.S2, V%d.S2], 16(%s)", v, v+1, ptr)
	default:
		b.Raw("FSTPS (F%d, F%d), (%s)", v, v+1, ptr)
	}
}

// twLoad loads the next twiddle of the group into V28 (real parts) and V29
// (imaginary parts), and moves R14 past it.
func (st f32simdStep) twLoad(b *asm.Builder) {
	switch st.g {
	case 4:
		b.Raw("VLD1.P 32(R14), [V28.S4, V29.S4]")
	case 2:
		b.Raw("VLD1.P 16(R14), [V28.S2, V29.S2]")
	default:
		b.Raw("FLDPS (R14), (F28, F29)").Raw("ADD $8, R14")
	}
}

// f32simdConsts is skConsts in float32: each constant rounded once to float32
// (cmd/asm rounds the decimal to float64 and then to float32; for these
// constants that equals rounding the exact value once, which the bit-exact
// tests confirm) and duplicated into four lanes. h is the register for √2/2.
func f32simdConsts(b *asm.Builder, r, h int) {
	dup := func(v int, c string) {
		b.Raw("FMOVS $(%s), F%d", c, v).Raw("VDUP V%d.S[0], V%d.S4", v, v)
	}
	switch r {
	case 3:
		dup(24, "0.5")
		dup(25, "0.8660254037844386467637231707529361834714026269051903140279") // sin(2π/3)
	case 5:
		dup(24, "0.30901699437494742410229341718281905886015458990288")  // c51 = cos(2π/5)
		dup(25, "-0.80901699437494742410229341718281905886015458990289") // c52 = cos(4π/5)
		dup(26, "0.95105651629515357211643933337938214340569863412575")  // s51 = sin(2π/5)
		dup(27, "0.58778525229247312916870595463907276859765243764314")  // s52 = sin(4π/5)
	case 8:
		dup(h, "0.707106781186547524400844362104849") // √2/2
	}
}

func f32simdBfly(b *asm.Builder, r int, inverse bool, out func(j, yr, yi int)) {
	map[int]func(*asm.Builder, bool, func(j, yr, yi int)){2: f32simdBfly2, 3: f32simdBfly3, 4: f32simdBfly4, 5: f32simdBfly5, 8: f32simdBfly8}[r](b, inverse, out)
}

// f32simdSops is sops on .4S.
func f32simdSops(b *asm.Builder, inverse bool) (add, sub, aMinusSz, aPlusSz func(d, n, m int)) {
	add = func(d, n, m int) { b.VFADD4S(d, n, m) }
	sub = func(d, n, m int) { b.VFSUB4S(d, n, m) }
	pm := func(d, n, m int, plus bool) {
		if plus {
			add(d, n, m)
		} else {
			sub(d, n, m)
		}
	}
	aMinusSz = func(d, a, z int) { pm(d, a, z, !inverse) }
	aPlusSz = func(d, a, z int) { pm(d, a, z, inverse) }
	return
}

// f32simdBfly2 … f32simdBfly8 are bfly2 … bfly8 on .4S, operation for
// operation.
func f32simdBfly2(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	b.VFADD4S(4, 0, 2).VFADD4S(5, 1, 3)
	out(0, 4, 5)
	b.VFSUB4S(4, 0, 2).VFSUB4S(5, 1, 3)
	out(1, 4, 5)
}

func f32simdBfly3(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	add, sub, aMinusSz, aPlusSz := f32simdSops(b, inverse)
	add(6, 2, 4) // t1
	add(7, 3, 5)
	sub(2, 2, 4) // t2
	sub(3, 3, 5)
	add(8, 0, 6) // y0
	add(9, 1, 7)
	out(0, 8, 9)
	b.VFMLS4S(0, 6, 24).VFMLS4S(1, 7, 24) // ca, in place on x0
	b.VFMUL4S(2, 2, 25).VFMUL4S(3, 3, 25) // q, in place on t2
	aMinusSz(10, 0, 3)                    // y1 = ca + rotS(q)
	aPlusSz(11, 1, 2)
	out(1, 10, 11)
	aPlusSz(10, 0, 3) // y2 = ca - rotS(q)
	aMinusSz(11, 1, 2)
	out(2, 10, 11)
}

func f32simdBfly4(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	add, sub, aMinusSz, aPlusSz := f32simdSops(b, inverse)
	add(8, 0, 4) // t2
	add(9, 1, 5)
	sub(10, 0, 4) // t1
	sub(11, 1, 5)
	add(12, 2, 6) // t3
	add(13, 3, 7)
	sub(14, 2, 6) // t4
	sub(15, 3, 7)
	add(16, 8, 12) // y0
	add(17, 9, 13)
	out(0, 16, 17)
	aMinusSz(0, 10, 15) // y1 = t1 + rotS(t4)
	aPlusSz(1, 11, 14)
	out(1, 0, 1)
	sub(16, 8, 12) // y2
	sub(17, 9, 13)
	out(2, 16, 17)
	aPlusSz(0, 10, 15) // y3 = t1 - rotS(t4)
	aMinusSz(1, 11, 14)
	out(3, 0, 1)
}

func f32simdBfly5(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	add, sub, aMinusSz, aPlusSz := f32simdSops(b, inverse)
	add(10, 2, 8) // t1
	add(11, 3, 9)
	sub(2, 2, 8) // t2
	sub(3, 3, 9)
	add(12, 4, 6) // t3
	add(13, 5, 7)
	sub(4, 4, 6) // t4
	sub(5, 5, 7)
	add(14, 0, 10) // y0 = (a + t1) + t3
	add(15, 1, 11)
	add(14, 14, 12)
	add(15, 15, 13)
	out(0, 14, 15)
	for k := 0; k < 2; k++ {
		b.VFMUL4S(16+k, 12+k, 25).VFMLA4S(16+k, 10+k, 24) // c51·t1 + c52·t3
		add(16+k, 0+k, 16+k)                              // r1
		b.VFMUL4S(18+k, 12+k, 24).VFMLA4S(18+k, 10+k, 25) // c52·t1 + c51·t3
		add(18+k, 0+k, 18+k)                              // r2
		b.VFMUL4S(20+k, 4+k, 27).VFMLA4S(20+k, 2+k, 26)   // u = s51·t2 + s52·t4
		b.VFMUL4S(22+k, 2+k, 27).VFMLS4S(22+k, 4+k, 26)   // v = s52·t2 - s51·t4
	}
	aMinusSz(0, 16, 21) // y1 = r1 + rotS(u)
	aPlusSz(1, 17, 20)
	out(1, 0, 1)
	aMinusSz(0, 18, 23) // y2 = r2 + rotS(v)
	aPlusSz(1, 19, 22)
	out(2, 0, 1)
	aPlusSz(0, 18, 23) // y3 = r2 - rotS(v)
	aMinusSz(1, 19, 22)
	out(3, 0, 1)
	aPlusSz(0, 16, 21) // y4 = r1 - rotS(u)
	aMinusSz(1, 17, 20)
	out(4, 0, 1)
}

// f32simdBfly8 is bfly8; √2/2 is in V18.
func f32simdBfly8(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	add, sub, aMinusSz, aPlusSz := f32simdSops(b, inverse)
	add(16, 2, 10) // a1 = x1 + x5
	add(17, 3, 11)
	sub(2, 2, 10) // a5 = x1 - x5
	sub(3, 3, 11)
	add(28, 6, 14) // a3 = x3 + x7
	add(29, 7, 15)
	sub(6, 6, 14) // a7 = x3 - x7
	sub(7, 7, 15)
	add(20, 16, 28) // a1 = a1 + a3
	add(21, 17, 29)
	sub(16, 16, 28) // a3 = a1 - a3, rotated where it is used
	sub(17, 17, 29)
	aMinusSz(22, 2, 7) // a5 = a5 + rotS(a7)
	aPlusSz(23, 3, 6)
	aPlusSz(24, 2, 7) // a7 = a5 - rotS(a7)
	aMinusSz(25, 3, 6)
	aMinusSz(2, 22, 23) // P
	aPlusSz(3, 23, 22)  // Q
	if inverse {
		b.VFNEG4S(6, 24)
		sub(6, 6, 25)  // R = -a7r - a7i
		sub(7, 24, 25) // T = -a7i + a7r
	} else {
		sub(6, 25, 24) // R = -a7r + a7i
		b.VFNEG4S(7, 25)
		sub(7, 7, 24) // T = -a7i - a7r
	}
	add(22, 0, 8) // a0 = x0 + x4
	add(23, 1, 9)
	sub(0, 0, 8) // a4 = x0 - x4
	sub(1, 1, 9)
	add(24, 4, 12) // a2 = x2 + x6
	add(25, 5, 13)
	sub(4, 4, 12) // a6 = x2 - x6
	sub(5, 5, 13)
	add(26, 22, 24) // a0 = a0 + a2
	add(27, 23, 25)
	sub(22, 22, 24) // a2 = a0 - a2
	sub(23, 23, 25)
	aMinusSz(8, 0, 5) // a4 = a4 + rotS(a6)
	aPlusSz(9, 1, 4)
	aPlusSz(10, 0, 5) // a6 = a4 - rotS(a6)
	aMinusSz(11, 1, 4)
	add(12, 26, 20) // y0 = a0 + a1
	add(13, 27, 21)
	out(0, 12, 13)
	b.Raw("VMOV V8.B16, V14.B16").Raw("VMOV V9.B16, V15.B16") // y1 = a4 + h·(P, Q)
	b.VFMLA4S(14, 2, 18).VFMLA4S(15, 3, 18)
	out(1, 14, 15)
	aMinusSz(14, 22, 17) // y2 = a2 + rotS(a3)
	aPlusSz(15, 23, 16)
	out(2, 14, 15)
	b.Raw("VMOV V10.B16, V14.B16").Raw("VMOV V11.B16, V15.B16") // y3 = a6 + h·(R, T)
	b.VFMLA4S(14, 6, 18).VFMLA4S(15, 7, 18)
	out(3, 14, 15)
	sub(14, 26, 20) // y4 = a0 - a1
	sub(15, 27, 21)
	out(4, 14, 15)
	b.VFMLS4S(8, 2, 18).VFMLS4S(9, 3, 18) // y5 = a4 - h·(P, Q), in place
	out(5, 8, 9)
	aPlusSz(14, 22, 17) // y6 = a2 - rotS(a3)
	aMinusSz(15, 23, 16)
	out(6, 14, 15)
	b.VFMLS4S(10, 6, 18).VFMLS4S(11, 7, 18) // y7 = a6 - h·(R, T), in place
	out(7, 10, 11)
}

// f32simdPassStep emits one step of a float32 radix-r pass: passStep with
// the fusions of the twiddle product as gc compiles f32Mul in each pass.
func f32simdPassStep(b *asm.Builder, r int, in, out []string, inverse bool, st f32simdStep) {
	for j := 0; j < r; j++ {
		st.load(b, in[j], 2*j)
	}
	f32simdBfly(b, r, inverse, func(j, yr, yi int) {
		if j == 0 {
			st.store(b, yr, out[0])
			return
		}
		st.twLoad(b)
		b.VFMUL4S(30, yr, 28).VFMLS4S(30, yi, 29) // re = yr·wr - yi·wi, yi·wi fused
		if r == 2 {
			b.VFMUL4S(31, yr, 29).VFMLA4S(31, yi, 28) // im = yr·wi + yi·wr, yi·wr fused
		} else {
			b.VFMUL4S(31, yi, 28).VFMLA4S(31, yr, 29) // im = yi·wr + yr·wi, yr·wi fused
		}
		if st.first {
			b.Raw("VMOV V%d.S[0], V30.S[0]", yr).Raw("VMOV V%d.S[0], V31.S[0]", yi)
		}
		st.store(b, 30, out[j])
	})
}

// Pass32 emits name(cc, ch, tw *complex64, ido, l1 int), the float32 pass
// (ido >= 2).
func Pass32(f *emit.File, name string, r int, inverse bool) {
	check("Pass32", r, 2, 3, 4, 5, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	in, out := skStreams[r][0], skStreams[r][1]
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("cc", "R19").LoadArg("ch", "R20").LoadArg("tw", "R2").
		LoadArg("ido", "R3").LoadArg("l1", "R5").
		Raw("LSL $3, R3, R16"). // S: one input stream
		Raw("MUL R5, R16, R17") // OS: one output stream
	next := ""
	switch r {
	case 2:
		next = "ADD R16<<1, R19, R19"
	case 4:
		next = "ADD R16<<2, R19, R19"
	case 8:
		next = "ADD R16<<3, R19, R19"
	default:
		b.Raw("MOVD $%d, R4", r).Raw("MUL R4, R16, R4")
		next = "ADD R4, R19, R19"
	}
	f32simdConsts(b, r, 18)
	b.Label("kloop").Raw("MOVD R19, %s", in[0])
	for j := 1; j < r; j++ {
		b.Raw("ADD R16, %s, %s", in[j-1], in[j])
	}
	b.Raw("MOVD R20, %s", out[0])
	for j := 1; j < r; j++ {
		b.Raw("ADD R17, %s, %s", out[j-1], out[j])
	}
	b.Raw("MOVD R2, R14").
		Raw("LSR $2, R3, R15"). // groups of four points
		Raw("CBZ R15, firstpair")
	f32simdPassStep(b, r, in, out, inverse, f32simdStep{g: 4, first: true})
	b.Raw("SUBS $1, R15, R15").
		Raw("BEQ quadsdone")
	b.Label("qloop")
	f32simdPassStep(b, r, in, out, inverse, f32simdStep{g: 4})
	b.Raw("SUBS $1, R15, R15").
		Raw("BNE qloop")
	b.Label("quadsdone").
		Raw("TBZ $1, R3, single") // ido&2: a group of two
	f32simdPassStep(b, r, in, out, inverse, f32simdStep{g: 2})
	b.Raw("B single")
	b.Label("firstpair") // ido is 2 or 3: the first group is the pair
	f32simdPassStep(b, r, in, out, inverse, f32simdStep{g: 2, first: true})
	b.Label("single").
		Raw("TBZ $0, R3, knext") // ido&1: one point left
	f32simdPassStep(b, r, in, out, inverse, f32simdStep{g: 1})
	b.Label("knext").
		Raw("%s", next).
		Raw("ADD R16, R20, R20").
		Raw("SUBS $1, R5, R5").
		Raw("BNE kloop").
		Ret()
	f.Add(b.Func())
}

// f32simdUzp splits the pairs of complex64 p (blocks k, k+1) and q (blocks
// k+2, k+3) into the real parts of the four blocks, V(d), and their
// imaginary parts, V(d+1). d may be p (the imaginary parts are taken first);
// d+1 must be neither p nor q.
func f32simdUzp(b *asm.Builder, p, q, d int) {
	b.Raw("VUZP2 V%d.S4, V%d.S4, V%d.S4", q, p, d+1).
		Raw("VUZP1 V%d.S4, V%d.S4, V%d.S4", q, p, d)
}

// f32simdLastLoad loads four blocks of r points (4r complex64 from R0) and
// leaves point j of the four blocks split in V(2j) (real parts) and V(2j+1)
// (imaginary parts). Viewed as 64-bit lanes, one register pair p, q holds
// point j of blocks k, k+1 and of blocks k+2, k+3, and VUZP1/VUZP2 split
// them. Radix 2, 3 and 4 get p and q straight from VLD2/VLD3/VLD4 on .2D
// lanes (one per block pair); radix 8 from VLD4 on one block at a time,
// whose points j and j+4 share a register, paired across blocks with
// VZIP1/VZIP2; radix 5, which has no structure load, from plain VLD1 loads
// (T0..T4 = points 0-1, 2-3, 4|0, 1-2, 3-4 of a block pair), with VEXT for
// the pairs that straddle two registers and an element move (VMOV D[1]) for
// the others.
func f32simdLastLoad(b *asm.Builder, r int) {
	regs := func(first, n int) string {
		s := ""
		for i := 0; i < n; i++ {
			if i > 0 {
				s += ", "
			}
			s += fmt.Sprintf("V%d.D2", first+i)
		}
		return s
	}
	switch r {
	case 2, 3, 4:
		b.Raw("VLD%d.P %d(R0), [%s]", r, 16*r, regs(16, r)).
			Raw("VLD%d.P %d(R0), [%s]", r, 16*r, regs(20, r))
		for j := 0; j < r; j++ {
			f32simdUzp(b, 16+j, 20+j, 2*j)
		}
	case 8:
		for q := 0; q < 4; q++ {
			b.Raw("VLD4.P 64(R0), [%s]", regs(16+4*q, 4))
		}
		for i := 0; i < 4; i++ {
			b.Raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", 20+i, 16+i, 2*i).
				Raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", 20+i, 16+i, 8+2*i).
				Raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", 28+i, 24+i, 16+i).
				Raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", 28+i, 24+i, 20+i)
			f32simdUzp(b, 2*i, 16+i, 2*i)
			f32simdUzp(b, 8+2*i, 20+i, 8+2*i)
		}
	case 5:
		b.Raw("VLD1.P 64(R0), [%s]", regs(16, 4)).
			Raw("VLD1.P 16(R0), [V20.D2]").
			Raw("VLD1.P 32(R0), [V21.D2, V22.D2]").
			Raw("VLD1.P 16(R0), [V23.D2]").
			Raw("VLD1.P 32(R0), [V28.D2, V29.D2]")
		pairs := func(t [5]int, e1, e3 int) [5]int {
			b.Raw("VEXT $8, V%d.B16, V%d.B16, V%d.B16", t[3], t[0], e1). // points 1 | 1
											Raw("VEXT $8, V%d.B16, V%d.B16, V%d.B16", t[4], t[1], e3). // points 3 | 3
											Raw("VMOV V%d.D[1], V%d.D[1]", t[2], t[0]).                // points 0 | 0
											Raw("VMOV V%d.D[1], V%d.D[1]", t[3], t[1]).                // points 2 | 2
											Raw("VMOV V%d.D[1], V%d.D[1]", t[4], t[2])                 // points 4 | 4
			return [5]int{t[0], e1, t[1], e3, t[2]}
		}
		p := pairs([5]int{16, 17, 18, 19, 20}, 10, 11)
		q := pairs([5]int{21, 22, 23, 28, 29}, 12, 13)
		for j := 0; j < 5; j++ {
			f32simdUzp(b, p[j], q[j], 2*j)
		}
	}
}

// Last32 emits name(cc, ch
// *complex64, l1 int): the final pass (ido == 1) for l1 a multiple of four,
// four blocks per iteration (f32simdLastLoad); output j of the four blocks,
// ch[j·l1+k .. j·l1+k+3], is one VST2.
func Last32(f *emit.File, name string, r int, inverse bool) {
	check("Last32", r, 2, 3, 4, 5, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	out := skStreams[r][1]
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("cc", "R0").LoadArg("ch", out[0]).LoadArg("l1", "R4").
		Raw("LSL $3, R4, R17")
	for j := 1; j < r; j++ {
		b.Raw("ADD R17, %s, %s", out[j-1], out[j])
	}
	b.Raw("LSR $2, R4, R5")
	if r != 8 {
		f32simdConsts(b, r, 18)
	}
	b.Label("loop")
	f32simdLastLoad(b, r)
	if r == 8 {
		// √2/2 again: the loads above used V18.
		f32simdConsts(b, r, 18)
	}
	f32simdBfly(b, r, inverse, func(j, yr, yi int) {
		b.Raw("VST2.P [V%d.S4, V%d.S4], 32(%s)", yr, yi, out[j])
	})
	b.Raw("SUBS $1, R5, R5").
		Raw("BNE loop").
		Ret()
	f.Add(b.Func())
}
