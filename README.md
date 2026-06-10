# go-asmgen

Ergonomic generation of Go-compatible Plan 9 assembly for **every 64-bit Go
target** — **amd64**, **arm64**, **riscv64**, and **loong64**. Encoding is
delegated to the Go toolchain assembler (`cmd/asm`); go-asmgen computes the ABI0
frame layout and emits well-formed Plan 9 instruction text.

`avo` does this for amd64 by encoding instruction bytes itself — powerful, but
exactly what makes extending it to new ISAs expensive. go-asmgen instead emits
Plan 9 text and lets `cmd/asm` encode, so each architecture is just a thin
move/register surface over a **shared** ABI0 layout model ([`abi`](abi)). avo
remains the richer choice for amd64-specific work; go-asmgen's niche is **one
uniform builder across all four targets** (and it is the only such tool for the
non-amd64 ones). The architectures differ only in their move tables:

| 8-byte int | float32 | float64 | |
|---|---|---|---|
| `MOVQ` | `MOVSS` | `MOVSD` | amd64 |
| `MOVD` | `FMOVS` | `FMOVD` | arm64 |
| `MOV` | `MOVF` | `MOVD` | riscv64 |
| `MOVV` | `MOVF` | `MOVD` | loong64 |

## Status

v0 — **amd64**, **arm64**, **riscv64**, **loong64**, **ABI0**. Correct for
sequences of scalars in any combination: signed/unsigned integers of 1/2/4/8
bytes, pointers, and 32/64-bit floats. Each builder selects the right move per
type and the shared layout computes ABI0 offsets (result area word-aligned,
sub-word loads sign/zero-extended). Every emitted offset and access width is
cross-checked by `go vet` asmdecl and exercised by runtime tests — natively on
amd64 and arm64, and under qemu-user for riscv64 and loong64.

**Aggregates** are supported too: struct, slice, and string parameters are laid
out by Go's struct rules and each field is addressed as `name_field+offset(FP)`
(e.g. `s_base`/`s_len`/`s_cap` for a slice) — exactly what asmdecl validates. See
[`examples/aggregate`](examples/aggregate).

**SIMD** works through the `Raw` escape hatch over the loaded pointers: go-asmgen
lays out the ABI0 frame and the vector body (SSE/AVX on amd64, NEON on arm64) is
emitted directly. See [`examples/simd`](examples/simd) — packed-add, runtime
tested. Vector *type* args (passing `[4]float32` by value) and `cmd/asm`-modelled
vector helpers are future work.

Not yet correct for arrays as value parameters or first-class vector types.

## Use it as a library

Three small packages: an architecture builder (`amd64` / `arm64` / `riscv64` /
`loong64`), the ABI0 layout model (`abi`), and the Plan 9 file writer (`emit`).

```sh
go get github.com/go-asmgen/asmgen@v0.1.0
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

The library packages (`abi`, `emit`, `amd64`, `arm64`, `riscv64`, `loong64`) are
architecture-independent and held to 100% test coverage:

```sh
go test ./abi/... ./emit/... ./amd64/... ./arm64/... ./riscv64/... ./loong64/...
```

The generated assembly is the real test of correctness. On an arm64 host (Apple
Silicon or arm64 Linux):

```sh
go generate ./examples/add/... ./examples/types/...
GOARCH=arm64 go vet ./examples/add/... ./examples/types/...   # asmdecl
go test ./examples/add/... ./examples/types/...               # runtime
```

riscv64 and loong64 have no common native host, so validate them statically
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
   natively on amd64 and arm64 runners, under qemu-user for riscv64 and loong64.
4. **100% library coverage** is gated on `abi`, `emit`, and the four builders.

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
  LSX/LASX on loong64 — all four assemblers support SIMD), and array value args.
- Optional: derive instruction mnemonic tables from cmd/internal/obj to catch
  typos at generation time (still delegating encoding to cmd/asm).

## Design notes

- ABI0 not ABIInternal: ABIInternal is an unstable internal contract that can
  change between Go releases. ABI0 (stack-based, FP-relative) is stable and is
  what hand-written .s targets.
- NOSPLIT by default in v0: avoids stack-growth preamble. Revisit for functions
  with large frames or that call other functions.
