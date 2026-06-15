#!/usr/bin/env bash
# bench-hw.sh — run the go-simd + go-compressions SIMD benchmarks on a REAL
# machine and emit a markdown table to paste into each repo's README, replacing
# the "qemu-validated; native perf pending" placeholders for ppc64le / s390x.
#
# The whole ecosystem's POWER/Z numbers are currently llvm-mca *estimates* (no
# GitHub-hosted POWER/Z runner; qemu-TCG is not cycle-accurate). Run this on real
# hardware — see ./README.md for free OSS access (GCC Compile Farm gives both
# ppc64le AND s390x over one SSH account; IBM LinuxONE Community Cloud = s390x;
# OSU Open Source Lab = ppc64le).
#
# Usage:  ./bench-hw.sh [count]          # count defaults to 6
#   On a cfarm ppc64le box  -> ppc64le numbers; on an s390x box -> s390x numbers;
#   also works on amd64/arm64/riscv64/loong64 to refresh those rows.
#
# Requires: git, a Go toolchain (>= the newest go directive in the repos, 1.25+).

set -uo pipefail
COUNT="${1:-6}"
ARCH="$(go env GOARCH 2>/dev/null || echo unknown)"
MACH="$(uname -m)"
DATE="$(date -u +%Y-%m-%d)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

SIMD_REPOS="matchlen base64 base32 hex utf8 bitpack popcount adler32 strconv \
crc64 ascii85 streamvbyte floats xxhash ascii bitset int8dot levenshtein jsonvalidate"
COMP_REPOS="lz4 blake3 b3sum lzfse lzfsec"

echo "# Native SIMD benchmarks — ${MACH} (GOARCH=${ARCH}), ${DATE}"
echo
echo "_Real-hardware measurement (replaces the llvm-mca estimate for this arch)._"
echo "_Go: $(go version 2>/dev/null | awk '{print $3}')._"
echo

run_one() { # $1 = org, $2 = repo
  local org="$1" repo="$2" dir="$WORK/$repo"
  git clone -q --depth 1 "https://github.com/${org}/${repo}" "$dir" 2>/dev/null || {
    echo "### ${org}/${repo} — clone failed (skipped)"; echo; return; }
  echo "### ${org}/${repo}"
  echo '```'
  ( cd "$dir" && GOFLAGS=-mod=mod GOWORK=off \
      go test -run '^$' -bench=. -benchmem -count="$COUNT" ./... 2>&1 \
      | grep -E '^(Benchmark|PASS|FAIL|ok|---)' ) || echo "(no benchmarks / build failed)"
  echo '```'
  echo
}

for r in $SIMD_REPOS;  do run_one go-simd "$r"; done
for r in $COMP_REPOS;  do run_one go-compressions "$r"; done

echo "_Paste the relevant Benchmark lines (and the ours-vs-competitor ratio) into"
echo "each repo's README perf table; flip its ppc64le/s390x row from"
echo "\"qemu-validated; native perf pending\" to the measured GB/s + this date._"
