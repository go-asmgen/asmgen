// Command popcount builds popcount.wat programmatically via the wasm-emit
// package — the third kernel after matchlen and hex, showcasing the
// wasm-SIMD-only op `i8x16.popcnt` and the pairwise-widening reduction chain
// (extadd_pairwise_i8x16_u → extadd_pairwise_i16x8_u → i32x4.add).
//
// Algorithm:
//
//	acc = zero_v128            // i32x4 accumulator
//	for i in 0..nBlocks:
//	  bytes = v128.load(src + 16*i)
//	  bytePop = i8x16.popcnt(bytes)             // 16 u8 popcounts (0..8)
//	  u16sum  = i16x8.extadd_pairwise_u(bytePop) // 8 u16 lanes (0..16)
//	  u32sum  = i32x4.extadd_pairwise_u(u16sum)  // 4 u32 lanes (0..32)
//	  acc     = i32x4.add(acc, u32sum)           // running accumulator
//	// horizontal sum: sum(acc[0..3])
//	return acc[0] + acc[1] + acc[2] + acc[3]
//
// Per 16 input bytes: 1 load + 1 popcnt + 2 extadd_pairwise + 1 add.
// wasm-SIMD `i8x16.popcnt` has no direct 128-bit analogue in SSE/NEON (SSE has
// popcnt only on GPR64; NEON's `CNT` produces 16 nibble-level counts but wraps
// differently), so this kernel is particularly wasm-native — the go-asmgen
// model shines here because emitting the WAT is trivial while writing an
// equivalent Plan 9 asm kernel is nontrivial per-arch.
//
// Signature:
//
//	(func $popcount (param $srcPtr i32) (param $nBlocks i32) (result i32))
//
// Each block counts 16 input bytes. Caller handles the tail with a scalar
// loop.
//
// Run with:
//
//	go run . > popcount.wat
package main

import (
	"fmt"
	"io"
	"os"

	wasm "github.com/go-asmgen/asmgen/wasm"
)

// runFunc / osExit are dependency-injection seams so tests can drive main()'s
// success and error branches without spawning a subprocess. See main_test.go.
var (
	runFunc = run
	osExit  = os.Exit
)

func main() {
	if err := runFunc(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		osExit(1)
	}
}

func run(out io.Writer) error {
	m := wasm.NewModule()
	m.ImportMemory("env", "memory")

	fn := wasm.NewFunction("popcount", "popcount",
		[]wasm.Param{
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		[]wasm.ValueType{wasm.I32},
	)
	fn.Local("i", wasm.I32)
	fn.Local("acc", wasm.V128)
	fn.Local("bytes", wasm.V128)

	// acc = zero_v128
	fn.V128Zero()
	fn.LocalSet("acc")
	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// bytes = v128.load(srcPtr + i*16)
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.LocalSet("bytes")

	// acc = acc + i32x4.extadd_pairwise( i16x8.extadd_pairwise( i8x16.popcnt(bytes) ) )
	fn.LocalGet("acc")
	fn.LocalGet("bytes")
	fn.I8x16Popcnt()
	fn.I16x8ExtaddPairwiseI8x16U()
	fn.I32x4ExtaddPairwiseI16x8U()
	fn.I32x4Add()
	fn.LocalSet("acc")

	// i += 1
	fn.LocalGet("i")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("i")
	fn.Br(loop)

	fn.End() // loop
	fn.End() // block

	// Horizontal sum: acc[0] + acc[1] + acc[2] + acc[3]
	fn.LocalGet("acc")
	fn.I32x4ExtractLane(0)
	fn.LocalGet("acc")
	fn.I32x4ExtractLane(1)
	fn.I32Add()
	fn.LocalGet("acc")
	fn.I32x4ExtractLane(2)
	fn.I32Add()
	fn.LocalGet("acc")
	fn.I32x4ExtractLane(3)
	fn.I32Add()

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
