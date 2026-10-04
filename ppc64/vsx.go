package ppc64

import "github.com/go-asmgen/asmgen/internal/gap"

// VSX float64 arithmetic the Go assembler does not have.
//
// cmd/asm has the VSX loads, stores, splats and permutes (LXVD2X, LXVDSX,
// STXVD2X, XXPERMDI, ...) but none of the vector double-precision arithmetic.
// Registers are VSX numbers 0–63 (VS0–VS31 overlay F0–F31, VS32–VS63 overlay
// V0–V31).
//
// TRANSITIONAL. go-asmgen emits Plan 9 text and lets cmd/asm encode it; these
// methods emit WORDs only because no Go release, master included (checked
// 2026-10-04), assembles these instructions. Each WORD's comment is the
// spelling proposed for cmd/asm. The encodings, the GNU as references they are
// held to and that spelling live in internal/gap, from which tools/goasmgap
// derives the cmd/asm patch (goasm-patches/) and notices when a Go release
// catches up; these methods then emit the mnemonic instead.

// XVADDDP emits xvadddp: VSR[t] = VSR[a] + VSR[b], two float64 lanes.
func (b *Builder) XVADDDP(t, a, bb int) *Builder { return b.vsx("xvadddp", t, a, bb) }

// XVSUBDP emits xvsubdp: VSR[t] = VSR[a] - VSR[b].
func (b *Builder) XVSUBDP(t, a, bb int) *Builder { return b.vsx("xvsubdp", t, a, bb) }

// XVMULDP emits xvmuldp: VSR[t] = VSR[a] × VSR[b] (each product rounded once).
func (b *Builder) XVMULDP(t, a, bb int) *Builder { return b.vsx("xvmuldp", t, a, bb) }

// XVDIVDP emits xvdivdp: VSR[t] = VSR[a] / VSR[b].
func (b *Builder) XVDIVDP(t, a, bb int) *Builder { return b.vsx("xvdivdp", t, a, bb) }

// XVMADDADP emits xvmaddadp: VSR[t] = VSR[a] × VSR[b] + VSR[t], fused (one
// rounding); the accumulator form a dot product or GEMM uses.
func (b *Builder) XVMADDADP(t, a, bb int) *Builder { return b.vsx("xvmaddadp", t, a, bb) }

// XVMAXDP emits xvmaxdp: VSR[t] = max(VSR[a], VSR[b]) per lane. Its NaN rule
// is the ISA's (a NaN operand yields the other one where it is a number), not
// Go's NaN-propagating max: a caller that needs the latter must test for NaN.
func (b *Builder) XVMAXDP(t, a, bb int) *Builder { return b.vsx("xvmaxdp", t, a, bb) }

// XVMINDP emits xvmindp: VSR[t] = min(VSR[a], VSR[b]) per lane (same NaN rule).
func (b *Builder) XVMINDP(t, a, bb int) *Builder { return b.vsx("xvmindp", t, a, bb) }

// XVSQRTDP emits xvsqrtdp: VSR[t] = sqrt(VSR[b]), correctly rounded.
func (b *Builder) XVSQRTDP(t, bb int) *Builder { return b.vsx("xvsqrtdp", t, bb) }

// vsx emits the WORD the gap registry encodes, which also refuses a register
// outside VS0–VS63 (it would spill into a neighbouring field).
func (b *Builder) vsx(isa string, ops ...int) *Builder {
	return b.Raw("%s", gap.Lookup("ppc64", isa).Word(ops...))
}
