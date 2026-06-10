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

v0 — proof of pipeline. Correct only for sequences of 8-byte
(int64/uint64/pointer) arguments and results under **ABI0**. Not yet correct for
mixed-size args, structs, floats, or vectors.

## Validate locally (network + Go toolchain required)

The container these files were authored in has no Go toolchain, so the steps
below must be run on your machine. They are the real test of correctness.

```sh
cd examples/add
go generate ./...        # runs gen.go -> writes add_arm64.s
GOARCH=arm64 go vet ./... # asmdecl cross-checks .s offsets vs add.go decl
GOARCH=arm64 go build ./...
go test ./...            # run on an arm64 host or under qemu/Rosetta
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

- v0: arm64, ABI0, 8-byte int/ptr args. (here)
- Widen arm64 type support: 1/2/4-byte ints, float regs (F0..), proper
  alignment + padding rules. Test each against asmdecl.
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
