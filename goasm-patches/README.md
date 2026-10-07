# Patches for the Go assembler

go-asmgen emits Plan 9 assembly and lets `cmd/asm` encode it. When a kernel
needs an instruction `cmd/asm` cannot assemble, go-asmgen emits it as a `WORD`
for the time being. It records the instruction in [`internal/gap`](../internal/gap),
and this directory holds the patch that would teach `cmd/asm` the instruction,
so the `WORD` can go.

Each patch is a `git format-patch` against Go master, named
`<arch>-<topic>.patch`. CI applies them every week (see below).

| Patch | Adds | Status |
|---|---|---|
| `ppc64-vsx-dp.patch` | `XVADDDP` `XVSUBDP` `XVMULDP` `XVDIVDP` `XVMADDADP` `XVMAXDP` `XVMINDP` `XVSQRTDP` | mailed 2026-10-05: [CL 845145](https://go.dev/cl/845145) |
| `loong64-vfmadd-vldrepl.patch` | `VFMADDD` `XVFMADDD` and their 14 siblings (`VF[N]M{ADD,SUB}{F,D}`, V and X); `VMOVQ`/`XVMOVQ` broadcast loads at the ends of every element size's offset range (−2048 was refused for all, as were 2046–2047 for `.B` and 2046 for `.H`) | mailed 2026-10-05: [CL 845165](https://go.dev/cl/845165) |
| `loong64-vstelm-range.patch` | A range check for the element stores `VMOVQ Vd.T[i], off(Rj)` / `XVMOVQ` (vstelm). On Go master (go1.28-devel, 2026-10-04), an out-of-range offset was masked with `&0xff` and **silently** stored elsewhere: `VMOVQ V1.V[0], 1024(R4)` became a store to −1024. Now refused with "offset out of range". go-asmgen does not use these stores; it found the bug while writing the patch above. Go 1.27 does not have them. | mailed 2026-10-05: [CL 845166](https://go.dev/cl/845166) |

## From a registry entry to a CL

An entry in `internal/gap` holds what a patch needs that is not wiring:

- the proposed Plan 9 spelling and operand order;
- the opcode mask;
- reference encodings from an independent assembler (GNU as, Apple as),
  covering every register field at its ends.

`tools/goasmgap` turns an entry into the mechanical part of a CL and checks the rest:

```sh
# 1. Does Go already have it under another name? Search by encoding, not by
#    name: loong64's vldrepl.d turned out to be `VMOVQ off(R), V.V2`. scan
#    matches the opcode only, so it cannot see a range defect such as the
#    refused -2048. Only verify's cases can.
go run ./tools/goasmgap scan -goroot ~/go-master

# 2. The test lines for src/cmd/asm/internal/asm/testdata/<arch>.s, in that
#    file's layout and byte order. Put them next to the closest instruction
#    family.
go run ./tools/goasmgap testdata -arch ppc64

# 3. Wire the instruction into cmd/internal/obj/<arch> by hand (a.out.go or
#    inst.go, the optab and opcode tables). Regenerate anames.go with the
#    tree's own go first on PATH. Add error cases to testdata/<arch>error.s
#    for operands one step past each limit. Then run Go's tests:
PATH=~/go-master/bin:$PATH go generate cmd/internal/obj/ppc64
~/go-master/bin/go test cmd/asm/internal/asm cmd/internal/obj/ppc64

# 4. Hold the result to every independent reference on every target. verify
#    builds cmd/asm from the tree's source, so a stale installed binary
#    cannot pass for the patch.
go run ./tools/goasmgap verify -goroot ~/go-master -arch ppc64 -require
```

A reference must come from an assembler other than Go's: GNU as, Apple `as`, or
`llvm-mc --show-encoding` (for example `--triple=loongarch64 -mattr=+lasx`).
Note in `Source` when a second assembler agrees. Instructions a patch adds
but go-asmgen does not emit (an instruction family's siblings, say) get no
registry entry: there is no method that needs them. Go's own test lines, which
step 3 checks, cover them.

`verify` without `-require` exits 0 on `ABSENT` and `PARTIAL`, since it only
reports the state of things. Use `-require` whenever a script needs a pass or
fail answer.

Then `git format-patch` the commit into this directory, add a row above, and
submit from a Go checkout with `git am <patch>` and `git codereview mail`. The
commit author must be an address of the Gerrit account that signed the Google
CLA. Once mailed, add `<CL number> <patch file>` to `cls.txt`, so the weekly
check follows the CL and tests its current patchset.

## What Go's reviewers asked for

From the review of CL 845145 (ppc64 maintainer, 2026-10-06), applied to
all three CLs:

- **A short commit message.** Two or three sentences: what is added and
  any intentional behaviour change. Do not explain Go assembler syntax or
  list each table edit. The diff shows them.
- **Reuse optab shapes; do not add entries.** Attach new opcodes with
  `opset` to an existing entry of the same shape, and merge entries that
  duplicate a shape. Accepting FPR operands on merged VSX opcodes is
  intentional.
- **Few test lines.** One to three per instruction (for VSX: a VSR, an
  FPR and a VR form), with a blank line between instructions, plus each
  limit a fix changes. The exhaustive reference set stays in
  `internal/gap` and is checked by `goasmgap verify`, not in Go's
  testdata.
- **One-line comments,** such as `/* xx3-form 3 operand opcodes */`, not
  explanations.

## The weekly check

`.github/workflows/goasm-gaps.yml` runs every Monday, and on any pull request
that touches the registry, the tool or a patch. It:

1. runs `verify` against the latest Go release and against Go master;
2. opens an issue if either now fully assembles a registry entry, because the
   `WORD` can then become a mnemonic;
3. reads each mailed CL listed in [`cls.txt`](cls.txt) on Gerrit, and opens
   an issue when one has unresolved review comments or was abandoned: a mailed
   CL is forgotten the same way an unmailed patch is;
4. applies every patch to master (a mailed CL as its CURRENT patchset on
   Gerrit, what the reviewers see, not the local file), runs Go's own `cmd/asm` and
   `cmd/internal/obj/<arch>` tests, and runs `verify -require` for those
   architectures. It fails if a patch no longer applies, if Go's tests fail,
   or if the reference encodings are no longer produced.
