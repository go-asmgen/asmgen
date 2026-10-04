package ppc64

import "fmt"

// VSX float64 arithmetic the Go assembler does not have.
//
// TRANSITIONAL. go-asmgen emits Plan 9 text and lets cmd/asm encode it; these
// methods encode by hand only because no Go release, master included (checked
// 2026-10-04), assembles VSX vector double arithmetic. A patch adding the mnemonics to cmd/internal/obj/ppc64 is being
// prepared; once a Go release carries it, these methods emit the mnemonics
// instead, as the arm64 vector-float methods did when Go 1.27 added theirs.
// Until then, the WORDs bypass cmd/asm's operand checks, which is why vsr
// refuses bad registers here and the encodings are pinned against GNU as.
//
// cmd/asm has the VSX loads, stores, splats and permutes (LXVD2X, LXVDSX,
// STXVD2X, XXPERMDI, ...) but none of the vector double-precision arithmetic:
// no XVADDDP, XVMULDP or XVMADDADP. A float64 kernel on ppc64le either runs
// scalar or encodes them by hand. These methods encode them: each emits one
// WORD with the instruction's Power ISA encoding and its assembly in a comment.
// Registers are VSX numbers 0–63 (VS0–VS31 overlay F0–F31, VS32–VS63 overlay
// V0–V31). The encodings are checked against GNU as 2.44 on a POWER9
// (TestVSXEncodings).
//
// Power ISA 3.0, XX3-form (opcode 60, T/A/B in bits 6–20, an 8-bit XO, and
// the high register bits AX/BX/TX in bits 29–31) and XX2-form for the square
// root (a 9-bit XO, no A).

// XVADDDP emits xvadddp: VSR[t] = VSR[a] + VSR[b], two float64 lanes.
func (b *Builder) XVADDDP(t, a, bb int) *Builder { return b.xx3("xvadddp", 96, t, a, bb) }

// XVSUBDP emits xvsubdp: VSR[t] = VSR[a] - VSR[b].
func (b *Builder) XVSUBDP(t, a, bb int) *Builder { return b.xx3("xvsubdp", 104, t, a, bb) }

// XVMULDP emits xvmuldp: VSR[t] = VSR[a] × VSR[b] (each product rounded once).
func (b *Builder) XVMULDP(t, a, bb int) *Builder { return b.xx3("xvmuldp", 112, t, a, bb) }

// XVDIVDP emits xvdivdp: VSR[t] = VSR[a] / VSR[b].
func (b *Builder) XVDIVDP(t, a, bb int) *Builder { return b.xx3("xvdivdp", 120, t, a, bb) }

// XVMADDADP emits xvmaddadp: VSR[t] = VSR[a] × VSR[b] + VSR[t], fused (one
// rounding); the accumulator form a dot product or GEMM uses.
func (b *Builder) XVMADDADP(t, a, bb int) *Builder { return b.xx3("xvmaddadp", 97, t, a, bb) }

// XVMAXDP emits xvmaxdp: VSR[t] = max(VSR[a], VSR[b]) per lane. Its NaN rule
// is the ISA's (a NaN operand yields the other one where it is a number), not
// Go's NaN-propagating max: a caller that needs the latter must test for NaN.
func (b *Builder) XVMAXDP(t, a, bb int) *Builder { return b.xx3("xvmaxdp", 224, t, a, bb) }

// XVMINDP emits xvmindp: VSR[t] = min(VSR[a], VSR[b]) per lane (same NaN rule).
func (b *Builder) XVMINDP(t, a, bb int) *Builder { return b.xx3("xvmindp", 232, t, a, bb) }

// XVSQRTDP emits xvsqrtdp: VSR[t] = sqrt(VSR[b]), correctly rounded.
func (b *Builder) XVSQRTDP(t, bb int) *Builder {
	vsr(t)
	vsr(bb)
	w := uint32(60)<<26 | uint32(t&31)<<21 | uint32(bb&31)<<11 | uint32(203)<<2 |
		uint32(bb>>5)<<1 | uint32(t>>5)
	return b.Raw("WORD $0x%08x // xvsqrtdp vs%d, vs%d", w, t, bb)
}

func (b *Builder) xx3(mnemonic string, xo uint32, t, a, bb int) *Builder {
	vsr(t)
	vsr(a)
	vsr(bb)
	return b.Raw("WORD $0x%08x // %s vs%d, vs%d, vs%d", xx3(xo, t, a, bb), mnemonic, t, a, bb)
}

func xx3(xo uint32, t, a, b int) uint32 {
	return uint32(60)<<26 | uint32(t&31)<<21 | uint32(a&31)<<16 | uint32(b&31)<<11 |
		xo<<3 | uint32(a>>5)<<2 | uint32(b>>5)<<1 | uint32(t>>5)
}

// vsr refuses a register number outside VS0–VS63: it would spill into the
// neighbouring field of the encoding and name a different instruction.
func vsr(r int) {
	if r < 0 || r > 63 {
		panic(fmt.Sprintf("ppc64: VSX register VS%d does not exist", r))
	}
}
