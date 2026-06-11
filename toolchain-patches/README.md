# RESOLVED / OBSOLETE (2026-06-11)

All three instructions are already available on Go master, so issue golang/go#79958
was closed as obsolete and no CLs were mailed:

- arm64 `VMUL` — upstream in commit b1c8857f95.
- riscv64 `VRORVI` (zvbb) — upstream in commit 58efaf3859.
- loong64 `vpickve2gr.d` — reachable via `VMOVQ Vj.V[index], Rd` (loong64/doc.go); no
  separate mnemonic needed.

Lesson: check upstream HEAD before filing. Follow-up: wire `VMUL`/`VRORVI` into
go-asmgen so NEON-multiply and RVV-rotate kernels use the real instructions.

---

# Go toolchain prototypes — two missing vector instructions

Verified prototypes adding two SIMD instructions to Go's assembler, found missing
while building go-asmgen SIMD kernels. Both are encoding-confirmed by an
independent disassembler (`llvm-mc`) and pass `go test cmd/asm/internal/asm`.
Ready to submit upstream (CLA signed).

## go-vmul-neon.patch — integer `VMUL` for arm64 (NEON)

`VMUL Vm.<T>, Vn.<T>, Vd.<T>` (Vd = Vn * Vm per element), arrangements
.8B/.16B/.4H/.8H/.2S/.4S, `.2D` rejected. Mirrors the existing VADD three-same
family (case 72; oprrr `7<<25 | 1<<21 | 0x27<<10` = base 0x0E209C00).
Verified: `VMUL V1.S4, V2.S4, V3.S4` -> `0x4ea19c43` = `mul v3.4s, v2.4s, v1.4s`.
4 files, 76 lines.

**Unblocks:** vectorized Adler/Rollsum on arm64 — the rdiff weak-checksum SIMD is
currently amd64-only (SSE `PMADDUBSW`) precisely because Go's arm64 assembler had
no integer vector multiply.

## go-vror-rvv-zvbb.patch — `vror.vi` for riscv64 (Zvbb)

`VRORVI $n, Vs2, Vd` (Vd = rotateright(Vs2, n) per element). OPIVI format,
identical to VSRLVI but funct6 0x14. Verified: `VRORVI $7, V2, V3` -> `0x5223b1d7`
= `vror.vi v3, v2, 7`. 6 files, ~17 lines. Immediate 0..31 (covers e32 rotates;
32..63 would need imm[5] folded into funct6 — out of scope).

**Unblocks:** single-instruction RVV rotate. The go-asmgen BLAKE3 mix4 RVV kernel
currently does shift-or (3 ops); with this it's one `vror.vi`, like loong64's
native `VROTRW`.

## Submitting

```sh
# in a real Go checkout (go.googlesource.com/go):
git apply /path/to/go-vmul-neon.patch
git add -A && git commit -m "cmd/internal/obj/arm64: add integer VMUL"
git codereview mail   # -> go-review.googlesource.com
```
Note: `anames.go` is normally regenerated via `go generate ./...` in the obj
package; the hand-edits match the generator output but regenerate for a real CL.
