<p align="center"><img src="https://raw.githubusercontent.com/go-asmgen/brand/main/social/go-asmgen.png" alt="go-asmgen/asmgen" width="720"></p>

# go-asmgen

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-0079A8)](https://go-asmgen.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8)](https://go.dev/dl/)
[![Status](https://img.shields.io/badge/status-6%20arches%20live-1a7f37)](#roadmap)

Ergonomic generation of Go-compatible Plan 9 assembly for **every 64-bit Go
target** — **amd64**, **arm64**, **riscv64**, **loong64**, **ppc64le**, and
**s390x**. Encoding is
delegated to the Go toolchain assembler (`cmd/asm`); go-asmgen computes the ABI0
frame layout and emits well-formed Plan 9 instruction text.

`avo` does this for amd64 by encoding instruction bytes itself — powerful, but
exactly what makes extending it to new ISAs expensive. go-asmgen instead emits
Plan 9 text and lets `cmd/asm` encode, so each architecture is just a thin
move/register surface over a **shared** ABI0 layout model ([`abi`](abi)). avo
remains the richer choice for amd64-specific work; go-asmgen's niche is **one
uniform builder across all six targets** (and it is the only such tool for the
non-amd64 ones — now including ppc64le's VSX and s390x's vector facility). The architectures differ only in their move tables:

| 8-byte int | float32 | float64 | |
|---|---|---|---|
| `MOVQ` | `MOVSS` | `MOVSD` | amd64 |
| `MOVD` | `FMOVS` | `FMOVD` | arm64 |
| `MOV` | `MOVF` | `MOVD` | riscv64 |
| `MOVV` | `MOVF` | `MOVD` | loong64 |
| `MOVD` | `FMOVS` | `FMOVD` | ppc64le |
| `MOVD` | `FMOVS` | `FMOVD` | s390x (big-endian) |

## Seventh target: wasm-SIMD (v128)

Go's compiler does not emit `v128` from Go source, and Plan 9 assembly has
no wasm dialect — so wasm-SIMD kernels have to be external, imported via
`//go:wasmimport`. The [`wasm`](wasm) package ports the same layering
pattern to a seventh target: a programmatic WAT text emitter that
`wat2wasm` compiles into `.wasm`, byte-equivalent to a hand-authored
kernel and drift-gated against the generator in consumer CI.

Thirteen kernels ship in [`examples/wasm/`](examples/wasm) (matchlen, hex,
hex_decode, popcount, toupper, memchr, isascii, utf8len, json_clean,
adler32, base64_encode, indexany4, base64_decode), each with a
golden-file test and a wazero cross-check against a Go stdlib reference.
The CI `wasm-e2e` job regenerates every kernel, drift-gates against the
committed golden, compiles via `wat2wasm`, and runs the wazero verifier
on all thirteen. See the
[wasm docs section](https://go-asmgen.github.io/docs/latest/wasm/) for
the emitter surface, kernel signatures, and the drift + regression CI
pattern used by consumers.

The wasm package was previously shipped as a standalone
`go-asmgen/wasm` module (tags v0.1.0 – v0.3.0) — that repo has since
been retired in favor of this fold. Pin
`github.com/go-asmgen/asmgen/wasm/…` and
`github.com/go-asmgen/asmgen/examples/wasm/<kernel>`.

## Status

v0 — **amd64**, **arm64**, **riscv64**, **loong64**, **ppc64le**, **s390x**,
**ABI0**. Correct for
sequences of scalars in any combination: signed/unsigned integers of 1/2/4/8
bytes, pointers, and 32/64-bit floats. Each builder selects the right move per
type and the shared layout computes ABI0 offsets (result area word-aligned,
sub-word loads sign/zero-extended). Every emitted offset and access width is
cross-checked by `go vet` asmdecl and exercised by runtime tests — natively on
amd64 and arm64, and under qemu-user for riscv64, loong64, ppc64le, and s390x
(s390x exercising the big-endian path).

**Aggregates and arrays** are supported: struct, slice, and string parameters
(fields addressed as `name_field+offset(FP)`, e.g. `s_base`/`s_len`/`s_cap`) and
fixed-size `[n]T` arrays passed by value (element-wise, `name_0…name_(n-1)`).
The layout math is architecture-independent and proven generically in `abi`'s
own tests; **amd64, arm64, riscv64, and loong64** additionally have dedicated,
runtime-tested example programs for both — see [`examples`](examples). ppc64le
and s390x don't have dedicated aggregate/array examples yet (SIMD examples
exist for all six — see below).

**SIMD** works through the `Raw` escape hatch over loaded pointers: go-asmgen lays
out the ABI0 frame and the vector body is emitted directly. Runtime-tested
packed-add on all six targets — **SSE2 + AVX2** (amd64), **NEON** (arm64),
**RVV** (riscv64), **LSX + LASX** (loong64), **VSX** (ppc64le), **vector facility**
(s390x, big-endian), up to 256-bit. See
[`examples/simd`](examples/simd).

**Stack frames and TEXT flags**: `NewFuncFlags` emits any flags (`NOSPLIT`,
`NOSPLIT|NOFRAME`, or none for the stack-growth preamble); `frameSize > 0`
reserves locals addressed `name-N(SP)`. See [`examples/frame`](examples/frame).

**CPU feature probes**: `amd64.FeatureProbe(name, amd64.AVX2)` emits the
CPUID/XGETBV gate a dispatched kernel needs in front of it, as a `func() bool`,
so a package that wants no runtime dependency does not have to hand-write CPUID
to get one. The OS half is the part hand-rolled probes drop: a CPU can report
AVX2 on a kernel that does not save YMM state. `POPCNT`, `FMA` (CPUID leaf 1,
separate from AVX2's leaf-7 bit) and `AVX512F` are
there too (`AVX512F` also requires the OS to save opmask and ZMM state, XCR0
mask 0xE6; on macOS it answers false, since Darwin enables that state lazily),
and the list stops at what has callers. `amd64.VendorProbe(name, "GenuineIntel")`
(v0.12.0) compares CPUID leaf 0's vendor string, for tuning choices measured to
differ by vendor (go-fft's radix rule); it never gates correctness, since a
hypervisor may report its own vendor.

**arm64 vector float64 arithmetic**: `Builder.VFADD2D`, `VFSUB2D`, `VFMUL2D`,
`VFNEG2D`, `VFMLA2D` and `VFMLS2D` emit `VFADD`/`VFSUB`/`VFMUL`/`VFNEG`/`VFMLA`/
`VFMLS` on the `.D2` arrangement and leave the encoding to `cmd/asm`, which has
had the first four since Go 1.27; the module therefore requires Go 1.27 (1.27.1 since v0.15.2). (Up
to v0.14 they were hand-encoded `WORD`s; the output is now mnemonics, the
machine code is the same.) A test assembles them with `cmd/asm` and compares
the result with the system assembler's encodings. Kernels built from them ran
on Apple M4 and Neoverse-N1 against Go's own arithmetic and `math.FMA`:
bit-identical over 2.4M lanes.

**ppc64le VSX float64 arithmetic the Go assembler lacks**: `cmd/asm` has the
VSX loads, stores and permutes (`LXVD2X`, `LXVDSX`, `STXVD2X`, `XXPERMDI`) but
no vector double arithmetic. `Builder.XVADDDP`, `XVSUBDP`, `XVMULDP`,
`XVDIVDP`, `XVMADDADP` (fused), `XVMAXDP`, `XVMINDP` and `XVSQRTDP` emit them as
`WORD`s over VSX registers 0–63. Each `WORD` carries the spelling proposed for
`cmd/asm` as its comment. The encodings are pinned against GNU as 2.44 on a
POWER9 (70 cases, every register field at 0, 31, 32 and 63). The example
kernels in `examples/simd/ppc64` were run on POWER8 and POWER9 against Go's
arithmetic, `math.Sqrt` and `math.FMA`, and the results were bit-identical.
(`XVMAXDP`/`XVMINDP` follow the ISA's NaN rule, not Go's NaN-propagating
`max`.)

**loong64 LSX/LASX fused multiply-add and broadcast load**:
`Builder.XVFMADDD`/`VFMADDD` (fused, four/two lanes) are `WORD`s, because
`cmd/asm` has no vector float FMA (`VMADDV`/`XVMADDV` are the integer ones).
`XVLDREPLD`/`VLDREPLD` (one float64 into every lane) emit `XVMOVQ off(R),
X.V4` / `VMOVQ off(R), V.V2`, which is how `cmd/asm` spells `xvldrepl.d` /
`vldrepl.d`. It has done so since at least Go 1.26. The one exception is
offset −2048: the ISA allows it, `cmd/asm` refuses it, so it stays a `WORD`.
The encodings are pinned against GNU as 2.43 on a Loongson 3C5000L (32 cases),
and the example kernels in `examples/simd/loong64` were run on that machine
against `math.FMA`, with bit-identical results.

**Instructions `cmd/asm` lacks are transitional.** Every `WORD` above has an
entry in `internal/gap`: its encoding, the independent references, the
opcode mask, and the Plan 9 spelling proposed upstream. `tools/goasmgap`
works from that registry:

- it finds an instruction in Go's test data by encoding, under whatever name
  Go gives it;
- it writes the `cmd/asm` test lines for an upstream patch;
- it holds a toolchain to every reference case.

The patches live in [`goasm-patches/`](goasm-patches/). A weekly workflow
opens an issue when a Go release or master gains one of these instructions,
and checks that the patches still apply to master and still encode correctly.

A typed vector-load helper (to drop the `Raw` boilerplate) and first-class vector
*types* are the main remaining items.

## Use it as a library

Three small packages: an architecture builder (`amd64` / `arm64` / `riscv64` /
`loong64` / `ppc64` / `s390x`), the ABI0 layout model (`abi`), and the Plan 9 file
writer (`emit`).

```sh
go get github.com/go-asmgen/asmgen@latest
```

```go
package main

import (
	"os"

	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	// func add(a, b int64) int64
	sig := arm64.Layout(
		[]string{"a", "b"}, []arm64.Type{arm64.Int64, arm64.Int64},
		[]string{"ret"}, []arm64.Type{arm64.Int64},
	)
	b := arm64.NewFunc("add", sig, 0)
	b.LoadArg("a", "R0").
		LoadArg("b", "R1").
		Raw("ADD R1, R0, R2").
		StoreRet("R2", "ret").
		Ret()

	f := emit.NewFile("arm64")
	f.Add(b.Func())
	os.WriteFile("add_arm64.s", []byte(f.String()), 0o644)
}
```

For struct/slice/string parameters, build the layout with
[`abi`](abi)`.LayoutArgs` and the `Struct`/`Slice`/`String` constructors; see
[`examples/aggregate`](examples/aggregate).

## Validate locally (Go toolchain required)

The library packages (`abi`, `emit`, `amd64`, `arm64`, `riscv64`, `loong64`,
`ppc64`, `s390x`, `wasm`) are architecture-independent and held to 100% test
coverage:

```sh
go test ./abi/... ./emit/... ./amd64/... ./arm64/... ./riscv64/... ./loong64/... ./ppc64/... ./s390x/... ./wasm/... ./internal/...
```

The generated assembly is the real test of correctness. On an arm64 host (Apple
Silicon or arm64 Linux):

```sh
go generate ./examples/add/... ./examples/types/...
GOARCH=arm64 go vet ./examples/add/... ./examples/types/...   # asmdecl
go test ./examples/add/... ./examples/types/...               # runtime
```

riscv64, loong64, ppc64le and s390x have no common native host, so validate them statically
anywhere (asmdecl + `cmd/asm`) and run them under emulation (shown for riscv64;
loong64 is identical with `GOARCH=loong64` / `qemu-loongarch64-static`):

```sh
go generate ./examples/riscv64/...
GOOS=linux GOARCH=riscv64 go vet ./examples/riscv64/...    # asmdecl
GOOS=linux GOARCH=riscv64 go build ./examples/riscv64/...  # cmd/asm checks mnemonics
# runtime under qemu-user (e.g. in CI), or via Docker's emulation:
GOARCH=riscv64 go test -exec=qemu-riscv64-static ./examples/riscv64/...
```

### What CI checks (in order)

1. **`go vet` asmdecl passes.** The cheapest, strongest check: it verifies every
   `name+offset(FP)` in the `.s` matches the Go declaration. Wrong offsets are
   caught before runtime.
2. **`cmd/asm` accepts every instruction**, and the committed `.s` is regenerated
   and diffed — a stale or invalid `.s` fails the build.
3. **Runtime test** — the function is actually called and its result checked:
   natively on amd64 and arm64 runners, under qemu-user for riscv64, loong64,
   ppc64le and s390x.
4. **100% library coverage** is gated on `abi`, `emit`, `internal/gap` and the seven builders.
5. **govulncheck** finds no known vulnerability. [SECURITY.md](SECURITY.md)
   says what counts as one in a code generator, and how to report it.

## Roadmap

- v0: arm64, ABI0, 8-byte int/ptr args. (done)
- Widen arm64 scalar support: 1/2/4-byte signed/unsigned ints, pointers, float
  regs (F0.., `FMOVS`/`FMOVD`), word-aligned result area, sub-word sign/zero
  extension. Validated against asmdecl + runtime tests. (done)
- Extract the shared ABI0 model (`internal/abi`) and add **riscv64** as a thin
  second architecture over it, runtime-proven under qemu-user. (done)
- Add **loong64** as a third architecture — same recipe, just a move table
  (`MOVV`, `MOVF`/`MOVD`), runtime-proven under qemu-user. (done)
- **Aggregates**: struct/slice/string parameters laid out by Go's struct rules,
  fields addressed as `name_field+offset(FP)`, asmdecl- and runtime-validated.
  (done)
- Make the library importable: promote `abi` and `emit` out of `internal/`. (done)
- Add **amd64** (fourth target; richer move table with `MOVxQSX/ZX` sub-word
  loads and SSE float moves), runtime-proven natively, and **SIMD** examples
  (amd64 SSE + arm64 NEON packed add) through `Raw`. (done — here)
- First-class vector *types* (pass `[4]float32` by value; RVV on riscv64,
  LSX/LASX on loong64 — all six assemblers support SIMD), and array value args.
- Optional: derive instruction mnemonic tables from cmd/internal/obj to catch
  typos at generation time (still delegating encoding to cmd/asm).

## Design notes

- ABI0 not ABIInternal: ABIInternal is an unstable internal contract that can
  change between Go releases. ABI0 (stack-based, FP-relative) is stable and is
  what hand-written .s targets.
- NOSPLIT by default in v0: avoids stack-growth preamble. Revisit for functions
  with large frames or that call other functions.
