// Command base64_encode builds base64_encode.wat programmatically via the
// wasm-emit package — the eleventh kernel and the first to bring
// Lemire-style bit-manipulation to wasm-SIMD.
//
// Per iteration: 12 input bytes → 16 output ASCII chars (StdEncoding).
// The 4:3 expansion ratio matches base64 exactly; 4-byte "orphan" tail
// (nBytes % 12 != 0) is the caller's problem (they either pad to a
// 12-byte boundary or handle the last group in scalar Go).
//
// Algorithm (adapted from Lemire's SSE implementation, published in
// "Faster Base64 Encoding and Decoding using AVX2 Instructions",
// arXiv:1704.00605):
//
//  1. Load 16 input bytes (12 real + 4 tail, harmlessly masked out).
//  2. Shuffle bytes so each 3-byte input group maps to two u16 lanes
//     (b0<<8|b1) and (b1<<8|b2) — this arranges the 24 bits of each
//     triplet as two adjacent big-endian words with 4 bits of overlap.
//  3. Extract the four 6-bit indexes per triplet with two rounds:
//     Round 1 (PMULHUW-style): mask 0x0fc0_fc00 then multiply by
//     0x0400_0040 and take the high 16 of the u32 product. Wasm-SIMD
//     lacks PMULHUW, so this is emulated with i32x4.extmul_low/high
//     + i32x4.shr_u by 16 + i16x8.narrow_i32x4_u.
//     Round 2 (PMULLW-style): mask 0x003f_03f0 then multiply by
//     0x0100_0010 via i16x8.mul (which IS PMULLW).
//  4. OR the two rounds — the final v128 now holds all 16 6-bit
//     indexes (0..63) sequentially in its 16 byte lanes.
//  5. Convert each 6-bit index to an ASCII char using the range-add
//     pattern:
//     result = idx + 65
//     + (idx > 25 ? 6   : 0)   ; 'a'..'z' offset
//     + (idx > 51 ? -75 : 0)   ; '0'..'9' offset
//     + (idx == 62 ? -15 : 0)  ; '+' offset
//     + (idx == 63 ? -12 : 0)  ; '/' offset
//     Each conditional is a mask (i8x16.gt_u / i8x16.eq) AND-ed with
//     the offset broadcast, then summed in via i8x16.add. Negative
//     offsets go modular (e.g. -75 becomes 181).
//  6. v128.store the 16 ASCII chars.
//
// Signature:
//
//	(func $base64_encode (param $dstPtr i32) (param $srcPtr i32)
//	                     (param $nBlocks i32))
//
// nBlocks blocks × 12 input bytes → nBlocks × 16 output chars. Caller
// ensures srcPtr has (nBlocks * 12 + 4) bytes readable — the last
// v128.load overreads 4 bytes past the useful end of the last block,
// which are shuffled into positions the extraction ignores. Sub-12-byte
// tail is caller's problem.
//
// Run with:
//
//	go run . > base64_encode.wat
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

// repU32 replicates a 4-byte pattern 4 times to produce a 16-byte v128
// constant. The Lemire masks and multipliers are u32-per-lane, so we
// broadcast the same pattern to all four u32 lanes of the v128.
func repU32(u32le [4]byte) []byte {
	out := make([]byte, 16)
	for i := 0; i < 4; i++ {
		copy(out[i*4:], u32le[:])
	}
	return out
}

// broadcast returns 16 copies of a single byte.
func broadcast(b byte) []byte {
	v := make([]byte, 16)
	for i := range v {
		v[i] = b
	}
	return v
}

