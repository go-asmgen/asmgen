package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Radix-16 Stockham passes (the fft package's radix16.go, Round 19).
//
// The 16-point butterfly is the 4×4 split of bfly16, the algorithm of FFTW's
// n1fv_16/t1fv_16 codelets: four radix-4 butterflies over the inputs n2,
// n2+4, n2+8, n2+12 (stage 1), the internal twiddles W16^(n2·k1), then four
// radix-4 butterflies over the stage-1 outputs k1 of each (stage 2), whose
// output k2 is the pass's output k1+4·k2. Every operation is the Go
// reference's, in the same order, separately rounded.
//
// Sixteen YMM registers cannot hold the sixteen stage-1 values and the
// working set, so the values of k1 = 2 and 3 (eight registers) are spilled
// to the frame, 32 bytes each, and read back for stage 2; k1 = 0 and 1 stay
// in Y8..Y15.
//
// The twiddles come pre-duplicated (radix16Twiddles): each product takes
// (wr, wr) and (wi, wi) as memory operands, and swapping y is its only
// shuffle. On Zen 3 that made the twiddled pass 7% faster (2026-10-05).
//
// The pass kernel addresses sixteen input and sixteen output streams through
// four base registers each (skIn, skOut): AX, SI, R8, R9 in and BX, DI, R11,
// R15 out. That leaves no register for the block and group counters, which
// live in the frame (radix16Blocks, radix16Groups), nor for the twiddle base,
// which is reloaded from the arguments at each block.
//
// There is no AVX-512 radix-16 kernel: none could be verified on hardware
// this round, and the fft package does not factor with radix 16 where the
// AVX-512 kernels run.

const (
	radix16Spill  = 256 // eight spilled stage-1 values, 32 bytes each
	radix16Blocks = 256 // blocks left (pass kernel)
	radix16Groups = 264 // four-point groups left (pass kernel)
	radix16Frame  = 272
)

// r16keep is the register that holds stage-1 value (n2, k1), k1 = 0 or 1,
// between the stages.
func r16keep(n2, k1 int) int { return 8 + 4*k1 + n2 }

// r16slot is the frame offset of a spilled stage-1 value (k1 = 2, 3).
func r16slot(n2, k1 int) string { return fmt.Sprintf("%d(SP)", 32*(4*(k1-2)+n2)) }

// rot16 emits dst = src·W16^e for e in {1, 2, 3, 6, 9}: c·src + swap(src)·[−ss,
// ss], the four operations of radix16Rot. ta and tb are scratch.
func (e skEmit) rot16(dst, src, ex, ta, tb int) {
	k := map[int][2]int{
		1: {e.c.K.R16C1, e.c.K.R16SS1},
		2: {e.c.K.H, e.c.K.R16SSH},
		3: {e.c.K.R16S1, e.c.K.R16SSC1},
		6: {e.c.K.R16NH, e.c.K.R16SSH},
		9: {e.c.K.R16NC1, e.c.K.R16SSN1},
	}[ex]
	e.rotC(dst, src, k[0], k[1], ta, tb)
}

// rotC emits dst = c·src + swap(src)·[−ss, ss] with c and the ss row at
// constants offsets kc and ks: radix16Rot's four operations, or, in an FMA
// kernel (e.c.FMA), the swap, one multiply and one fused multiply-add,
// fma(c, src, swap(src)·[−ss, ss]) (radix16RotFMA).
func (e skEmit) rotC(dst, src, kc, ks, ta, tb int) {
	if e.c.FMA {
		e.perm("swap", src, tb)
		e.mulk(tb, tb, ks)
		e.raw("VFMADD231PD %d(R14), %s, %s", e.k(kc), e.v(src), e.v(tb))
		e.raw("VMOVAPD %s, %s", e.v(tb), e.v(dst))
		return
	}
	e.mulk(ta, src, kc)
	e.perm("swap", src, tb)
	e.mulk(tb, tb, ks)
	e.add(dst, ta, tb)
}

// bfly4r emits the radix-4 butterfly of registers in into z, through t: the
// Go bfly4's t2, t1, t3, t4 = rotS(x1 − x3), then y0..y3.
func (e skEmit) bfly4r(in, t, z [4]int) {
	e.add(t[0], in[0], in[2])
	e.sub(t[1], in[0], in[2])
	e.add(t[2], in[1], in[3])
	e.sub(t[3], in[1], in[3])
	e.rot(t[3], t[3])
	e.add(z[0], t[0], t[2])
	e.add(z[1], t[1], t[3])
	e.sub(z[2], t[0], t[2])
	e.sub(z[3], t[1], t[3])
}

// body16 emits one radix-16 butterfly over the points the register width
// holds, with the outer twiddles (twStore) unless this is a final pass.
func (e skEmit) body16(first bool) {
	// Stage 1.
	for n2 := 0; n2 < 4; n2++ {
		for q := 0; q < 4; q++ {
			e.in(q, n2+4*q)
		}
		// y0 straight into its register; y1, y2, y3 into Y0..Y2.
		e.bfly4r([4]int{0, 1, 2, 3}, [4]int{4, 5, 6, 7}, [4]int{r16keep(n2, 0), 0, 1, 2})
		for k1 := 1; k1 < 4; k1++ {
			src, dst := k1-1, 5 // y_k1, and where its twiddled value goes
			if k1 == 1 {
				dst = r16keep(n2, 1)
			}
			switch ex := n2 * k1; ex {
			case 0:
				if k1 == 1 {
					e.raw("VMOVAPD %s, %s", e.v(src), e.v(dst))
				}
				dst = src
			case 4:
				e.rot(dst, src)
			default:
				e.rot16(dst, src, ex, 3, 4)
			}
			if k1 >= 2 {
				e.raw("VMOVUPD %s, %s", e.v(dst), r16slot(n2, k1))
			}
		}
	}
	// Stage 2.
	for k1 := 0; k1 < 4; k1++ {
		var in [4]int
		t, z, tt := [4]int{0, 1, 2, 3}, [4]int{4, 5, 6, 7}, [4]int{8, 9, 10, 11}
		if k1 >= 2 {
			for n2 := 0; n2 < 4; n2++ {
				in[n2] = n2
				e.raw("VMOVUPD %s, %s", r16slot(n2, k1), e.v(n2))
			}
			t, z, tt = [4]int{4, 5, 6, 7}, [4]int{8, 9, 10, 11}, [4]int{12, 13, 14, 15}
		} else {
			for n2 := 0; n2 < 4; n2++ {
				in[n2] = r16keep(n2, k1)
			}
		}
		e.bfly4r(in, t, z)
		for k2 := 0; k2 < 4; k2++ {
			if j := k1 + 4*k2; j == 0 {
				e.st(z[0], skOut(0))
			} else {
				e.twStore(z[k2], j, tt[0], tt[1], tt[2], tt[3], first)
			}
		}
	}
}

// advance16 moves the eight stream bases of the radix-16 pass kernel.
func (e skEmit) advance16(bytes int) {
	for _, r := range []string{"AX", "SI", "R8", "R9", "BX", "DI", "R11", "R15"} {
		e.raw("ADDQ $%d, %s", bytes, r)
	}
}

// pass16 emits Pass's radix-16 pass, the loop of pass around body16.
func (c *Kernels) pass16(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"cc", "ch", "tw", "k", "ido", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, radix16Frame)
	q0 := skEmit{c: c, b: b, w: "Y", g: 4, dup: true}
	q1 := skEmit{c: c, b: b, w: "Y", g: 4, half: 1, dup: true}
	p := skEmit{c: c, b: b, w: "Y", g: 2, dup: true}
	x := skEmit{c: c, b: b, w: "X", g: 1, dup: true}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").
		LoadArg("ido", "CX").LoadArg("l1", "R8")
	b.Raw("MOVQ CX, DX").
		Raw("IMULQ R8, DX").
		Raw("SHLQ $4, CX"). // S
		Raw("SHLQ $4, DX"). // OS
		Raw("LEAQ (CX)(CX*2), R12").
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("MOVQ R8, %d(SP)", radix16Blocks).
		Raw("kloop:").
		Raw("CMPQ %d(SP), $0", radix16Blocks).
		Raw("JEQ done").
		Raw("MOVQ CX, R9").
		Raw("SHRQ $6, R9"). // four-point groups = ido/4
		Raw("MOVQ R9, %d(SP)", radix16Groups).
		Raw("LEAQ (AX)(CX*4), SI").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("LEAQ (AX)(CX*8), R8").
		Raw("LEAQ (SI)(CX*8), R9").
		Raw("LEAQ (BX)(DX*8), R11").
		Raw("LEAQ (DI)(DX*8), R15")
	b.LoadArg("tw", "R10")
	b.Raw("CMPQ %d(SP), $0", radix16Groups).
		Raw("JEQ firstpair")
	q0.body16(true)
	q0.advance16(32)
	q1.body16(false)
	q1.advance16(32)
	q0.twNext(16)
	b.Raw("DECQ %d(SP)", radix16Groups).
		Raw("qloop:").
		Raw("CMPQ %d(SP), $0", radix16Groups).
		Raw("JEQ pair")
	q0.body16(false)
	q0.advance16(32)
	q1.body16(false)
	q1.advance16(32)
	q0.twNext(16)
	b.Raw("DECQ %d(SP)", radix16Groups).
		Raw("JMP qloop").
		Raw("pair:").
		Raw("TESTQ $32, CX"). // ido & 2
		Raw("JZ single")
	p.body16(false)
	p.advance16(32)
	p.twNext(16)
	b.Raw("JMP single").
		Raw("firstpair:") // ido is 2 or 3: the first pair is the two-point group
	p.body16(true)
	p.advance16(32)
	p.twNext(16)
	b.Raw("single:").
		Raw("TESTQ $16, CX"). // ido & 1
		Raw("JZ knext")
	x.body16(false)
	x.advance16(16)
	// P advanced by S over the block; the next block starts 16·S after this
	// one. O advanced by S, which is the next block's output start.
	b.Raw("knext:").
		Raw("LEAQ (AX)(CX*8), AX").
		Raw("LEAQ (AX)(CX*4), AX").
		Raw("ADDQ R12, AX").
		Raw("DECQ %d(SP)", radix16Blocks).
		Raw("JMP kloop").
		Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}

