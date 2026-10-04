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
| `ppc64-vsx-dp.patch` | `XVADDDP` `XVSUBDP` `XVMULDP` `XVDIVDP` `XVMADDADP` `XVMAXDP` `XVMINDP` `XVSQRTDP` | ready, not yet mailed |
| — | loong64 `VFMADDD`/`XVFMADDD`; `(X)VMOVQ` broadcast load at offset −2048 | registry entry only |

Two earlier prototypes are not here because Go already has them: arm64 integer
`VMUL` and riscv64 `VRORVI` (Zvbb) both shipped in Go 1.27.

## From a registry entry to a CL

An entry in `internal/gap` holds what a patch needs that is not wiring:

- the proposed Plan 9 spelling and operand order;
- the opcode mask;
- reference encodings from an independent assembler (GNU as, Apple as),
  covering every register field at its ends.

`tools/goasmgap` turns an entry into the mechanical part of a CL and checks the rest:

```sh
# 1. Does Go already have it under another name? Search by encoding, not by
#    name: loong64's vldrepl.d turned out to be `VMOVQ off(R), V.V2`.
go run ./tools/goasmgap scan -goroot ~/go-master

# 2. The test lines for src/cmd/asm/internal/asm/testdata/<arch>.s, in that
#    file's layout and byte order.
go run ./tools/goasmgap testdata -arch ppc64

# 3. Wire the instruction into cmd/internal/obj/<arch> by hand (a.out.go,
#    anames.go via go generate, the optab and opcode tables), build, then
#    hold the result to every reference encoding on every target:
go run ./tools/goasmgap verify -goroot ~/go-master -arch ppc64 -require
```

`verify` prints one verdict per instruction and target: `SUPPORTED`, `PARTIAL`
(names the refused cases), `ABSENT`, or `WRONG` (it assembles to a different
word, which always fails). Before it trusts a refusal, it checks that the
toolchain can assemble at all.

Then `git format-patch` the commit into this directory, add a row above, and
submit from a Go checkout with `git am <patch>` and `git codereview mail`.

## The weekly check

`.github/workflows/goasm-gaps.yml` runs every Monday, and on any pull request
that touches the registry, the tool or a patch. It:

1. runs `verify` against the latest Go release and against Go master;
2. opens an issue if either now fully assembles a registry entry, because the
   `WORD` can then become a mnemonic;
3. applies every patch here to master, rebuilds `cmd/asm`, and runs
   `verify -require` for those architectures. It fails if a patch no longer
   applies, or no longer produces the reference encodings.
