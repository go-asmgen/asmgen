#!/usr/bin/env bash
# bench-hw.sh — run the go-simd + go-compressions SIMD benchmarks on a REAL
# machine via the Go module proxy, and emit a markdown report to fold into each
# repo's README perf table (replacing any "native perf pending" placeholder).
#
# WHY the module proxy and not `git clone`: the GCC Compile Farm — the practical
# source of free POWER / RISC-V silicon — has LOCKED egress. github.com times
# out from a cfarm box, but proxy.golang.org is reachable. So we fetch every
# module through the proxy (`go get`), which also makes this script portable to
# any host. `go get -t` is REQUIRED, not optional: the benchmarks compare against
# competitor modules (pierrec, mhr3, barakmich, emmansun, tmthrgd, zeebo, …) that
# are TEST dependencies — without -t they fail with "missing go.sum entry
# [setup failed]" and you silently lose every ours-vs-competitor row.
#
# Usage:  ./bench-hw.sh [count] [benchtime]
#   count defaults to 6, benchtime to 300ms. On a slow in-order core (e.g. the
#   SpacemiT X60 riscv64 board, cfarm95) start with `./bench-hw.sh 1 200ms`.
#
# Go toolchain: needs >= the repos' go directive (1.26+). cfarm home dirs often
# ship an OLD Go (cfarm95 had go1.23.5 — too old to ASSEMBLE the newer RVV/VSX
# kernels, so the SIMD path would not even build). go.dev IS reachable from
# cfarm, so install a current Go first, e.g.:
#   A="$(uname -m)"; case "$A" in x86_64) A=amd64;; aarch64) A=arm64;; esac
#   mkdir -p ~/gosdk && curl -fsSL "https://go.dev/dl/go1.26.4.linux-$A.tar.gz" \
#     | tar -C ~/gosdk -xz && export PATH="$HOME/gosdk/go/bin:$PATH"
#
# This complements ci.yml (which proves CORRECTNESS under qemu). It adds the
# real-hardware PERFORMANCE half: measured numbers, never extrapolated.

set -uo pipefail
COUNT="${1:-6}"
BENCHTIME="${2:-300ms}"
ARCH="$(go env GOARCH 2>/dev/null || echo unknown)"
MACH="$(uname -m)"
GOVER="$(go version 2>/dev/null | awk '{print $3}')"
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT
cd "$WORK" || exit 1
go mod init bench >/dev/null 2>&1
export GOFLAGS=-mod=mod GOWORK=off GOTOOLCHAIN=local

# go-simd libraries that ship benchmarks (all 20 repos; histogram/levenshtein are
# honest non-vector cases but still benchmark vs an oracle/competitor).
SIMD="matchlen base64 base32 hex utf8 bitpack popcount adler32 strconv crc64 \
ascii85 streamvbyte floats xxhash ascii bitset int8dot levenshtein jsonvalidate histogram"
# go-compressions LIBRARIES with benchmarks (matchlen lives in go-simd; the CLIs
# b3sum/lz4c/lzfsec have no benchmarks — validated at the library level).
COMP="lz4 blake3 lzfse"

echo "# Native SIMD benchmarks — ${MACH} (GOARCH=${ARCH})"
echo
echo "_Real-hardware measurement via the Go module proxy. Go: ${GOVER:-unknown}._"
echo "_count=${COUNT}, benchtime=${BENCHTIME}. Fold the relevant Benchmark lines"
echo "(and the ours-vs-competitor ratio) into each repo's README, dated, noting"
echo "the CPU model. Replace estimates only with MEASURED values._"
echo

bench() { # $1 = module import path, $2 = display label
  local path="$1" label="$2"
  echo "### ${label}"
  if ! go get -t "${path}@latest" >/dev/null 2>&1; then
    echo '```'; echo "(go get -t failed — check egress / proxy reachability; skipped)"; echo '```'; echo
    return
  fi
  echo '```'
  go test -run '^$' -bench=. -benchmem -count="$COUNT" -benchtime="$BENCHTIME" "$path" 2>&1 \
    | grep -E '^(Benchmark|PASS|FAIL|ok|---|panic)' || echo "(no benchmarks / build failed)"
  echo '```'
  echo
}

for r in $SIMD; do bench "github.com/go-simd/$r"          "go-simd/$r"; done
for r in $COMP; do bench "github.com/go-compressions/$r"  "go-compressions/$r"; done

echo "_s390x: NOT on the GCC Compile Farm — use an IBM LinuxONE Community Cloud"
echo "VM (see README). loongarch64: cfarm400/401 have been down ~3 years; the"
echo "free options are a Debian LoongArch porter box or a self-hosted runner —"
echo "until one exists, loong64 rows stay qemu-correctness-only, not extrapolated._"