func run(out io.Writer) error {
	m := wasm.NewModule()
	m.ImportMemory("env", "memory")

	// Byte-shuffle pattern: for each 3-byte input group producing 4
	// output chars, arrange bytes so u16 lane 2g holds (b0<<8)|b1 and
	// u16 lane 2g+1 holds (b1<<8)|b2. In little-endian byte order this
	// means output bytes 4g..4g+3 = [b1, b0, b2, b1] repeated per group.
	shuf := []byte{
		1, 0, 2, 1,
		4, 3, 5, 4,
		7, 6, 8, 7,
		10, 9, 11, 10,
	}
	// Lemire's u32-per-lane constants (little-endian byte order).
	mask1 := repU32([4]byte{0x00, 0xfc, 0xc0, 0x0f}) // 0x0fc0_fc00
	mulhi := repU32([4]byte{0x40, 0x00, 0x00, 0x04}) // 0x0400_0040
	mask2 := repU32([4]byte{0xf0, 0x03, 0x3f, 0x00}) // 0x003f_03f0
	mullo := repU32([4]byte{0x10, 0x00, 0x00, 0x01}) // 0x0100_0010

	// Range-add constants for 6-bit-index → ASCII conversion.
	c51 := broadcast(51) // gt_u threshold for '0'-'9' branch
	c25 := broadcast(25) // gt_u threshold for 'a'-'z' branch
	c62 := broadcast(62) // eq for '+'
	c63 := broadcast(63) // eq for '/'
	// Additive corrections layered on the base (idx + 65):
	//   idx > 25: +6      → drops 'A'..'Z' bump to 'a'..'z' offset
	//   idx > 51: -75     → drops range to '0'..'9' (181 = -75 mod 256)
	//   idx == 62: -15    → 62+65+6-75-15 = 43 = '+'  (241 = -15 mod 256)
	//   idx == 63: -12    → 63+65+6-75-12 = 47 = '/'  (244 = -12 mod 256)
	add65 := broadcast(65)
	add06 := broadcast(6)
	sub75 := broadcast(181) // -75 mod 256
	sub15 := broadcast(241) // -15 mod 256
	sub12 := broadcast(244) // -12 mod 256

	fn := wasm.NewFunction("base64_encode", "base64_encode",
		[]wasm.Param{
			{Name: "dstPtr", Type: wasm.I32},
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		nil,
	)
	fn.Local("i", wasm.I32)
	fn.Local("bytes", wasm.V128)
	fn.Local("triplets", wasm.V128) // shuffled: each 4-byte group = [b1, b0, b2, b1]
	fn.Local("hi", wasm.V128)       // Round-1 PMULHUW-style result
	fn.Local("lo", wasm.V128)       // Round-2 PMULLW-style result
	fn.Local("idx", wasm.V128)      // 16 x u6 indexes (0..63) per byte lane

	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// bytes = v128.load(srcPtr + i*12)
	// Note: 12-byte stride per block (not 16); v128.load reads 16 but
	// only bytes 0..11 are shuffled into extracted positions.
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(12)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.LocalSet("bytes")

	// triplets = i8x16.shuffle(bytes, bytes, shuf)
	fn.LocalGet("bytes")
	fn.LocalGet("bytes")
	fn.I8x16Shuffle(shuf)
	fn.LocalSet("triplets")

	// Round 1: hi = pmulhuw_emulated(triplets & mask1, mulhi)
	//   masked = triplets & mask1
	//   lo32 = i32x4.extmul_low_i16x8_u(masked, mulhi)
	//   hi32 = i32x4.extmul_high_i16x8_u(masked, mulhi)
	//   lo32 >>= 16
	//   hi32 >>= 16
	//   hi = i16x8.narrow_i32x4_u(lo32, hi32)
	fn.LocalGet("triplets")
	fn.V128Const16(mask1)
	fn.V128And()
	// stack top = (triplets & mask1)
	// Duplicate via LocalTee into a scratch — no scratch v128 local
	// slot free; use the "hi" slot as scratch too.
	fn.LocalTee("hi")
	fn.V128Const16(mulhi)
	fn.I32x4ExtmulLowI16x8U()
	fn.I32Const(16)
	fn.I32x4ShrU()
	fn.LocalGet("hi")
	fn.V128Const16(mulhi)
	fn.I32x4ExtmulHighI16x8U()
	fn.I32Const(16)
	fn.I32x4ShrU()
	fn.I16x8NarrowI32x4U()
	fn.LocalSet("hi")

	// Round 2: lo = pmullw(triplets & mask2, mullo)  (which is i16x8.mul)
	fn.LocalGet("triplets")
	fn.V128Const16(mask2)
	fn.V128And()
	fn.V128Const16(mullo)
	fn.I16x8Mul()
	fn.LocalSet("lo")

	// idx = hi | lo   (per-byte layout: 4 sequential 6-bit indexes per group)
	fn.LocalGet("hi")
	fn.LocalGet("lo")
	fn.V128Or()
	fn.LocalSet("idx")

	// Range-add ASCII conversion:
	//   result = idx + 65
	//          + (idx > 25 ? 6 : 0)
	//          + (idx > 51 ? -75 : 0)   ← 181 mod 256
	//          + (idx == 62 ? -15 : 0)  ← 241 mod 256
	//          + (idx == 63 ? -16 : 0)  ← 240 mod 256
	// Build the output address first so v128.store can pop it below the
	// packed result at the end.
	fn.LocalGet("dstPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()

	// Start with idx + 65.
	fn.LocalGet("idx")
	fn.V128Const16(add65)
	fn.I8x16Add()

	// + (gt_u(idx, 25) & 6)
	fn.LocalGet("idx")
	fn.V128Const16(c25)
	fn.I8x16GtU()
	fn.V128Const16(add06)
	fn.V128And()
	fn.I8x16Add()

	// + (gt_u(idx, 51) & 181)
	fn.LocalGet("idx")
	fn.V128Const16(c51)
	fn.I8x16GtU()
	fn.V128Const16(sub75)
	fn.V128And()
	fn.I8x16Add()

	// + (eq(idx, 62) & 241)
	fn.LocalGet("idx")
	fn.V128Const16(c62)
	fn.I8x16Eq()
	fn.V128Const16(sub15)
	fn.V128And()
	fn.I8x16Add()

	// + (eq(idx, 63) & 244)
	fn.LocalGet("idx")
	fn.V128Const16(c63)
	fn.I8x16Eq()
	fn.V128Const16(sub12)
	fn.V128And()
	fn.I8x16Add()

	// v128.store the 16 ASCII chars.
	fn.V128Store(0)

	// i += 1
	fn.LocalGet("i")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("i")
	fn.Br(loop)

	fn.End() // loop
	fn.End() // done block

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
