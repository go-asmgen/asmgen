#!/bin/sh
# Cycle-accurate static throughput of a kernel's inner loop via llvm-mca, using
# the real per-microarchitecture scheduling model (ports + latencies). Static =
# no cache/branch-prediction effects (use gem5 for those); ideal for tight,
# compute-bound SIMD kernels.
#
# Usage:  mca.sh <triple> <mcpu> <loop.s>   # loop.s is the hot loop in LLVM asm
#   amd64 (Zen3/EPYC):  mca.sh x86_64-linux-gnu znver3       examples/matchlen-avx2.s
#   amd64 (Zen4):       mca.sh x86_64-linux-gnu znver4       loop.s
#   arm64 (Neoverse):   mca.sh aarch64-linux-gnu neoverse-v2 loop.s
#   riscv64 (RVV):      mca.sh riscv64 sifive-x280 -mattr=+v loop.s   # base RVV only
# To get the loop in LLVM asm from a Go kernel: build the package, then
# `llvm-objdump -d --no-show-raw-insn <binary>` and copy the inner loop, or
# hand-translate the few hot instructions.
#
# NOTE: bytes/cycle = (bytes processed per loop iteration) / Block-RThroughput.
set -e
L="$(brew --prefix llvm 2>/dev/null)/bin"
[ -x "$L/llvm-mca" ] || L="$(dirname "$(command -v llvm-mca)")"
"$L/llvm-mca" -mtriple="$1" -mcpu="$2" "$3" 2>&1 | grep -E 'Iterations|Total Cycles|Block RThroughput|uOps Per Cycle|error|unsupported'
