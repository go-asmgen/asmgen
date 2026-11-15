package arm64

import "github.com/go-asmgen/asmgen/internal/gap"

// Signed saturating rounded shift right unsigned narrow (SQRSHRUN): each
// signed wide lane of Vn is shifted right by an immediate with rounding,
// ((x + 2^(shift-1)) >> shift), clipped to the unsigned range of the narrow
// lane and written to Vd. It is the last step of every fixed-point filter
// whose result is a pixel: H.264's Clip1Y((b1 + 16) >> 5) is one SQRSHRUN
// with shift 5 on eight sixteen-bit lanes.
//
// TRANSITIONAL, like the by-element forms in elem.go: Go's assembler (1.27.4)
// has VSQXTUN and VSRSHR but not this fused form, so these methods emit a
// WORD commented with the proposed spelling (VSQRSHRUN $shift, Vn.<wide>,
// Vd.<narrow>; the "2" forms write the high half of Vd, VSQRSHRUN2). The
// encodings and their Apple clang references live in internal/gap.
//
// Operands are in A64 order: (d, n, shift). The shift is 1 to the narrow lane
// width in bits (8, 16, 32); a register or shift outside the range panics at
// generation time.

// VSQRSHRUN8B emits Vd.8B = narrow(Vn.8H, shift): the low half of Vd.
func (b *Builder) VSQRSHRUN8B(d, n, shift int) *Builder {
	return b.narrow("sqrshrun.8b", d, n, shift)
}

// VSQRSHRUN2x16B emits the high half of Vd.16B from Vn.8H.
func (b *Builder) VSQRSHRUN2x16B(d, n, shift int) *Builder {
	return b.narrow("sqrshrun2.16b", d, n, shift)
}

// VSQRSHRUN4H emits Vd.4H = narrow(Vn.4S, shift): the low half of Vd.
func (b *Builder) VSQRSHRUN4H(d, n, shift int) *Builder {
	return b.narrow("sqrshrun.4h", d, n, shift)
}

// VSQRSHRUN2x8H emits the high half of Vd.8H from Vn.4S.
func (b *Builder) VSQRSHRUN2x8H(d, n, shift int) *Builder {
	return b.narrow("sqrshrun2.8h", d, n, shift)
}

// VSQRSHRUN2S emits Vd.2S = narrow(Vn.2D, shift): the low half of Vd.
func (b *Builder) VSQRSHRUN2S(d, n, shift int) *Builder {
	return b.narrow("sqrshrun.2s", d, n, shift)
}

// VSQRSHRUN2x4S emits the high half of Vd.4S from Vn.2D.
func (b *Builder) VSQRSHRUN2x4S(d, n, shift int) *Builder {
	return b.narrow("sqrshrun2.4s", d, n, shift)
}

func (b *Builder) narrow(isa string, ops ...int) *Builder {
	return b.Raw("%s", gap.Lookup("arm64", isa).Word(ops...))
}
