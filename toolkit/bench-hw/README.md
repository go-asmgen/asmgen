# bench-hw — real-hardware benchmarks for the go-asmgen ecosystem

Across go-simd (20 repos) and go-compressions (lz4/blake3/lzfse + CLIs), the SIMD
kernels are correctness-validated on every target, but turning their **perf** from
`llvm-mca` *estimates* into *measured* numbers needs real silicon (GitHub Actions
has no POWER/Z/RISC-V/LoongArch hosted runner, and qemu-TCG is not cycle-accurate).
This directory is how we close that gap, and a record of how far it is closed.

## Status (June 2026)

| Arch | Native perf | How |
|---|---|---|
| **amd64**, **arm64** | ✅ measured | GitHub-hosted CI runners |
| **ppc64le** (VSX) | ✅ measured | GCC Compile Farm — POWER10 (cfarm433) |
| **riscv64** (RVV) | ✅ measured | GCC Compile Farm — SpacemiT X60 / RVV 1.0 (cfarm95) |
| **ppc64 (big-endian)** | ✅ build+test validated | GCC Compile Farm — POWER9 (cfarm435/436); generic path, 7th validated arch |
| **s390x** (vector) | ⏳ pending hardware | NOT on cfarm → needs an IBM LinuxONE Community Cloud VM (below) |
| **loong64** (LSX) | ⏳ pending hardware | cfarm400/401 down ~3 years → Debian porter box or self-hosted runner (below) |

The measured numbers **overturned** the estimates (e.g. the matchlen ppc64le
llvm-mca model said ~0.6× scalar; real POWER10 VSX is **6.3×**) — which is exactly
why this directory exists: never publish an extrapolation as a measurement.

## Free OSS hardware access (the project qualifies — FOSS, BSD-3, public)

| Service | Archs | Form | Notes |
|---|---|---|---|
| **GCC Compile Farm** — <https://portal.cfarm.net/> (apply: `/users/new/`) | **ppc64le, riscv64, ppc64 big-endian** (and amd64/arm64) — **no s390x, no working loong64** | one SSH account → all machines | egress-locked (see below); the workhorse for ad-hoc runs |
| **IBM LinuxONE Community Cloud** — <https://community.ibm.com/zsystems/l1cc/> | **s390x** | free *permanent* VM for OSS projects (also 120-day trial VMs for anyone) | **the** s390x path; register it as a self-hosted runner |
| **Open Mainframe Project** — <https://openmainframeproject.org/> | s390x | community s390x dev resources / mentorship | complements L1CC |
| **OSU Open Source Lab** — <https://osuosl.org/services/powerdev/> | ppc64le (POWER8/9/10) | OpenStack + POWER CI for FOSS | persistent ppc64le CI |
| **Debian LoongArch porter box** — `shenzhou.debian.net` (see <https://wiki.debian.org/LoongArch>) | **loong64** | porter box for Debian Developers/maintainers (LoongArch is official from Debian 14) | the most realistic free loong64 silicon today |
| **loong64 org** — <https://github.com/loong64> | loong64 | community toolchains, container images, self-hosted-runner tooling | for standing up a loong64 runner |

So the **two missing arches** have a path each: **s390x → IBM LinuxONE Community
Cloud** (apply for a free permanent OSS VM), **loong64 → a Debian LoongArch porter
box** (needs Debian-maintainer standing) or a self-hosted runner built from the
[loong64](https://github.com/loong64) org's images. cfarm itself has neither.

## 1. Ad-hoc: `bench-hw.sh` (on a cfarm box)

cfarm egress is **locked** — `git clone https://github.com/…` times out — but
`proxy.golang.org` is reachable, so the script fetches everything through the Go
module proxy (`go get -t`), no git clone. It is therefore portable to any host.

```sh
# 1. SSH (cfarm host keys churn → relax checking)
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <user>@cfarm433.cfarm.net   # ppc64le POWER10
#  riscv64: cfarm95 (SpacemiT X60) ·  ppc64 big-endian: cfarm435/436 (POWER9)

# 2. Install a current Go (cfarm's preinstalled Go is usually too old to ASSEMBLE
#    the newer RVV/VSX kernels). go.dev is reachable from cfarm:
A="$(uname -m)"; case "$A" in x86_64) A=amd64;; aarch64) A=arm64;; esac
mkdir -p ~/gosdk && curl -fsSL "https://go.dev/dl/go1.26.4.linux-$A.tar.gz" \
  | tar -C ~/gosdk -xz && export PATH="$HOME/gosdk/go/bin:$PATH"

# 3. Run (pipe the script in; cfarm can't curl raw.githubusercontent either):
bash -s < bench-hw.sh 6 300ms > bench-$(go env GOARCH).md
#   slow in-order core? start with:  bash -s < bench-hw.sh 1 200ms
```

It runs `go test -bench -count=N` for every go-simd + go-compressions library and
prints a markdown report. Fold the relevant `Benchmark`/ratio lines into each
repo's README perf table — **measured GB/s + date + CPU model**, replacing any
"native perf pending" row. Report wins **and** parities **and** losses (the
ecosystem's honesty policy): on the in-order riscv64 core, for instance, int8dot
is 9.1× but base64/hex/utf8 sit at scalar parity — both get reported.

## 2. Continuous: `runner-template.yml` (self-hosted runner)

Register a self-hosted runner on a real box with arch labels, then drop
`runner-template.yml` into a repo's `.github/workflows/`:

* **s390x** ← an IBM LinuxONE Community Cloud VM → `--labels self-hosted,linux,s390x`
* **ppc64le** ← an OSU OSL instance → `--labels self-hosted,linux,ppc64le`
* **loong64** ← a Debian porter box / loong64-org image → `--labels self-hosted,linux,loong64`

```
./config.sh --url https://github.com/<org>/<repo> --token <TOKEN> --labels self-hosted,linux,s390x
./run.sh    # or install as a service
```

It runs the benchmarks on real hardware on demand / weekly and uploads the
results — real-hardware perf in CI alongside the qemu correctness gate.

## Honesty contract

Keep a human in the loop when turning estimates into published numbers. The
llvm-mca figures are model lower bounds and the real silicon can differ by a lot
in **either** direction (matchlen ppc64le: model ~0.6×, reality 6.3×). Replace an
estimate **only** with a *measured* value, dated, with the CPU model named — and
where there is still no hardware (s390x, loong64), keep the row labelled
qemu-correctness-only rather than extrapolating.
