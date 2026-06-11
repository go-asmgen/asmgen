Tracking issue: golang/go#79958 (FILED 2026-06-11; use "Updates #79958" on all
three so the issue isn't auto-closed before all land).

REBASE NEEDED (the .patch files were cut against Go ~2025-08-27 and have drifted
against current master, e.g. HEAD f2f369db): plain `git apply` and `git apply
--3way` both fail. Re-apply each change to the current tree, then regenerate
anames and validate:
  - arm64 VMUL: opcode enum in a.out.go (~L952 region), optab in asm7.go, then
    `go generate ./src/cmd/internal/obj/arm64`.
  - riscv VRORVI: cpu.go (~L1119 region) + asm + `go generate .../riscv`.
  - loong64 VPICKVE2GRV: **the patch targets a.out.go but loong64 now uses
    `cpu.go`** — re-target to cpu.go + `go generate .../loong64`.
  Then `cd src && ./make.bash` and `go test cmd/asm/internal/asm` (the golden
  encodings) before `git codereview mail`. Content (encodings) is unchanged and
  llvm-mc-verified; only placement drifted.

# Submitting these upstream

`UPSTREAM-ISSUE.md` is the issue body. The three verified `.patch` files are the
CLs. The CLA is signed.

## Workflow

1. **File the issue** on github.com/golang/go using `UPSTREAM-ISSUE.md` (its H1 is
   the title). Note the number `#NNNNN`.
2. **Mail each CL** from a full Go checkout (go.googlesource.com/go, not shallow):
   ```sh
   git apply /path/to/<patch>
   go generate ./src/cmd/internal/obj/<arch>/...   # regenerate anames.go cleanly
   go test cmd/asm/internal/asm                     # golden-encoding tests
   git commit -a                                    # message below
   git codereview mail                              # first time: git codereview change
   ```

## CL commit messages (Go house style)

**go-vmul-neon.patch**
```
cmd/internal/obj/arm64: add NEON integer VMUL

Add per-element integer multiply VMUL Vm.<T>, Vn.<T>, Vd.<T> for the
.8B/.16B/.4H/.8H/.2S/.4S arrangements (.2D rejected: no 64-bit NEON integer
multiply). Mirrors the three-same VADD family.

VMUL V1.S4, V2.S4, V3.S4 encodes to 0x4ea19c43 = mul v3.4s, v2.4s, v1.4s.

Updates #NNNNN
```

**go-vror-rvv-zvbb.patch**
```
cmd/internal/obj/riscv: add Zvbb vector rotate VRORVI (vror.vi)

Add per-element vector rotate-right-immediate VRORVI $n, Vs2, Vd (OPIVI,
funct6 0x14), immediate 0..31. Mirrors VSRLVI.

VRORVI $7, V2, V3 encodes to 0x5223b1d7 = vror.vi v3, v2, 7.

Updates #NNNNN
```

**go-vpickve2gr-loong64.patch**
```
cmd/internal/obj/loong64: add VPICKVE2GRV (vpickve2gr.d) mnemonic

Add VPICKVE2GRV $ui1, Vj, Rd to extract a 64-bit LSX lane into a GPR. The
encoding exists in the backend but is only reachable via the overloaded VMOVQ
element syntax; this adds a discoverable vector->GPR mnemonic.

VPICKVE2GRV $0, V2, R12 encodes to 0x72eff04c = vpickve2gr.d $r12, $vr2, 0.

Updates #NNNNN
```

## Priority

VMUL (arm64) and vror.vi (riscv64) are true **blockers** — kernels can't be
expressed without them. vpickve2gr (loong64) is an **ergonomic** addition (the
encoding is reachable via `VMOVQ V.V[i]` today). If the Go team prefers fewer
CLs, the first two are the ones that matter.
