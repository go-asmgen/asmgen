package arm64

import "fmt"

// Vector float arithmetic: .2D (two float64 lanes, the one a complex128 or a
// pair of float64 needs) and .4S (four float32 lanes).
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
func (b *Builder) VFNEG2D(d, n int) *Builder { return b.vfp2("VFNEG", "D2", d, n) }

// Vector float32 arithmetic, .4S arrangement (four float32 lanes: the real or
// the imaginary parts of four complex64 split by VLD2, or two complex64 as
// they lie). The same instructions as the .2D forms with the size bit clear;
// cmd/asm encodes both (TestVectorFloatEncodings checks them against the
// system assembler).

// VFADD4S emits Vd.4S = Vn.4S + Vm.4S.
func (b *Builder) VFADD4S(d, n, m int) *Builder { return b.vfp3s("VFADD", "S4", d, n, m) }

// VFSUB4S emits Vd.4S = Vn.4S - Vm.4S.
func (b *Builder) VFSUB4S(d, n, m int) *Builder { return b.vfp3s("VFSUB", "S4", d, n, m) }

// VFMUL4S emits Vd.4S = Vn.4S × Vm.4S (each product rounded once).
func (b *Builder) VFMUL4S(d, n, m int) *Builder { return b.vfp3s("VFMUL", "S4", d, n, m) }

// VFMLA4S emits Vd.4S += Vn.4S × Vm.4S, fused (one rounding).
func (b *Builder) VFMLA4S(d, n, m int) *Builder { return b.vfp3s("VFMLA", "S4", d, n, m) }

// VFMLS4S emits Vd.4S -= Vn.4S × Vm.4S, fused (one rounding).
func (b *Builder) VFMLS4S(d, n, m int) *Builder { return b.vfp3s("VFMLS", "S4", d, n, m) }

// VFNEG4S emits Vd.4S = -Vn.4S (a sign flip, exact).
func (b *Builder) VFNEG4S(d, n int) *Builder { return b.vfp2("VFNEG", "S4", d, n) }

func (b *Builder) vfp3(mnemonic string, d, n, m int) *Builder {
	return b.vfp3s(mnemonic, "D2", d, n, m)
}

// vfp3s emits a three-register vector float instruction with arrangement arr
// (cmd/asm spells .2D as D2 and .4S as S4).
func (b *Builder) vfp3s(mnemonic, arr string, d, n, m int) *Builder {
	vreg(d)
	vreg(n)
	vreg(m)
	return b.Raw("%s V%d.%s, V%d.%s, V%d.%s", mnemonic, m, arr, n, arr, d, arr)
}

// vfp2 emits a two-register vector float instruction with arrangement arr.
func (b *Builder) vfp2(mnemonic, arr string, d, n int) *Builder {
	vreg(d)
	vreg(n)
	return b.Raw("%s V%d.%s, V%d.%s", mnemonic, n, arr, d, arr)
}

// vreg refuses a register number outside V0–V31 at generation time, where the
// caller's stack still points at the mistake, rather than at assembly time.
func vreg(r int) {
	if r < 0 || r > 31 {
		panic(fmt.Sprintf("arm64: vector register V%d does not exist", r))
	}
}
