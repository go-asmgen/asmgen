package loong64

import "fmt"

// LSX/LASX float64 instructions the Go assembler does not have.
//
// cmd/asm has the vector float64 add/sub/mul/div on loong64 (VADDD/XVADDD,
// VMULD/XVMULD, ... in Go's F/D-suffixed naming), the vector loads and stores
// (VMOVQ/XVMOVQ) and lane moves, but no fused multiply-add and no broadcast
// load. A float64 kernel then pays a separate multiply and add (two roundings)
// and two instructions per broadcast. These methods encode them: each emits
// one WORD with the instruction's LoongArch encoding and its assembly in a
// comment. Vector registers are 0–31 (V/X share the numbering; F0–F31 are the
// low 64 bits). The encodings are checked against GNU as 2.43 on a Loongson
// 3C5000L (TestLASXEncodings).

// XVFMADDD emits xvfmadd.d: X[d] = X[j] × X[k] + X[a], four float64 lanes,
// fused (one rounding).
func (b *Builder) XVFMADDD(d, j, k, a int) *Builder {
	return b.r4("xvfmadd.d", "xr", 0x0a200000, d, j, k, a)
}

// VFMADDD emits vfmadd.d: V[d] = V[j] × V[k] + V[a], two float64 lanes, fused.
func (b *Builder) VFMADDD(d, j, k, a int) *Builder {
	return b.r4("vfmadd.d", "vr", 0x09200000, d, j, k, a)
}

// XVLDREPLD emits xvldrepl.d: the float64 at off(R[rj]) in all four lanes of
// X[d]. off is a multiple of 8 in [-2048, 2040].
func (b *Builder) XVLDREPLD(d, rj, off int) *Builder {
	return b.ldrepl("xvldrepl.d", "xr", 0x32100000, d, rj, off)
}

// VLDREPLD emits vldrepl.d: the float64 at off(R[rj]) in both lanes of V[d].
func (b *Builder) VLDREPLD(d, rj, off int) *Builder {
	return b.ldrepl("vldrepl.d", "vr", 0x30100000, d, rj, off)
}

func (b *Builder) r4(mnemonic, pfx string, base uint32, d, j, k, a int) *Builder {
	for _, r := range []int{d, j, k, a} {
		vecreg(r)
	}
	w := base | uint32(a)<<15 | uint32(k)<<10 | uint32(j)<<5 | uint32(d)
	return b.Raw("WORD $0x%08x // %s $%s%d, $%s%d, $%s%d, $%s%d", w, mnemonic, pfx, d, pfx, j, pfx, k, pfx, a)
}

func (b *Builder) ldrepl(mnemonic, pfx string, base uint32, d, rj, off int) *Builder {
	vecreg(d)
	if rj < 0 || rj > 31 {
		panic(fmt.Sprintf("loong64: general register R%d does not exist", rj))
	}
	if off%8 != 0 || off < -2048 || off > 2040 {
		panic(fmt.Sprintf("loong64: %s offset %d is not a multiple of 8 in [-2048, 2040]", mnemonic, off))
	}
	w := base | uint32(off/8)&0x1ff<<10 | uint32(rj)<<5 | uint32(d)
	return b.Raw("WORD $0x%08x // %s $%s%d, $r%d, %d", w, mnemonic, pfx, d, rj, off)
}

// vecreg refuses a vector register number outside 0–31: it would spill into
// the neighbouring field of the encoding and name a different instruction.
func vecreg(r int) {
	if r < 0 || r > 31 {
		panic(fmt.Sprintf("loong64: vector register %d does not exist", r))
	}
}
