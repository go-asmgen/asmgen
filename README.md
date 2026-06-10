# go-asmgen

Ergonomic generation of Go-compatible Plan 9 assembly for non-amd64
architectures, starting with **arm64**. Encoding is delegated to the Go
toolchain assembler (`cmd/asm`); go-asmgen computes the ABI0 frame layout and
emits well-formed Plan 9 instruction text.

This is the multi-architecture counterpart to what `avo` does for amd64. avo
encodes instruction bytes itself, which is exactly what makes extending it to
new ISAs expensive. go-asmgen instead emits Plan 9 text and lets `cmd/asm`
encode, so each new architecture only needs an ABI layout model plus a thin
instruction-emit surface.

## Status

v0 — arm64, **ABI0**. Correct for sequences of arm64 **scalars** in any
combination: signed/unsigned integers of 1/2/4/8 bytes, pointers, and 32/64-bit
floats. The builder selects the right move per type (`MOVB`/`MOVBU`, `MOVH`/`MOVHU`,
`MOVW`/`MOVWU`, `MOVD`, `FMOVS`/`FMOVD`) and computes ABI0 offsets (results
word-aligned, sub-word loads sign/zero-extended). Every emitted offset and access
width is cross-checked by `go vet` asmdecl and exercised by runtime tests on
native arm64.

Not yet correct for structs, arrays, or vector (V-register) values.

## Validate locally (Go toolchain required)

These steps are the real test of correctness; run them on an arm64 host (Apple
Silicon, arm64 Linux, or qemu/Rosetta).

```sh
go generate ./examples/...        # runs each gen.go -> writes *_arm64.s
GOARCH=arm64 go vet ./examples/... # asmdecl cross-checks .s offsets vs decls
GOARCH=arm64 go build ./examples/...
go test ./examples/...            # runtime correctness on arm64
```

The library packages (`arm64`, `internal/emit`) are architecture-independent and
held to 100% test coverage:

```sh
go test ./arm64/... ./internal/...
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
  extension. Validated against asmdecl + runtime tests. (done — here)
- Structs, arrays, and vector (V-register) values.
- riscv64 (start RV64GC), reusing the emit layer.
- loong64.
- Optional: derive instruction mnemonic tables from cmd/internal/obj to catch
  typos at generation time (still delegating encoding to cmd/asm).

## Design notes

- ABI0 not ABIInternal: ABIInternal is an unstable internal contract that can
  change between Go releases. ABI0 (stack-based, FP-relative) is stable and is
  what hand-written .s targets.
- NOSPLIT by default in v0: avoids stack-growth preamble. Revisit for functions
  with large frames or that call other functions.
