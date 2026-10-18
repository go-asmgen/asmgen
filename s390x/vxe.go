package s390x

import "github.com/go-asmgen/asmgen/internal/gap"

// Vector float32 arithmetic the Go assembler does not have.
//
// The vector-enhancements facility 1 (z14 and later) added the short (float32)
// format to the vector floating-point instructions. cmd/asm has the long
// (float64) forms (VFADB, WFADB, VFMADB, ...) but none of the short ones. The
// V forms operate on the four float32 lanes of a vector register, the W forms
// on element 0 alone. Registers are vector numbers 0–31; operands are in ISA
// order, the target first.
//
// TRANSITIONAL. go-asmgen emits Plan 9 text and lets cmd/asm encode it; these
// methods emit raw bytes only because no Go release, master included (checked
// 2026-10-18), assembles these instructions. Each is 6 bytes, emitted as
// WORD $0x<first four>; BYTE $0x<fifth>; BYTE $0x<sixth> on one line, with the
// spelling go.dev/cl/846286 gives it as the comment. The encodings, the GNU as
// references they are held to and that spelling live in internal/gap; when Go
// assembles them, these methods emit the mnemonic instead.
//
// A kernel using them needs a z14 or later: check
// golang.org/x/sys/cpu.S390X.HasVXE before calling it.

// VFASB emits vfasb: V[v1] = V[v2] + V[v3], four float32 lanes.
func (b *Builder) VFASB(v1, v2, v3 int) *Builder { return b.vxe("vfasb", v1, v2, v3) }

// WFASB emits wfasb: element 0 of V[v1] = V[v2] + V[v3] (element 0).
func (b *Builder) WFASB(v1, v2, v3 int) *Builder { return b.vxe("wfasb", v1, v2, v3) }

// VFSSB emits vfssb: V[v1] = V[v2] - V[v3].
func (b *Builder) VFSSB(v1, v2, v3 int) *Builder { return b.vxe("vfssb", v1, v2, v3) }

// WFSSB emits wfssb, the element-0 form of VFSSB.
func (b *Builder) WFSSB(v1, v2, v3 int) *Builder { return b.vxe("wfssb", v1, v2, v3) }

// VFMSB emits vfmsb: V[v1] = V[v2] × V[v3].
func (b *Builder) VFMSB(v1, v2, v3 int) *Builder { return b.vxe("vfmsb", v1, v2, v3) }

// WFMSB emits wfmsb, the element-0 form of VFMSB.
func (b *Builder) WFMSB(v1, v2, v3 int) *Builder { return b.vxe("wfmsb", v1, v2, v3) }

// VFDSB emits vfdsb: V[v1] = V[v2] / V[v3].
func (b *Builder) VFDSB(v1, v2, v3 int) *Builder { return b.vxe("vfdsb", v1, v2, v3) }

// WFDSB emits wfdsb, the element-0 form of VFDSB.
func (b *Builder) WFDSB(v1, v2, v3 int) *Builder { return b.vxe("wfdsb", v1, v2, v3) }

// VFMASB emits vfmasb: V[v1] = V[v2] × V[v3] + V[v4], fused (one rounding).
func (b *Builder) VFMASB(v1, v2, v3, v4 int) *Builder { return b.vxe("vfmasb", v1, v2, v3, v4) }

// WFMASB emits wfmasb, the element-0 form of VFMASB.
func (b *Builder) WFMASB(v1, v2, v3, v4 int) *Builder { return b.vxe("wfmasb", v1, v2, v3, v4) }

// VFMSSB emits vfmssb: V[v1] = V[v2] × V[v3] - V[v4], fused (one rounding).
func (b *Builder) VFMSSB(v1, v2, v3, v4 int) *Builder { return b.vxe("vfmssb", v1, v2, v3, v4) }

// WFMSSB emits wfmssb, the element-0 form of VFMSSB.
func (b *Builder) WFMSSB(v1, v2, v3, v4 int) *Builder { return b.vxe("wfmssb", v1, v2, v3, v4) }

// VFSQSB emits vfsqsb: V[v1] = sqrt(V[v2]), correctly rounded.
func (b *Builder) VFSQSB(v1, v2 int) *Builder { return b.vxe("vfsqsb", v1, v2) }

// WFSQSB emits wfsqsb, the element-0 form of VFSQSB.
func (b *Builder) WFSQSB(v1, v2 int) *Builder { return b.vxe("wfsqsb", v1, v2) }

// vxe emits the bytes the gap registry encodes, which also refuses a register
// outside V0–V31 (its high bit would spill into RXB's neighbour).
func (b *Builder) vxe(isa string, ops ...int) *Builder {
	return b.Raw("%s", gap.Lookup("s390x", isa).Word(ops...))
}
