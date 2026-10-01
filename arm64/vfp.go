package arm64

import "fmt"

// Vector float64 arithmetic the Go assembler does not have.
//
// cmd/asm accepts the fused VFMLA/VFMLS on arm64 but has no vector FADD, FSUB,
// FMUL or FNEG for floating-point lanes, so a NEON float64 kernel either
// emulates them (an add as a multiply-add by 1.0, at twice the latency — what
// go-fft/fft's first NEON butterflies did, and why they only tied the scalar
// loop) or encodes them by hand. These methods encode them: each emits one
// WORD with the instruction's A64 encoding and its assembly in a comment, for
// the .2D arrangement (two float64 lanes), the one a complex128 or a pair of
// float64 needs. The encodings are checked against the system assembler
// (TestVectorFloatEncodings); FMLA/FMLS are included so a kernel can use one
// register-number convention throughout.
//
// Arm ARM, "Advanced SIMD three same" (FADD/FSUB/FMUL/FMLA/FMLS, size=1 Q=1)
// and "two-register miscellaneous" (FNEG).

// VFADD2D emits Vd.2D = Vn.2D + Vm.2D.
func (b *Builder) VFADD2D(d, n, m int) *Builder { return b.vfp3("fadd", 0x4E60D400, d, n, m) }

// VFSUB2D emits Vd.2D = Vn.2D - Vm.2D.
func (b *Builder) VFSUB2D(d, n, m int) *Builder { return b.vfp3("fsub", 0x4EE0D400, d, n, m) }

// VFMUL2D emits Vd.2D = Vn.2D × Vm.2D (each product rounded once).
func (b *Builder) VFMUL2D(d, n, m int) *Builder { return b.vfp3("fmul", 0x6E60DC00, d, n, m) }

// VFMLA2D emits Vd.2D += Vn.2D × Vm.2D, fused (one rounding).
func (b *Builder) VFMLA2D(d, n, m int) *Builder { return b.vfp3("fmla", 0x4E60CC00, d, n, m) }

// VFMLS2D emits Vd.2D -= Vn.2D × Vm.2D, fused (one rounding).
func (b *Builder) VFMLS2D(d, n, m int) *Builder { return b.vfp3("fmls", 0x4EE0CC00, d, n, m) }

// VFNEG2D emits Vd.2D = -Vn.2D (a sign flip, exact).
func (b *Builder) VFNEG2D(d, n int) *Builder {
	vreg(d)
	vreg(n)
	return b.Raw("WORD $0x%08x // fneg v%d.2d, v%d.2d", 0x6EE0F800|uint32(n)<<5|uint32(d), d, n)
}

func (b *Builder) vfp3(mnemonic string, base uint32, d, n, m int) *Builder {
	vreg(d)
	vreg(n)
	vreg(m)
	return b.Raw("WORD $0x%08x // %s v%d.2d, v%d.2d, v%d.2d",
		base|uint32(m)<<16|uint32(n)<<5|uint32(d), mnemonic, d, n, m)
}

// vreg refuses a register number outside V0–V31: it would spill into the
// neighbouring field of the encoding and name a different instruction.
func vreg(r int) {
	if r < 0 || r > 31 {
		panic(fmt.Sprintf("arm64: vector register V%d does not exist", r))
	}
}
