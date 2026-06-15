# bench-hw — real POWER/Z benchmarks for the go-asmgen ecosystem

Across go-simd (20 repos) and go-compressions (5 repos), the **ppc64le (VSX)** and
**s390x (vector facility)** kernels are correctness-validated under qemu but their
perf numbers are **llvm-mca static estimates** — GitHub Actions has no POWER/Z
hosted runner, and qemu-TCG is not cycle-accurate. This directory closes that gap
once you have access to real hardware.

## Free OSS hardware access (the project qualifies — FOSS, BSD-3, public)

| Service | Archs | Form | Use |
|---|---|---|---|
| **GCC Compile Farm** — <https://portal.cfarm.net/> (apply: `/users/new/`) | **ppc64le + s390x** (and ppc64 **big-endian** on cfarm435/436) | one SSH account → all machines | **ad-hoc benchmarking now** |
| **IBM LinuxONE Community Cloud** — <https://community.ibm.com/zsystems/l1cc/> | s390x | permanent free VM (2c/8G/100G) for OSS | **persistent s390x CI runner** |
| **OSU Open Source Lab** — <https://osuosl.org/services/powerdev/> | ppc64le (POWER8/9/10) | OpenStack + POWER CI for FOSS | **persistent ppc64le CI runner** |

Recommended: cfarm for immediate both-arch numbers; LinuxONE CC + OSU OSL as
self-hosted GitHub Actions runners for continuous native-perf CI.

## 1. Ad-hoc: `bench-hw.sh` (on a cfarm box)

```sh
ssh user@cfarm433.cfarm.net      # ppc64le, little-endian (also cfarm29/434)
#  or: ssh user@cfarmNN.cfarm.net for an s390x machine
# install a recent Go (>= the repos' go directive, 1.25+), then:
curl -fsSL https://raw.githubusercontent.com/go-asmgen/asmgen/main/toolkit/bench-hw/bench-hw.sh | bash > bench-ppc64le.md
```

It clones every go-simd + go-compressions repo, runs `go test -bench -count=6` on
the box's native arch, and prints a markdown report. Paste the relevant
`Benchmark`/ratio lines into each repo's README perf table, flipping its
ppc64le/s390x row from *"qemu-validated; native perf pending"* to the measured
GB/s + the date.

## 2. Continuous: `runner-template.yml` (self-hosted runner)

Register a self-hosted runner on a LinuxONE-CC (s390x) and/or OSU-OSL (ppc64le)
box with arch labels, then drop `runner-template.yml` into a repo's
`.github/workflows/`. It runs the benchmarks on real POWER/Z on demand / weekly
and uploads the results — real-hardware perf in CI alongside the qemu correctness
gate. (See the file header for the `config.sh --labels` registration command.)

## Honesty contract

Keep a human in the loop when turning estimates into published numbers: the
llvm-mca figures are conservative lower bounds; the real silicon may differ (e.g.
the matchlen ppc64le model said ~0.6× scalar — confirm before acting on it).
Replace estimates only with *measured* values, dated, and note the CPU model.
