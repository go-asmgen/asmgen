package arm64

import "fmt"

// Vector float64 arithmetic, .2D arrangement (two float64 lanes, the one a
// complex128 or a pair of float64 needs).
//
// These emit cmd/asm mnemonics and leave the encoding to the Go assembler, as
// every other method here does. VFMLA/VFMLS have long been there; VFADD,
// VFSUB, VFMUL and VFNEG arrived in Go 1.27 (CL 762200), which is why this
// module requires it. Before that they were hand-encoded as WORDs, bypassing
// cmd/asm's checks; the encodings cmd/asm now produces were compared with the
// system assembler's (TestVectorFloatEncodings) and are identical.
//
// Operands follow Plan 9 order, sources first: the method takes (d, n, m) like
// the A64 form "fadd vd.2d, vn.2d, vm.2d" and emits "VFADD Vm.D2, Vn.D2, Vd.D2".

// VFADD2D emits Vd.2D = Vn.2D + Vm.2D.
func (b *Builder) VFADD2D(d, n, m int) *Builder { return b.vfp3("VFADD", d, n, m) }

// VFSUB2D emits Vd.2D = Vn.2D - Vm.2D.
func (b *Builder) VFSUB2D(d, n, m int) *Builder { return b.vfp3("VFSUB", d, n, m) }

// VFMUL2D emits Vd.2D = Vn.2D × Vm.2D (each product rounded once).
func (b *Builder) VFMUL2D(d, n, m int) *Builder { return b.vfp3("VFMUL", d, n, m) }

// VFMLA2D emits Vd.2D += Vn.2D × Vm.2D, fused (one rounding).
func (b *Builder) VFMLA2D(d, n, m int) *Builder { return b.vfp3("VFMLA", d, n, m) }

// VFMLS2D emits Vd.2D -= Vn.2D × Vm.2D, fused (one rounding).
func (b *Builder) VFMLS2D(d, n, m int) *Builder { return b.vfp3("VFMLS", d, n, m) }

// VFNEG2D emits Vd.2D = -Vn.2D (a sign flip, exact).
func (b *Builder) VFNEG2D(d, n int) *Builder {
	vreg(d)
	vreg(n)
	return b.Raw("VFNEG V%d.D2, V%d.D2", n, d)
}

func (b *Builder) vfp3(mnemonic string, d, n, m int) *Builder {
	vreg(d)
	vreg(n)
	vreg(m)
	return b.Raw("%s V%d.D2, V%d.D2, V%d.D2", mnemonic, m, n, d)
}

// vreg refuses a register number outside V0–V31 at generation time, where the
// caller's stack still points at the mistake, rather than at assembly time.
func vreg(r int) {
	if r < 0 || r > 31 {
		panic(fmt.Sprintf("arm64: vector register V%d does not exist", r))
	}
}
