# cmd/internal/obj: add three missing SIMD instructions blocking real vector kernels (arm64 NEON, riscv64 Zvbb, loong64 LSX)

### Proposal

Add three vector instructions to Go's hand-written assemblers. Each is a single
mnemonic, each fills a concrete gap that today blocks a real SIMD kernel from
being written in Go assembly, and each has a verified encoding (cross-checked
against an independent disassembler, LLVM's `llvm-mc`):

| Arch | ISA ext | Mnemonic | Verified encoding | Decodes to |
|------|---------|----------|-------------------|------------|
| arm64 | NEON | `VMUL Vm.<T>, Vn.<T>, Vd.<T>` | `0x4ea19c43` | `mul v3.4s, v2.4s, v1.4s` |
| riscv64 | Zvbb | `VRORVI $n, Vs2, Vd` | `0x5223b1d7` | `vror.vi v3, v2, 7` |
| loong64 | LSX | `VPICKVE2GRV $ui1, Vj, Rd` | `0x72eff04c` | `vpickve2gr.d $r12, $vr2, 0` |

The patches are ready (one self-contained diff per instruction, with assembler
test-data entries) and the contributor's CLA is signed. We're happy to mail
these as three separate CLs.

### Background / motivation

While building a code generator for SIMD kernels across Go's four 64-bit
targets, we hit the same wall on three of the four architectures: the
hand-written assembler simply has no mnemonic for an instruction the kernel
needs. These are not exotic instructions — they're the integer-vector multiply,
the vector rotate, and the vector-lane-to-GPR extract that every nontrivial SIMD
algorithm reaches for. The gaps mean kernels for BLAKE3 hashing, LZ-style match
finding, and Adler/rolling checksums are either amd64-only today or can't be
expressed at all on these targets.

This is deliberately **distinct from the `archsimd` intrinsics effort**
(#73787). That work is about compiler-level SIMD intrinsics, is amd64-first, and
operates at a different layer. This proposal is the much smaller, orthogonal task
of closing per-instruction holes in the existing hand-written `cmd/internal/obj`
assemblers — the same mechanism that already encodes `VADD`, `VSRL`, `VMOVQ`,
etc. Each addition mirrors an instruction already present in the same family.

### The three instructions

#### 1. arm64 NEON integer `VMUL`

`VMUL Vm.<T>, Vn.<T>, Vd.<T>` — per-element integer multiply (`Vd = Vn * Vm`),
arrangements `.8B/.16B/.4H/.8H/.2S/.4S`; `.2D` is rejected (no 64-bit NEON
integer multiply exists). It mirrors the existing three-same `VADD` family
(optab case 72), base opcode `7<<25 | 1<<21 | 0x27<<10`.

- Verified: `VMUL V1.S4, V2.S4, V3.S4` → `0x4ea19c43` = `mul v3.4s, v2.4s, v1.4s`.
- **Use case:** vectorized Adler-32 / rolling weak checksums (rdiff/librsync
  style). The SIMD weak-checksum path is currently amd64-only (SSE `PMADDUBSW`)
  precisely because arm64 had no integer vector multiply mnemonic. Today Go's
  arm64 assembler exposes floating-point `VFMUL` but no integer `VMUL`. A second
  consumer: vectorised base64 (Lemire) needs an integer multiply for its 6-bit
  field extraction on arm64 NEON, same gap.
- Patch: `go-vmul-neon.patch` (4 files, 76 lines).

#### 2. riscv64 Zvbb `vror.vi`

`VRORVI $n, Vs2, Vd` — per-element vector rotate-right by immediate
(`Vd = rotr(Vs2, n)`). OPIVI format, identical in shape to the existing
`VSRLVI` but with funct6 `0x14`. Immediate range 0..31 (covers e32 element
rotates; 32..63 would require folding `imm[5]` into funct6 and is out of scope
for this CL).

- Verified: `VRORVI $7, V2, V3` → `0x5223b1d7` = `vror.vi v3, v2, 7`.
- **Use case:** the BLAKE3 `mix`/`G` function rotates. Without it, the RVV kernel
  emulates a rotate as shift-left + shift-right + or (3 instructions per rotate);
  with `vror.vi` it's a single instruction, matching what loong64 already does
  natively via `VROTRW`.
- Patch: `go-vror-rvv-zvbb.patch` (6 files, ~17 lines), including
  `riscv64validation.s` entries for the out-of-range immediate diagnostics.

#### 3. loong64 LSX `vpickve2gr.d` (vector → GPR)

`VPICKVE2GRV $ui1, Vj, Rd` — extract a 64-bit lane (`ui1` ∈ {0, 1}) of a 128-bit
LSX vector into a general-purpose register. Encoding
`0x72eff000 | (lane<<10) | (Vj<<5) | Rd`.

- Verified: `VPICKVE2GRV $0, V2, R12` → `0x72eff04c` and `VPICKVE2GRV $1, V2, R12`
  → `0x72eff44c`; the emitted bytes decode under `llvm-mc --disassemble
  -mattr=+lsx` to `vpickve2gr.d $r12, $vr2, 0` / `... 1` respectively
  (`$r12` = ABI alias `$t0`).
- **Use case:** this is the one instruction that unblocks essentially *all* LSX
  reductions. Any horizontal step — locating the first differing byte in a
  SIMD common-prefix / LZ match-finder, reducing a vector compare-mask to a
  branch, reading a popcount/sum lane out — needs to move a vector lane into a
  GPR. There is a `vpickve2gr.d` encoder helper inside the loong64 backend today,
  but it is only reachable through the overloaded `VMOVQ Vj.V[i], Rd`
  element-syntax; there is no plain, discoverable vector→GPR mnemonic the way
  other arches expose one. This CL adds `VPICKVE2GRV` as a first-class mnemonic
  taking `$ui1, Vj, Rd`.
- Patch: `go-vpickve2gr.patch` (3 files, +21 lines).

### Verification method

For all three, the assembler was built from the patched tree and used to emit the
instruction; the resulting 32-bit words were then fed back through `llvm-mc
--disassemble` for the relevant target (`-mattr=+lsx`, RVV/Zvbb, NEON) to confirm
they decode to the intended upstream mnemonic. The arm64 and riscv64 patches also
add `cmd/asm/internal/asm/testdata` golden-encoding lines (and, for riscv64,
validation-error lines) so `go test cmd/asm/internal/asm` covers them.

### Notes for reviewers

- `anames.go` is normally regenerated via `go generate ./...` in the obj package;
  the hand-edits here match the generator output but will be regenerated in the
  mailed CLs.
- These are independent and can land as three separate CLs; we can also split by
  architecture maintainer.
- CLA is signed and we're ready to mail via `git codereview mail`.
