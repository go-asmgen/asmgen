package loong64

// Privileged CSR access (LoongArch Vol. 1 §4.2.1), for bare-metal code at
// PLV0: csr is the 14-bit CSR number, 0 to 0x3fff.
//
// TRANSITIONAL. cmd/asm has none of the three (Go master, 2026-10-25), so
// they are emitted as WORDs whose comment is the spelling proposed in
// go.dev/cl/853786. The encodings and their references live in internal/gap.

// CSRRD emits csrrd: R[rd] = CSR[csr].
func (b *Builder) CSRRD(rd, csr int) *Builder { return b.word("csrrd", rd, csr) }

// CSRWR emits csrwr: CSR[csr] and R[rd] are exchanged.
func (b *Builder) CSRWR(rd, csr int) *Builder { return b.word("csrwr", rd, csr) }

// CSRXCHG emits csrxchg: the bits of CSR[csr] selected by the mask R[rj] are
// replaced by those of R[rd], and R[rd] receives the old CSR value. rj must
// not be 0 or 1, which would encode csrrd or csrwr.
func (b *Builder) CSRXCHG(rd, rj, csr int) *Builder { return b.word("csrxchg", rd, rj, csr) }
