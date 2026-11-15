package arm64

import "github.com/go-asmgen/asmgen/internal/gap"

// SSHL, signed shift left by register: each lane of Vn is shifted by the
// signed low byte of the same lane of Vm; a negative count is an arithmetic
// shift right, rounding toward minus infinity. It is the variable shift a
// fixed-point kernel needs when the shift is a runtime parameter (H.264
// weighted prediction's logWD), where VSSHR and VSRSHR, which take an
// immediate, do not apply.
//
// TRANSITIONAL, like the by-element forms in elem.go: Go's assembler (1.27.4)
// has no register-form SSHL, so these methods emit a WORD commented with the
// proposed spelling (VSSHL Vm.T, Vn.T, Vd.T). Operands are in A64 order:
// (d, n, m), "sshl vd.t, vn.t, vm.t". A register outside V0-V31 panics at
// generation time.

// VSSHL8B emits Vd.8B = Vn.8B shifted by Vm.8B.
func (b *Builder) VSSHL8B(d, n, m int) *Builder { return b.sshl("sshl.8b", d, n, m) }

// VSSHL16B emits Vd.16B = Vn.16B shifted by Vm.16B.
func (b *Builder) VSSHL16B(d, n, m int) *Builder { return b.sshl("sshl.16b", d, n, m) }

// VSSHL4H emits Vd.4H = Vn.4H shifted by Vm.4H.
func (b *Builder) VSSHL4H(d, n, m int) *Builder { return b.sshl("sshl.4h", d, n, m) }

// VSSHL8H emits Vd.8H = Vn.8H shifted by Vm.8H.
func (b *Builder) VSSHL8H(d, n, m int) *Builder { return b.sshl("sshl.8h", d, n, m) }

// VSSHL2S emits Vd.2S = Vn.2S shifted by Vm.2S.
func (b *Builder) VSSHL2S(d, n, m int) *Builder { return b.sshl("sshl.2s", d, n, m) }

// VSSHL4S emits Vd.4S = Vn.4S shifted by Vm.4S.
func (b *Builder) VSSHL4S(d, n, m int) *Builder { return b.sshl("sshl.4s", d, n, m) }

// VSSHL2D emits Vd.2D = Vn.2D shifted by Vm.2D.
func (b *Builder) VSSHL2D(d, n, m int) *Builder { return b.sshl("sshl.2d", d, n, m) }

func (b *Builder) sshl(isa string, ops ...int) *Builder {
	return b.Raw("%s", gap.Lookup("arm64", isa).Word(ops...))
}
