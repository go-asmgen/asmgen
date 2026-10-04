package loong64

import "github.com/go-asmgen/asmgen/internal/gap"

// LSX/LASX float64 fused multiply-add and broadcast load.
//
// cmd/asm has the vector float64 add/sub/mul/div (VADDD/XVADDD, VMULD/XVMULD,
// ... in Go's naming), loads, stores and lane moves. Vector registers are
// 0–31 (V/X share the numbering; F0–F31 are the low 64 bits).
//
// The broadcast load is in cmd/asm, spelt as an arrangement load: VMOVQ
// off(R), V.V2 is vldrepl.d and XVMOVQ off(R), X.V4 is xvldrepl.d (since at
// least Go 1.26). These methods emit that, so cmd/asm encodes it, except at
// offset -2048, which the ISA's scaled signed 9-bit field holds and cmd/asm
// refuses; that one offset is emitted as a WORD.
//
// TRANSITIONAL. The fused multiply-add is not in cmd/asm at all (Go 1.27.1
// and master, 2026-10-04; VMADDV/XVMADDV are the integer ones), so it is
// emitted as a WORD whose comment is the spelling proposed for cmd/asm. The
// encodings, the GNU as references they are held to and that spelling live in
// internal/gap, from which tools/goasmgap derives the cmd/asm patch
// (goasm-patches/) and notices when a Go release catches up; these methods
// then emit the mnemonic instead.

// XVFMADDD emits xvfmadd.d: X[d] = X[j] × X[k] + X[a], four float64 lanes,
// fused (one rounding).
func (b *Builder) XVFMADDD(d, j, k, a int) *Builder { return b.word("xvfmadd.d", d, j, k, a) }

// VFMADDD emits vfmadd.d: V[d] = V[j] × V[k] + V[a], two float64 lanes, fused.
func (b *Builder) VFMADDD(d, j, k, a int) *Builder { return b.word("vfmadd.d", d, j, k, a) }

// XVLDREPLD emits xvldrepl.d: the float64 at off(R[rj]) in all four lanes of
// X[d]. off is a multiple of 8 in [-2048, 2040].
func (b *Builder) XVLDREPLD(d, rj, off int) *Builder { return b.ldrepl("xvldrepl.d", d, rj, off) }

// VLDREPLD emits vldrepl.d: the float64 at off(R[rj]) in both lanes of V[d].
func (b *Builder) VLDREPLD(d, rj, off int) *Builder { return b.ldrepl("vldrepl.d", d, rj, off) }

func (b *Builder) word(isa string, ops ...int) *Builder {
	return b.Raw("%s", gap.Lookup("loong64", isa).Word(ops...))
}

func (b *Builder) ldrepl(isa string, d, rj, off int) *Builder {
	in := gap.Lookup("loong64", isa)
	w := in.Word(d, rj, off) // refuses what does not encode
	if off == -2048 {
		return b.Raw("%s", w)
	}
	return b.Raw("%s", in.Syntax(d, rj, off))
}