// last16 emits Last's radix-16 final pass (ido == 1), two blocks per
// register as in last, an odd l1 finished by one 128-bit step.
func (c *Kernels) last16(f *emit.File, name string) {
	sig := asm.Layout(
		[]string{"cc", "ch", "k", "l1"},
		[]asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64}, nil, nil,
	)
	b := asm.NewFunc(name, sig, radix16Spill)
	y := skEmit{c: c, b: b, w: "Y", last: 16}
	x := skEmit{c: c, b: b, w: "X", last: 16}
	b.LoadArg("cc", "AX").LoadArg("ch", "BX").LoadArg("k", "R14").LoadArg("l1", "DX")
	b.Raw("MOVQ DX, R9").
		Raw("SHRQ $1, R9"). // block pairs
		Raw("SHLQ $4, DX"). // OS
		Raw("LEAQ (DX)(DX*2), R13").
		Raw("LEAQ (BX)(DX*4), DI").
		Raw("LEAQ (BX)(DX*8), R11").
		Raw("LEAQ (DI)(DX*8), R15").
		Raw("loop:").
		Raw("TESTQ R9, R9").
		Raw("JZ tail")
	y.body16(false)
	b.Raw("ADDQ $%d, AX", 32*16)
	for _, r := range []string{"BX", "DI", "R11", "R15"} {
		b.Raw("ADDQ $32, %s", r)
	}
	b.Raw("DECQ R9").
		Raw("JMP loop").
		Raw("tail:").
		Raw("TESTQ $16, DX"). // l1 odd
		Raw("JZ done")
	x.body16(false)
	b.Raw("done:").
		Raw("VZEROUPPER").
		Ret()
	f.Add(b.Func())
}
