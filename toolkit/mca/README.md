# Cycle-accurate kernel analysis (llvm-mca)

`llvm-mca` is the toolkit's **cycle-accurate static analyzer**: feed it a hot loop
+ a target `-mcpu`, and it predicts throughput from that microarchitecture's real
scheduling model (issue ports, latencies, dispatch width). Multi-arch: x86-64,
AArch64, RISC-V. No execution needed — it reads the generated `.s` kernels.

## Why it's in the toolkit

It gives **objective, hardware-independent** throughput predictions to cross-check
our measured benchmarks — and to reason about a kernel *before* we have silicon.

**Methodology validation (matchlen, znver3 = the EPYC 7763 CI runner):**

| | Block RThroughput | bytes/cycle |
|---|---|---|
| SSE2 (16 B/iter) | 1.0 cyc | 16 |
| AVX2 (32 B/iter) | 1.0 cyc | 32 |

→ **AVX2/SSE = 2.00×** predicted, vs **2.08×** measured on CI. The static model and
the live benchmark agree within ~4% — the methodology is sound.

```sh
./mca.sh x86_64-linux-gnu znver3 examples/matchlen-avx2.s
```

## Honest limits

- **Static**: assumes infinite cache, perfect branch prediction, the loop in
  steady state. For memory-hierarchy/branch effects use a full-system simulator
  (**gem5**, cycle-approximate). For tight compute-bound SIMD kernels (what we
  generate), llvm-mca is the right, lightweight tool.
- **New ISA extensions may be unmodeled.** As of LLVM 22, **RISC-V Zvbb is not in
  any core's scheduling model** (`vror.vi` → "unsupported instruction"), so the
  go1.27 BLAKE3 Zvbb rotate cannot be timed here — only the base-RVV emulation is
  modeled. That number still needs **real RVV+Zvbb hardware** (or a cycle-accurate
  RTL/gem5 model that covers Zvbb).
