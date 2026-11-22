package amd64

import (
	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// EVEX-256 twins of the radix-10/15/20 final-pass kernels (go-fft/fft#162).
//
// lastPFA parks the 5·q stage-1 values in a 32·5·q-byte frame (a store and a
// reload each, and a store-to-load forward between the two stages). With the
// 32 ymm registers of AVX-512VL (Y16..Y31 are only encodable in EVEX, at
// 256 bits: the width, hence the pipe rate, of the VEX kernels) they stay in
// registers: no frame, and every arithmetic operation is the VEX kernel's, in
// its order, so the twin is bit-identical to it.
//
// Allocation: stage 1 runs on Y0..Y12 as in lastPFA. The values of groups
// n2 < q-1 are copied (VMOVAPD, a register rename) to Y16 + 5·n2 + k1; those
// of the last group stay where bfly5 left them (Y7, Y1, Y2, Y3, Y4). Stage 2
// reads them in place and uses Y0, Y5, Y6, Y8..Y12 as scratch, which none of
// them occupies. Every instruction on a register of Y16..Y31 is an
// AVX512F/VL one (VMOVAPD, VADDPD, VSUBPD); the shuffles and the sign flip
// (VPERMILPD, VXORPD) only ever touch Y0..Y15, so AVX512DQ is not needed.

// evexHeld is the register that holds stage-1 value (n2, k1) after stage 1.
func evexHeld(n2, k1, q int) int {
	if n2 == q-1 {
		return [5]int{7, 1, 2, 3, 4}[k1]
	}
	return 16 + 5*n2 + k1
}

// comp2BodyEVEX is comp2Body's final pass with the stage-1 values in registers.
func (e skEmit) comp2BodyEVEX(m comp2Map) {
	q := m.q
	for n2 := range q {
		y := e.comp2Bfly5(m.in[n2])
		if n2 == q-1 {
			break
		}
		for k1, r := range y {
			e.raw("VMOVAPD %s, %s", e.v(r), e.v(evexHeld(n2, k1, q)))
		}
	}
	for k1 := range 5 {
		var x [4]int
		for n2 := range q {
			x[n2] = evexHeld(n2, k1, q)
		}
		var z []int
		switch q {
		case 2:
			e.add(0, x[0], x[1])
			e.sub(5, x[0], x[1])
			z = []int{0, 5}
		case 3:
			e.comp12Bfly3([3]int{x[0], x[1], x[2]}, [3]int{0, 5, 6}, [3]int{8, 9, 10})
			z = []int{0, 5, 6}
		default:
			e.bfly4r(x, [4]int{0, 5, 6, 8}, [4]int{9, 10, 11, 12})
			z = []int{9, 10, 11, 12}
		}
		for k2, r := range z {
			e.st(r, e.outAddr(m.out[k1][k2]))
		}
	}
}

// LastEVEX emits name(cc, ch *complex128, k *float64, l1 int), the EVEX-256
// twin of Last for r = 10, 15 or 20: the same final pass, bit for bit, with
// no stack frame. It needs AVX512F and AVX512VL (gate it on both).
func (c *Kernels) LastEVEX(f *emit.File, name string, r int) {
	check("LastEVEX", r, 10, 15, 20)
	q := r / 5
	m := comp2NewMap(q)
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, 0)
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
	y.comp2BodyEVEX(m)
	b.Raw("ADDQ $%d, AX", 32*r)
	for _, o := range comp2OutBase[:q] {
		b.Raw("ADDQ $32, %s", o)
	}
	b.Raw("DECQ R9").
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ $16, DX"). // l1 odd
		Raw("JZ done")
	x.comp2BodyEVEX(m)
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
