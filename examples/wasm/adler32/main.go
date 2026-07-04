// Command adler32 builds adler32.wat programmatically via the wasm-emit
// package — the tenth kernel and the first with real i16x8 arithmetic
// content (widening multiply for the weighted-byte-sum step).
//
// Adler-32 is a rolling checksum: two running 32-bit sums (a, b) with
// a starting at 1, b at 0, updated per input byte:
//
//	a = (a + byte) mod 65521
//	b = (b + a)   mod 65521
//
// Final checksum is (b << 16) | a. For a 16-byte SIMD chunk the update
// telescopes to:
//
//	delta_a = sum(bytes)
//	delta_b = 16*a_prev + sum(bytes[k] * (16-k)) for k in 0..15
//
// where the weighted sum applies coefficients [16, 15, 14, ..., 1] to
// the 16 input bytes. The kernel keeps `a` and `b` as scalar i32s across
// iterations, reads them via caller-provided pointers, updates them per
// chunk, and writes them back at exit — no modulo inside the loop (the
// caller batches 5552-byte segments and reduces between calls, per the
// standard NMAX-driven pattern).
//
// Algorithm (per 16-byte block):
//
//	bytes    = v128.load(src + 16*i)
//	// delta_a = sum(bytes) via the popcount-style two-stage widening reduction
//	pair16   = i16x8.extadd_pairwise_i8x16_u(bytes)     ; 8 x u16
//	pair32   = i32x4.extadd_pairwise_i16x8_u(pair16)     ; 4 x u32
//	delta_a  = horizontal_sum(pair32)                    ; scalar
//	// weighted = sum(bytes[k] * (16-k))
//	prod_lo  = i16x8.extmul_low_i8x16_u(bytes, coef)     ; 8 x u16
//	prod_hi  = i16x8.extmul_high_i8x16_u(bytes, coef)    ; 8 x u16
//	wpair    = i32x4.add(
//	             i32x4.extadd_pairwise_i16x8_u(prod_lo),
//	             i32x4.extadd_pairwise_i16x8_u(prod_hi))  ; 4 x u32
//	weighted = horizontal_sum(wpair)                     ; scalar
//	// scalar update
//	b += 16*a + weighted
//	a += delta_a
//
// The widening extmul keeps the byte×byte products in u16 lanes exactly
// (255 × 16 = 4080 < 65536), so no intermediate overflow.
//
// Signature:
//
//	(func $adler32 (param $srcPtr i32) (param $nBlocks i32)
//	               (param $aInit i32) (param $bInit i32)
//	               (param $aOut i32) (param $bOut i32))
//
// $aOut / $bOut are addresses where the kernel writes the final a and b
// as i32. The caller does modulo-65521 reduction outside (typically once
// per NMAX=5552 bytes = 347 chunks of 16) and handles the sub-16-byte
// tail with a scalar loop.
//
// Run with:
//
//	go run . > adler32.wat
package main

import (
	"fmt"
	"io"
	"os"

	wasm "github.com/go-asmgen/asmgen/wasm"
)

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

	// Coefficient vector for the weighted-sum step: byte k gets weight 16-k.
	coef := []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	fn := wasm.NewFunction("adler32", "adler32",
		[]wasm.Param{
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
			{Name: "aInit", Type: wasm.I32},
			{Name: "bInit", Type: wasm.I32},
			{Name: "aOut", Type: wasm.I32},
			{Name: "bOut", Type: wasm.I32},
		},
		nil,
	)
	fn.Local("i", wasm.I32)
	fn.Local("a", wasm.I32)
	fn.Local("b", wasm.I32)
	fn.Local("bytes", wasm.V128)
	fn.Local("wpair", wasm.V128)
	fn.Local("deltaA", wasm.I32)
	fn.Local("weighted", wasm.I32)

	// a = aInit; b = bInit; i = 0
	fn.LocalGet("aInit")
	fn.LocalSet("a")
	fn.LocalGet("bInit")
	fn.LocalSet("b")
	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// bytes = v128.load(srcPtr + 16*i)
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.LocalSet("bytes")

	// deltaA = horizontal_sum( i32x4.extadd_pairwise_i16x8_u(
	//             i16x8.extadd_pairwise_i8x16_u(bytes) ) )
	// pair32 is a v128 of 4 u32 lanes; sum via 4 extract_lane + 3 add.
	fn.LocalGet("bytes")
	fn.I16x8ExtaddPairwiseI8x16U()
	fn.I32x4ExtaddPairwiseI16x8U()
	// Materialize pair32 as a v128 local so we can extract 4 lanes.
	// Actually we can consume the top-of-stack v128 4 times via a
	// scratch local. Use `bytes` slot? No, it's still a live named
	// value — introduce a local for the pair sum.
	fn.LocalSet("wpair") // reuse wpair as temp before we compute the real one
	fn.LocalGet("wpair")
	fn.I32x4ExtractLane(0)
	fn.LocalGet("wpair")
	fn.I32x4ExtractLane(1)
	fn.I32Add()
	fn.LocalGet("wpair")
	fn.I32x4ExtractLane(2)
	fn.I32Add()
	fn.LocalGet("wpair")
	fn.I32x4ExtractLane(3)
	fn.I32Add()
	fn.LocalSet("deltaA")

	// wpair = i32x4.add( extadd_pairwise(extmul_low(bytes, coef)),
	//                    extadd_pairwise(extmul_high(bytes, coef)) )
	fn.LocalGet("bytes")
	fn.V128Const16(coef)
	fn.I16x8ExtmulLowI8x16U()
	fn.I32x4ExtaddPairwiseI16x8U()
	fn.LocalGet("bytes")
	fn.V128Const16(coef)
	fn.I16x8ExtmulHighI8x16U()
	fn.I32x4ExtaddPairwiseI16x8U()
	fn.I32x4Add()
	fn.LocalSet("wpair")

	// weighted = horizontal_sum(wpair)
	fn.LocalGet("wpair")
	fn.I32x4ExtractLane(0)
	fn.LocalGet("wpair")
	fn.I32x4ExtractLane(1)
	fn.I32Add()
	fn.LocalGet("wpair")
	fn.I32x4ExtractLane(2)
	fn.I32Add()
	fn.LocalGet("wpair")
	fn.I32x4ExtractLane(3)
	fn.I32Add()
	fn.LocalSet("weighted")

	// b = b + 16*a + weighted
	fn.LocalGet("b")
	fn.LocalGet("a")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.LocalGet("weighted")
	fn.I32Add()
	fn.LocalSet("b")

	// a = a + deltaA
	fn.LocalGet("a")
	fn.LocalGet("deltaA")
	fn.I32Add()
	fn.LocalSet("a")

	// i += 1
	fn.LocalGet("i")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("i")
	fn.Br(loop)

	fn.End() // loop
	fn.End() // done block

	// Write a and b back to caller-provided pointers.
	fn.LocalGet("aOut")
	fn.LocalGet("a")
	fn.I32Store(0)
	fn.LocalGet("bOut")
	fn.LocalGet("b")
	fn.I32Store(0)

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
