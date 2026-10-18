package arm64

import (
	asm "github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

// The Stockham kernels run stockham.go's radix-2, 3, 4, 5 and 8 passes
// (pass2 ... pass8) and the radix-2, 3, 4 and 5 final passes (pass2last ...
// pass5last) two points per NEON register, deinterleaved: VLD2
// splits two complex128 into a register of real parts and one of imaginary
// parts, every lane then runs exactly the scalar operation the Go pass runs on
// one point, and VST2 interleaves on store.
//
// Bit identity with the Go passes, which gc compiles on arm64 with fused
// multiply-adds. What gc fuses was read from go build -gcflags=-S (Go 1.27.1);
// each kernel copies it, and the tests compare kernel and Go pass bit for bit,
// signed zeros and infinities included:
//
//   - The butterfly's additions and subtractions are plain FADDD/FSUBD, so
//     VFADD/VFSUB.
//   - The ±i rotation rotS multiplies by s = ±1 and gc fuses that product into
//     the add or subtract that uses it. A product by ±1 is exact, so the result
//     is that of a plain add or subtract of ±z: the kernels exchange the real
//     and imaginary registers and choose VFADD or VFSUB (x - y is x + (-y) in
//     IEEE 754, signed zeros included). A sum of two negated operands, which
//     gc forms with FNMADDD/FNMSUBD, is written (-a) - b with a VFNEG, never
//     -(a + b), which differs on a zero sum.
//   - The twiddle product y·w compiles to re = FMULD(yr, wr) then FMSUBD
//     re - yi·wi, and, in pass3, pass4, pass5 and pass8, im = FMULD(yi, wr)
//     then FMADDD im + yr·wi: the products yr·wr and yi·wr are rounded, yi·wi
//     and yr·wi fused. In pass2 the imaginary part is the other way round, im =
//     FMULD(yr, wi) then FMADDD im + yi·wr (as in cmul.go's CMulScalar): which
//     product gc rounds depends on the code around it, so each kernel copies
//     the pass it replaces, with VFMUL, VFMLS and VFMLA.
//   - Radix 3 fuses 0.5·t1 into x0 - 0.5·t1 and rounds sin(2π/3)·t2.
//   - Radix 5: c·x + c'·y rounds c'·y and fuses c·x; c·x - c'·y rounds c·x and
//     fuses c'·y.
//   - Radix 8 computes its ∓45°/∓135° rotations unscaled and fuses their
//     √2/2 into the final add or subtract (see bfly8).
//
// Twiddles are read from one sequential stream (go-fft's StockhamTwiddles on
// arm64), already split: for each pair of points i, i+1 and each j, the four
// float64 re(w_j(i)) re(w_j(i+1)) im(w_j(i)) im(w_j(i+1)), so a VLD1 loads
// them as they are used. The pair i = 0, 1 is multiplied with w(0) = 1 like the
// others; the Go pass does not multiply point 0, and a multiply by one is not
// exact on signed zeros and infinities, so lane 0 of the untwiddled result is
// put back (VMOV element) before the store. An odd ido ends with one point
// alone (see step).
//
// Register use in a pass kernel: R2 tw, R3 ido, R5 the block counter, R14 the
// twiddle cursor, R15 the pair counter, R16 = ido·16 (one input stream), R17 =
// l1·ido·16 (one output stream), R4 = r·ido·16 (one input block, radix 3 and
// 5), R19 the block's input, R20 its output, and skStreams the streams. V28
// and V29 hold a twiddle, V30 and V31 the product; the constants are in V18
// (radix 8) and V24-V27 (radix 3 and 5).

// step is what one call of a pass body emits: the pair of points i = 0, 1
// (whose lane 0 is restored untwiddled), a later pair, or the last point of an
// odd ido. A lone point is loaded into lane 0 with a scalar FLDPD (which
// zeroes lane 1), runs through the same vector code with a dummy lane 1, and
// lane 0 is stored with FSTPD; its twiddles come as a pair whose second half is
// zero.
type step int

const (
	firstPair step = iota
	nextPair
	lastPoint
)

// load loads the next point(s) of the stream at ptr: real parts into V(v),
// imaginary parts into V(v+1).
func (st step) load(b *asm.Builder, ptr string, v int) {
	if st == lastPoint {
		b.Raw("FLDPD (%s), (F%d, F%d)", ptr, v, v+1)
		return
	}
	b.Raw("VLD2.P 32(%s), [V%d.D2, V%d.D2]", ptr, v, v+1)
}

// store stores V(v) (real parts) and V(v+1) (imaginary parts) to the stream
// at ptr.
func (st step) store(b *asm.Builder, v int, ptr string) {
	if st == lastPoint {
		b.Raw("FSTPD (F%d, F%d), (%s)", v, v+1, ptr)
		return
	}
	b.Raw("VST2.P [V%d.D2, V%d.D2], 32(%s)", v, v+1, ptr)
}

// skStreams holds, per radix, the registers that walk a pass's input and
// output streams.
var skStreams = map[int][2][]string{
	2: {{"R6", "R7"}, {"R10", "R11"}},
	3: {{"R6", "R7", "R8"}, {"R10", "R11", "R12"}},
	4: {{"R6", "R7", "R8", "R9"}, {"R10", "R11", "R12", "R13"}},
	5: {{"R6", "R7", "R8", "R9", "R10"}, {"R11", "R12", "R13", "R21", "R22"}},
	8: {{"R0", "R1", "R4", "R6", "R7", "R8", "R9", "R10"}, {"R11", "R12", "R13", "R21", "R22", "R23", "R24", "R25"}},
}

// skConsts emits the constants a radix's butterfly reads, each duplicated
// into both lanes of its register.
func skConsts(b *asm.Builder, r int) {
	dup := func(v int, c string) {
		b.Raw("FMOVD $(%s), F%d", c, v).Raw("VDUP V%d.D[0], V%d.D2", v, v)
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
		dup(18, "0.707106781186547524400844362104849") // √2/2
	}
}

// bfly emits the radix-r butterfly on x_j in V(2j), V(2j+1), calling out(j,
// yr, yi) for every output j in ascending order once y_j is in (yr, yi).
// Between calls, out may use V28-V31.
func bfly(b *asm.Builder, r int, inverse bool, out func(j, yr, yi int)) {
	map[int]func(*asm.Builder, bool, func(j, yr, yi int)){2: bfly2, 3: bfly3, 4: bfly4, 5: bfly5, 8: bfly8}[r](b, inverse, out)
}

// sops returns the add, the subtract, and the two ±s·z forms: aMinusSz(d, a,
// z) is a - s·z (forward a + z, inverse a - z) and aPlusSz a + s·z, s being
// the direction sign of stockham.go (-1 forward).
func sops(b *asm.Builder, inverse bool) (add, sub, aMinusSz, aPlusSz func(d, n, m int)) {
	add = func(d, n, m int) { b.VFADD2D(d, n, m) }
	sub = func(d, n, m int) { b.VFSUB2D(d, n, m) }
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

// bfly2 is the radix-2 butterfly of pass2 and pass2last: y0 = x0 + x1, y1 =
// x0 - x1.
func bfly2(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	b.VFADD2D(4, 0, 2).VFADD2D(5, 1, 3)
	out(0, 4, 5)
	b.VFSUB2D(4, 0, 2).VFSUB2D(5, 1, 3)
	out(1, 4, 5)
}

// bfly3 is stockham.go's bfly3: t1 = x1 + x2, t2 = x1 - x2, ca = x0 - 0.5·t1
// (fused), q = sin(2π/3)·t2 (rounded), y0 = x0 + t1, y1 = ca + rotS(q), y2 =
// ca - rotS(q).
func bfly3(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	add, sub, aMinusSz, aPlusSz := sops(b, inverse)
	add(6, 2, 4) // t1
	add(7, 3, 5)
	sub(2, 2, 4) // t2
	sub(3, 3, 5)
	add(8, 0, 6) // y0
	add(9, 1, 7)
	out(0, 8, 9)
	b.VFMLS2D(0, 6, 24).VFMLS2D(1, 7, 24) // ca, in place on x0
	b.VFMUL2D(2, 2, 25).VFMUL2D(3, 3, 25) // q, in place on t2
	aMinusSz(10, 0, 3)                    // y1 = ca + rotS(q)
	aPlusSz(11, 1, 2)
	out(1, 10, 11)
	aPlusSz(10, 0, 3) // y2 = ca - rotS(q)
	aMinusSz(11, 1, 2)
	out(2, 10, 11)
}

// bfly4 is stockham.go's bfly4: t2 = x0 + x2, t1 = x0 - x2, t3 = x1 + x3, t4
// = x1 - x3, y0 = t2 + t3, y1 = t1 + rotS(t4), y2 = t2 - t3, y3 = t1 - rotS(t4).
func bfly4(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	add, sub, aMinusSz, aPlusSz := sops(b, inverse)
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

// bfly5 is stockham.go's bfly5 (inlined by hand in pass5): t1 = x1 + x4, t2 =
// x1 - x4, t3 = x2 + x3, t4 = x2 - x3; r1 = a + (c51·t1 + c52·t3), r2 = a +
// (c52·t1 + c51·t3), u = s51·t2 + s52·t4, v = s52·t2 - s51·t4, each a product
// rounded and the other fused as described above; y0 = (a + t1) + t3, y1 = r1
// + rotS(u), y2 = r2 + rotS(v), y3 = r2 - rotS(v), y4 = r1 - rotS(u).
func bfly5(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	add, sub, aMinusSz, aPlusSz := sops(b, inverse)
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
		b.VFMUL2D(16+k, 12+k, 25).VFMLA2D(16+k, 10+k, 24) // c51·t1 + c52·t3
		add(16+k, 0+k, 16+k)                              // r1
		b.VFMUL2D(18+k, 12+k, 24).VFMLA2D(18+k, 10+k, 25) // c52·t1 + c51·t3
		add(18+k, 0+k, 18+k)                              // r2
		b.VFMUL2D(20+k, 4+k, 27).VFMLA2D(20+k, 2+k, 26)   // u = s51·t2 + s52·t4
		b.VFMUL2D(22+k, 2+k, 27).VFMLS2D(22+k, 4+k, 26)   // v = s52·t2 - s51·t4
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

// bfly8 is stockham.go's bfly8 as gc compiles it in pass8 and pass8last:
// beyond the ±s·z fusions, the ∓45°/∓135° rotations are computed unscaled,
// P = a5r - s·a5i, Q = a5i + s·a5r, R = -a7r - s·a7i (FNMADDD), T = -a7i +
// s·a7r (FNMSUBD), and their scaling by h = √2/2 is fused into the final add
// or subtract: y1 = a4 + h·(P, Q), y5 = a4 - h·(P, Q), y3 = a6 + h·(R, T),
// y7 = a6 - h·(R, T), each one FMADDD/FMSUBD. The kernel does the same with
// VFMLA/VFMLS on a copy of a4 and a6. h is in V18.
func bfly8(b *asm.Builder, inverse bool, out func(j, yr, yi int)) {
	add, sub, aMinusSz, aPlusSz := sops(b, inverse)
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
		b.VFNEG2D(6, 24)
		sub(6, 6, 25)  // R = -a7r - a7i
		sub(7, 24, 25) // T = -a7i + a7r
	} else {
		sub(6, 25, 24) // R = -a7r + a7i
		b.VFNEG2D(7, 25)
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
	b.VFMLA2D(14, 2, 18).VFMLA2D(15, 3, 18)
	out(1, 14, 15)
	aMinusSz(14, 22, 17) // y2 = a2 + rotS(a3)
	aPlusSz(15, 23, 16)
	out(2, 14, 15)
	b.Raw("VMOV V10.B16, V14.B16").Raw("VMOV V11.B16, V15.B16") // y3 = a6 + h·(R, T)
	b.VFMLA2D(14, 6, 18).VFMLA2D(15, 7, 18)
	out(3, 14, 15)
	sub(14, 26, 20) // y4 = a0 - a1
	sub(15, 27, 21)
	out(4, 14, 15)
	b.VFMLS2D(8, 2, 18).VFMLS2D(9, 3, 18) // y5 = a4 - h·(P, Q), in place
	out(5, 8, 9)
	aPlusSz(14, 22, 17) // y6 = a2 - rotS(a3)
	aMinusSz(15, 23, 16)
	out(6, 14, 15)
	b.VFMLS2D(10, 6, 18).VFMLS2D(11, 7, 18) // y7 = a6 - h·(R, T), in place
	out(7, 10, 11)
}

// passStep emits one step of a radix-r pass: load the point(s), run the
// butterfly, multiply outputs 1..r-1 by their twiddles (V28/V29 ← w, V30/V31
// ← y·w) and store.
func passStep(b *asm.Builder, r int, in, out []string, inverse bool, st step) {
	for j := 0; j < r; j++ {
		st.load(b, in[j], 2*j)
	}
	bfly(b, r, inverse, func(j, yr, yi int) {
		if j == 0 {
			st.store(b, yr, out[0])
			return
		}
		b.Raw("VLD1.P 32(R14), [V28.D2, V29.D2]")
		b.VFMUL2D(30, yr, 28).VFMLS2D(30, yi, 29) // re = yr·wr - yi·wi, yi·wi fused
		if r == 2 {
			b.VFMUL2D(31, yr, 29).VFMLA2D(31, yi, 28) // im = yr·wi + yi·wr, yi·wr fused
		} else {
			b.VFMUL2D(31, yi, 28).VFMLA2D(31, yr, 29) // im = yi·wr + yr·wi, yr·wi fused
		}
		if st == firstPair {
			b.Raw("VMOV V%d.D[0], V30.D[0]", yr).Raw("VMOV V%d.D[0], V31.D[0]", yi)
		}
		st.store(b, 30, out[j])
	})
}

// Pass emits name(cc, ch, tw
// *complex128, ido, l1 int): the radix-r pass for ido >= 2.
func Pass(f *emit.File, name string, r int, inverse bool) {
	check("Pass", r, 2, 3, 4, 5, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	in, out := skStreams[r][0], skStreams[r][1]
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("cc", "R19").LoadArg("ch", "R20").LoadArg("tw", "R2").
		LoadArg("ido", "R3").LoadArg("l1", "R5").
		Raw("LSL $4, R3, R16"). // S: one input stream
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
	skConsts(b, r)
	b.Label("kloop").Raw("MOVD R19, %s", in[0])
	for j := 1; j < r; j++ {
		b.Raw("ADD R16, %s, %s", in[j-1], in[j])
	}
	b.Raw("MOVD R20, %s", out[0])
	for j := 1; j < r; j++ {
		b.Raw("ADD R17, %s, %s", out[j-1], out[j])
	}
	b.Raw("MOVD R2, R14")
	passStep(b, r, in, out, inverse, firstPair)
	b.Raw("LSR $1, R3, R15").
		Raw("SUBS $1, R15, R15").
		Raw("BEQ pairsdone")
	b.Label("iloop")
	passStep(b, r, in, out, inverse, nextPair)
	b.Raw("SUBS $1, R15, R15").
		Raw("BNE iloop")
	b.Label("pairsdone").
		Raw("TBZ $0, R3, knext") // odd ido: one point left
	passStep(b, r, in, out, inverse, lastPoint)
	b.Label("knext").
		Raw("%s", next).
		Raw("ADD R16, R20, R20").
		Raw("SUBS $1, R5, R5").
		Raw("BNE kloop").
		Ret()
	f.Add(b.Func())
}

// Last emits name(cc, ch *complex128,
// l1 int): the radix-r final pass (ido == 1), two blocks per iteration, and
// for an odd l1 one block alone at the end (compLastTail). Block k is the r
// points cc[r·k .. r·k+r-1]; two blocks are loaded as they lie (VLD1 into
// V(t), t = lastTemps[r], 2r registers; radix 8 in two halves, compLastLoad8),
// point j of blocks k and k+1 paired with VZIP1/VZIP2 into V(2j) (real parts)
// and V(2j+1) (imaginary parts), and output j of both blocks, ch[j·l1+k] and
// ch[j·l1+k+1], is one VST2. No twiddles.
func Last(f *emit.File, name string, r int, inverse bool) {
	check("Last", r, 2, 3, 4, 5, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	out := skStreams[r][1]
	t := map[int]int{2: 20, 3: 10, 4: 20, 5: 10, 8: 20}[r]
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("cc", "R0").LoadArg("ch", out[0]).LoadArg("l1", "R4").
		Raw("LSL $4, R4, R17")
	for j := 1; j < r; j++ {
		b.Raw("ADD R17, %s, %s", out[j-1], out[j])
	}
	b.Raw("LSR $1, R4, R5")
	skConsts(b, r)
	b.Raw("CBZ R5, tail") // l1 == 1: no pair of blocks
	b.Label("loop")
	if r == 8 {
		compLastLoad8(b)
	} else {
		for v := 0; v < 2*r; {
			switch {
			case 2*r-v >= 4:
				b.Raw("VLD1.P 64(R0), [V%d.D2, V%d.D2, V%d.D2, V%d.D2]", t+v, t+v+1, t+v+2, t+v+3)
				v += 4
			default:
				b.Raw("VLD1.P 32(R0), [V%d.D2, V%d.D2]", t+v, t+v+1)
				v += 2
			}
		}
		for j := 0; j < r; j++ {
			b.Raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", t+r+j, t+j, 2*j).
				Raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", t+r+j, t+j, 2*j+1)
		}
	}
	bfly(b, r, inverse, func(j, yr, yi int) {
		b.Raw("VST2.P [V%d.D2, V%d.D2], 32(%s)", yr, yi, out[j])
	})
	b.Raw("SUBS $1, R5, R5").
		Raw("BNE loop")
	compLastTail(b, r, inverse, out)
	b.Ret()
	f.Add(b.Func())
}

// compLastLoad8 loads and pairs two radix-8 blocks for Last. The
// 16 points of blocks k and k+1 would need 16 registers beside the 16 the
// butterfly reads, and V18 holds √2/2, so they come in halves: block k's
// points 0-3 into V20-V23 and 4-7 into V24-V27, block k+1's points 0-3 into
// V28-V31; points 0-3 of both are paired into V0-V7, which frees V20-V23 for
// block k+1's points 4-7, paired with V24-V27 into V8-V15.
func compLastLoad8(b *asm.Builder) {
	for _, t := range []int{20, 24, 28} {
		b.Raw("VLD1.P 64(R0), [V%d.D2, V%d.D2, V%d.D2, V%d.D2]", t, t+1, t+2, t+3)
	}
	for j := 0; j < 4; j++ {
		b.Raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", 28+j, 20+j, 2*j).
			Raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", 28+j, 20+j, 2*j+1)
	}
	b.Raw("VLD1.P 64(R0), [V20.D2, V21.D2, V22.D2, V23.D2]")
	for j := 4; j < 8; j++ {
		b.Raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", 20+j-4, 24+j-4, 2*j).
			Raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", 20+j-4, 24+j-4, 2*j+1)
	}
}

// compLastTail emits the end of Last's final pass for an odd l1:
// the last block alone. Its r points are loaded as they lie (VLD1, one
// complex128 per register) and each is paired with itself, so both lanes of
// V(2j), V(2j+1) hold point j and run the same arithmetic; lane 0 of every
// output is stored with FSTPD. The output streams are where the pair loop
// left them, one point past the last pair.
func compLastTail(b *asm.Builder, r int, inverse bool, out []string) {
	t := 20
	if r == 3 || r == 5 {
		t = 10 // V20-V27 hold radix 5's constants and radix 3's scratch
	}
	b.Label("tail").Raw("TBZ $0, R4, done")
	for v := 0; v < r; {
		switch {
		case r-v >= 4:
			b.Raw("VLD1.P 64(R0), [V%d.D2, V%d.D2, V%d.D2, V%d.D2]", t+v, t+v+1, t+v+2, t+v+3)
			v += 4
		case r-v >= 2:
			b.Raw("VLD1.P 32(R0), [V%d.D2, V%d.D2]", t+v, t+v+1)
			v += 2
		default:
			b.Raw("VLD1.P 16(R0), [V%d.D2]", t+v)
			v++
		}
	}
	for j := 0; j < r; j++ {
		b.Raw("VZIP1 V%d.D2, V%d.D2, V%d.D2", t+j, t+j, 2*j).
			Raw("VZIP2 V%d.D2, V%d.D2, V%d.D2", t+j, t+j, 2*j+1)
	}
	bfly(b, r, inverse, func(j, yr, yi int) {
		b.Raw("FSTPD (F%d, F%d), (%s)", yr, yi, out[j])
	})
	b.Label("done")
}

// Batch emits name(cc, ch, tw
// *complex128, ido, l1, pairs, odd, jin, jout, adjin, adjout int): one
// Stockham pass of radix r over a batch of w = 2·pairs + odd transforms laid
// side by side, the strips of a non-contiguous N-D axis (stockham.go's
// passBatch). Point p of the batch is w neighbouring complex128 values, so
// the pair of lines c, c+1 at one point is a VLD2 as in a 1-D pass, and every
// lane runs exactly the arithmetic the 1-D kernels (and the Go passes) run on
// one line: the same butterflies, the same fused twiddle product. A point's
// twiddle is the same for every line, so it is loaded once per pair already
// broadcast and split, re into V28 and im into V29, by VLD2R from the batched
// twiddle table (batchTwiddles: for i = 1 .. ido-1, the r-1 twiddles of point
// i). Point i = 0 is not multiplied at all, as in the Go pass, so no lane needs
// restoring. An odd batch ends with one line alone, loaded into lane 0 with
// FLDPD as the 1-D kernels do with the last point of an odd ido.
//
// Input stream j of block k starts at cc + k·r·jin + j·jin, output stream j at
// ch + k·ido·sOut·16 + j·jout; each row (one point i) advances its streams by
// 32·pairs through the post-indexed loads and stores and then by adjin and
// adjout, which the caller sets to 16·(stride - 2·pairs), to reach the next
// point's row. After its ido rows, stream 0 of the output has reached the next
// block's output, and stream 0 of the input is one stream (jin) past its block
// start, so it skips r-1 more.
//
// Registers: the streams as in the pass kernels (skStreams), R2 the table, R26
// the current point's twiddles and R14 their cursor, R3 the rows left, R5 the
// blocks left, R15 the pairs left, R16 jin, R17 jout, R19 and R20 scratch.
func Batch(f *emit.File, name string, r int, inverse bool) {
	check("Batch", r, 2, 3, 4, 5, 8)
	names := []string{"cc", "ch", "tw", "ido", "l1", "pairs", "odd", "jin", "jout", "adjin", "adjout"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr}
	for range names[3:] {
		types = append(types, asm.Int64)
	}
	sig := asm.Layout(names, types, nil, nil)
	in, out := skStreams[r][0], skStreams[r][1]
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("cc", in[0]).LoadArg("ch", out[0]).LoadArg("tw", "R2").
		LoadArg("l1", "R5").LoadArg("jin", "R16").LoadArg("jout", "R17")
	skConsts(b, r)
	// row emits one point's row of the batch: the pairs, the odd line, then
	// the step to the next point's row.
	row := func(label string, twiddled bool) {
		b.LoadArg("pairs", "R15").
			Raw("CBZ R15, %so", label)
		b.Label(label + "p")
		batchStepNEON(b, r, in, out, inverse, twiddled, false)
		b.Raw("SUBS $1, R15, R15").
			Raw("BNE %sp", label)
		b.Label(label+"o").
			LoadArg("odd", "R19").
			Raw("CBZ R19, %se", label)
		batchStepNEON(b, r, in, out, inverse, twiddled, true)
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
	b.Raw("ADD $%d, R26, R26", (r-1)*16).
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

// batchStepNEON emits one step of a batched pass: a pair of lines (or, with
// single, the last line alone) at one point: load, butterfly, and, when
// twiddled, outputs 1..r-1 multiplied by the point's twiddles with the 1-D
// kernels' fused product (see passStep), then store.
func batchStepNEON(b *asm.Builder, r int, in, out []string, inverse, twiddled, single bool) {
	for j := 0; j < r; j++ {
		if single {
			b.Raw("FLDPD (%s), (F%d, F%d)", in[j], 2*j, 2*j+1)
		} else {
			b.Raw("VLD2.P 32(%s), [V%d.D2, V%d.D2]", in[j], 2*j, 2*j+1)
		}
	}
	if twiddled {
		b.Raw("MOVD R26, R14")
	}
	store := func(v int, ptr string) {
		if single {
			b.Raw("FSTPD (F%d, F%d), (%s)", v, v+1, ptr)
		} else {
			b.Raw("VST2.P [V%d.D2, V%d.D2], 32(%s)", v, v+1, ptr)
		}
	}
	bfly(b, r, inverse, func(j, yr, yi int) {
		if j == 0 || !twiddled {
			store(yr, out[j])
			return
		}
		b.Raw("VLD2R.P 16(R14), [V28.D2, V29.D2]")
		b.VFMUL2D(30, yr, 28).VFMLS2D(30, yi, 29) // re = yr·wr - yi·wi, yi·wi fused
		if r == 2 {
			b.VFMUL2D(31, yr, 29).VFMLA2D(31, yi, 28) // im = yr·wi + yi·wr, yi·wr fused
		} else {
			b.VFMUL2D(31, yi, 28).VFMLA2D(31, yr, 29) // im = yi·wr + yr·wi, yr·wi fused
		}
		store(30, out[j])
	})
}

// The split-layout kernels keep a transform's data block-split between its
// passes, as KFR does (splitin/splitout), so the passes between the first and
// the last load and store with VLD1/VST1 instead of VLD2/VST2. In the
// block-split layout the pair of points 2q, 2q+1 occupies the same 32 bytes as
// when interleaved, ordered re(2q), re(2q+1), im(2q), im(2q+1): one VLD1 of two
// registers is then exactly what a VLD2 makes of the interleaved pair, with no
// deinterleave, and every address is unchanged. The first pass of a transform
// reads its input interleaved and writes split (out), the passes between read
// and write split (in, out), and the last reads split and writes interleaved
// (LastSplit), so the caller sees only interleaved data. The
// arithmetic is that of the interleaved kernels, instruction for instruction,
// so the results are the same bits.
//
// A pair must not straddle two streams, so every stream must start at an even
// point: the passes run with an even ido only, and the final pass with an
// even radix (2 or 4) and an even l1. That leaves the powers of two.

// splitPassStep is passStep for a pair of points (ido is even, so there is
// never a lone point) with the loads and stores of the layout.
func splitPassStep(b *asm.Builder, r int, in, out []string, inverse bool, first bool, inSplit bool) {
	for j := 0; j < r; j++ {
		if inSplit {
			b.Raw("VLD1.P 32(%s), [V%d.D2, V%d.D2]", in[j], 2*j, 2*j+1)
		} else {
			b.Raw("VLD2.P 32(%s), [V%d.D2, V%d.D2]", in[j], 2*j, 2*j+1)
		}
	}
	store := func(v int, ptr string) {
		b.Raw("VST1.P [V%d.D2, V%d.D2], 32(%s)", v, v+1, ptr)
	}
	bfly(b, r, inverse, func(j, yr, yi int) {
		if j == 0 {
			store(yr, out[0])
			return
		}
		b.Raw("VLD1.P 32(R14), [V28.D2, V29.D2]")
		b.VFMUL2D(30, yr, 28).VFMLS2D(30, yi, 29) // re = yr·wr - yi·wi, yi·wi fused
		if r == 2 {
			b.VFMUL2D(31, yr, 29).VFMLA2D(31, yi, 28) // im = yr·wi + yi·wr, yi·wr fused
		} else {
			b.VFMUL2D(31, yi, 28).VFMLA2D(31, yr, 29) // im = yi·wr + yr·wi, yr·wi fused
		}
		if first {
			b.Raw("VMOV V%d.D[0], V30.D[0]", yr).Raw("VMOV V%d.D[0], V31.D[0]", yi)
		}
		store(30, out[j])
	})
}

// PassSplit emits name(cc, ch, tw *complex128, ido, l1 int): Pass's
// radix-r pass, writing split (reading split too when inSplit), for an even
// ido, with the layout's loads and stores.
func PassSplit(f *emit.File, name string, r int, inverse, inSplit bool) {
	check("PassSplit", r, 2, 3, 4, 5, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	in, out := skStreams[r][0], skStreams[r][1]
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("cc", "R19").LoadArg("ch", "R20").LoadArg("tw", "R2").
		LoadArg("ido", "R3").LoadArg("l1", "R5").
		Raw("LSL $4, R3, R16"). // one input stream
		Raw("MUL R5, R16, R17") // one output stream
	next := map[int]string{2: "ADD R16<<1, R19, R19", 4: "ADD R16<<2, R19, R19", 8: "ADD R16<<3, R19, R19"}[r]
	if next == "" {
		next = compSplitBlockStep(b, r)
	}
	skConsts(b, r)
	b.Label("kloop").Raw("MOVD R19, %s", in[0])
	for j := 1; j < r; j++ {
		b.Raw("ADD R16, %s, %s", in[j-1], in[j])
	}
	b.Raw("MOVD R20, %s", out[0])
	for j := 1; j < r; j++ {
		b.Raw("ADD R17, %s, %s", out[j-1], out[j])
	}
	b.Raw("MOVD R2, R14")
	splitPassStep(b, r, in, out, inverse, true, inSplit)
	b.Raw("LSR $1, R3, R15").
		Raw("SUBS $1, R15, R15").
		Raw("BEQ knext")
	b.Label("iloop")
	splitPassStep(b, r, in, out, inverse, false, inSplit)
	b.Raw("SUBS $1, R15, R15").
		Raw("BNE iloop")
	b.Label("knext").
		Raw("%s", next).
		Raw("ADD R16, R20, R20").
		Raw("SUBS $1, R5, R5").
		Raw("BNE kloop").
		Ret()
	f.Add(b.Func())
}

// LastSplit emits name(cc, ch *complex128, l1 int): Last's final pass
// (r = 2, 4 or 8), reading block-split
// input. Blocks k and k+1 are the 2r points from r·k, r pairs loaded as they
// lie into V(t+2q) (real parts of pair q) and V(t+2q+1) (imaginary parts).
// Point j of block k is lane j%2 of pair j/2, and of block k+1 the same lane of
// pair r/2+j/2 (r is even), so VZIP1 (j even) or VZIP2 (j odd) of the two
// pairs puts point j of both blocks in V(2j) and V(2j+1), as the interleaved
// kernel's zips do.
func LastSplit(f *emit.File, name string, r int, inverse bool) {
	check("LastSplit", r, 2, 4, 8)
	sig := asm.Layout(
		[]string{"cc", "ch", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	out := skStreams[r][1]
	t := 20
	b := asm.NewFunc(name, sig, 0)
	b.LoadArg("cc", "R0").LoadArg("ch", out[0]).LoadArg("l1", "R4").
		Raw("LSL $4, R4, R17")
	for j := 1; j < r; j++ {
		b.Raw("ADD R17, %s, %s", out[j-1], out[j])
	}
	b.Raw("LSR $1, R4, R5")
	skConsts(b, r)
	b.Raw("CBZ R5, tail") // l1 == 1: no pair of blocks
	b.Label("loop")
	if r == 8 {
		compSplitLoad8(b)
	} else {
		for v := 0; v < 2*r; v += 4 {
			b.Raw("VLD1.P 64(R0), [V%d.D2, V%d.D2, V%d.D2, V%d.D2]", t+v, t+v+1, t+v+2, t+v+3)
		}
		for j := 0; j < r; j++ {
			qa, qb := j/2, r/2+j/2
			z := "VZIP1"
			if j%2 == 1 {
				z = "VZIP2"
			}
			b.Raw("%s V%d.D2, V%d.D2, V%d.D2", z, t+2*qb, t+2*qa, 2*j).
				Raw("%s V%d.D2, V%d.D2, V%d.D2", z, t+2*qb+1, t+2*qa+1, 2*j+1)
		}
	}
	bfly(b, r, inverse, func(j, yr, yi int) {
		b.Raw("VST2.P [V%d.D2, V%d.D2], 32(%s)", yr, yi, out[j])
	})
	b.Raw("SUBS $1, R5, R5").
		Raw("BNE loop")
	compSplitTail(b, r, inverse, out)
	b.Ret()
	f.Add(b.Func())
}

// compSplitBlockStep emits, for PassSplit's radix 3 and 5, R4 =
// r·ido·16 (one input block) and returns the advance to the next block, as
// Pass does for the radices that are not powers of two.
func compSplitBlockStep(b *asm.Builder, r int) string {
	b.Raw("MOVD $%d, R4", r).Raw("MUL R4, R16, R4")
	return "ADD R4, R19, R19"
}

// compSplitLoad8 loads and pairs two block-split radix-8 blocks for
// LastSplit, in halves as compLastLoad8 does: block k's pairs 0-3
// into V20-V27 and block k+1's pairs 0 and 1 into V28-V31, points 0-3 of both
// paired into V0-V7; then block k+1's pairs 2 and 3 into the freed V20-V23,
// points 4-7 paired into V8-V15. Point j is lane j%2 of pair j/2.
func compSplitLoad8(b *asm.Builder) {
	for _, t := range []int{20, 24, 28} {
		b.Raw("VLD1.P 64(R0), [V%d.D2, V%d.D2, V%d.D2, V%d.D2]", t, t+1, t+2, t+3)
	}
	zip := func(j, a, c int) { // a: block k's pair, c: block k+1's pair
		z := "VZIP1"
		if j%2 == 1 {
			z = "VZIP2"
		}
		b.Raw("%s V%d.D2, V%d.D2, V%d.D2", z, c, a, 2*j).
			Raw("%s V%d.D2, V%d.D2, V%d.D2", z, c+1, a+1, 2*j+1)
	}
	for j := 0; j < 4; j++ {
		zip(j, 20+2*(j/2), 28+2*(j/2))
	}
	b.Raw("VLD1.P 64(R0), [V20.D2, V21.D2, V22.D2, V23.D2]")
	for j := 4; j < 8; j++ {
		zip(j, 24+2*((j-4)/2), 20+2*((j-4)/2))
	}
}

// compSplitTail emits the end of LastSplit's final pass for an
// odd l1: the last block alone, its r/2 pairs loaded as they lie into V20 on,
// point j (lane j%2 of pair j/2) paired with itself into both lanes of V(2j),
// V(2j+1), and lane 0 of every output stored with FSTPD, as compLastTail does.
func compSplitTail(b *asm.Builder, r int, inverse bool, out []string) {
	t := 20
	b.Label("tail").Raw("TBZ $0, R4, done")
	for v := 0; v < r; {
		if r-v >= 4 {
			b.Raw("VLD1.P 64(R0), [V%d.D2, V%d.D2, V%d.D2, V%d.D2]", t+v, t+v+1, t+v+2, t+v+3)
			v += 4
		} else {
			b.Raw("VLD1.P 32(R0), [V%d.D2, V%d.D2]", t+v, t+v+1)
			v += 2
		}
	}
	for j := 0; j < r; j++ {
		q := t + 2*(j/2)
		z := "VZIP1"
		if j%2 == 1 {
			z = "VZIP2"
		}
		b.Raw("%s V%d.D2, V%d.D2, V%d.D2", z, q, q, 2*j).
			Raw("%s V%d.D2, V%d.D2, V%d.D2", z, q+1, q+1, 2*j+1)
	}
	bfly(b, r, inverse, func(j, yr, yi int) {
		b.Raw("FSTPD (F%d, F%d), (%s)", yr, yi, out[j])
	})
	b.Label("done")
}
