# go-asmgen

Ergonomic generation of Go-compatible Plan 9 assembly for non-amd64
architectures — **arm64** and **riscv64**. Encoding is delegated to the Go
toolchain assembler (`cmd/asm`); go-asmgen computes the ABI0 frame layout and
emits well-formed Plan 9 instruction text.

This is the multi-architecture counterpart to what `avo` does for amd64. avo
encodes instruction bytes itself, which is exactly what makes extending it to
new ISAs expensive. go-asmgen instead emits Plan 9 text and lets `cmd/asm`
encode, so each new architecture only needs a thin move/register surface over
the **shared** ABI0 layout model. riscv64 is exactly that: it reuses
[`internal/abi`](internal/abi) unchanged and differs from arm64 only in
mnemonics (e.g. `MOV` vs `MOVD` for 8-byte ints, `MOVF`/`MOVD` vs `FMOVS`/`FMOVD`
for floats).

## Status

v0 — **arm64** and **riscv64**, **ABI0**. Correct for sequences of scalars in any
combination: signed/unsigned integers of 1/2/4/8 bytes, pointers, and 32/64-bit
floats. Each builder selects the right move per type and the shared layout
computes ABI0 offsets (result area word-aligned, sub-word loads sign/zero-
extended). Every emitted offset and access width is cross-checked by `go vet`
asmdecl and exercised by runtime tests — natively on arm64, and under qemu-user
for riscv64.

Not yet correct for structs, arrays, or vector (V-register) values.

## Validate locally (Go toolchain required)

The library packages (`arm64`, `riscv64`, `internal/abi`, `internal/emit`) are
architecture-independent and held to 100% test coverage:

```sh
go test ./arm64/... ./riscv64/... ./internal/...
```

The generated assembly is the real test of correctness. On an arm64 host (Apple
Silicon or arm64 Linux):

```sh
go generate ./examples/add/... ./examples/types/...
GOARCH=arm64 go vet ./examples/add/... ./examples/types/...   # asmdecl
go test ./examples/add/... ./examples/types/...               # runtime
```

riscv64 has no common native host, so validate it statically anywhere
(asmdecl + `cmd/asm`) and run it under emulation:

```sh
go generate ./examples/riscv64/...
GOOS=linux GOARCH=riscv64 go vet ./examples/riscv64/...    # asmdecl
GOOS=linux GOARCH=riscv64 go build ./examples/riscv64/...  # cmd/asm checks mnemonics
# runtime under qemu-user (e.g. in CI), or via Docker's riscv64 emulation:
GOARCH=riscv64 go test -exec=qemu-riscv64-static ./examples/riscv64/...
```

### Validation priorities (in order)

1. **`go vet` asmdecl passes.** This is the cheapest, strongest check: it
   verifies every `name+offset(FP)` in the .s matches the Go declaration. If
   offsets are wrong, vet catches it before runtime.
2. **Differential encoding check.** For each emitted instruction, also hand-write
   the same Plan 9 line, assemble both with `go tool asm -S`, and diff the bytes.
   Wire this into CI as the correctness guarantee.
3. **Runtime test on real arm64** (CI: GitHub Actions `ubuntu-24.04-arm`, or
   qemu-user, or Apple Silicon).

## Roadmap

- v0: arm64, ABI0, 8-byte int/ptr args. (done)
- Widen arm64 scalar support: 1/2/4-byte signed/unsigned ints, pointers, float
  regs (F0.., `FMOVS`/`FMOVD`), word-aligned result area, sub-word sign/zero
  extension. Validated against asmdecl + runtime tests. (done)
- Extract the shared ABI0 model (`internal/abi`) and add **riscv64** as a thin
  second architecture over it, runtime-proven under qemu-user. (done — here)
- Structs, arrays, and vector (V-register) values.
- loong64, reusing the shared layout model.
- Optional: derive instruction mnemonic tables from cmd/internal/obj to catch
  typos at generation time (still delegating encoding to cmd/asm).

## Design notes

- ABI0 not ABIInternal: ABIInternal is an unstable internal contract that can
  change between Go releases. ABI0 (stack-based, FP-relative) is stable and is
  what hand-written .s targets.
- NOSPLIT by default in v0: avoids stack-growth preamble. Revisit for functions
  with large frames or that call other functions.
