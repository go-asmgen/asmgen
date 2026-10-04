// Command base64_decode builds base64_decode.wat programmatically via the
// wasm-emit package — the thirteenth kernel and the companion of
// base64_encode. Per iteration: 16 base64 chars → 12 output bytes,
// matching StdEncoding decode of encoding/base64.
//
// Algorithm (a direct byte-level inverse of the encoder's Lemire dance):
//
//  1. Load 16 input chars.
//  2. Convert each ASCII char to its 6-bit index via range-mask + sub:
//     digit_mask  = ge_s(chars, '0') & le_s(chars, '9')
//     upper_mask  = ge_s(chars, 'A') & le_s(chars, 'Z')
//     lower_mask  = ge_s(chars, 'a') & le_s(chars, 'z')
//     plus_mask   = eq(chars, '+')
//     slash_mask  = eq(chars, '/')
//     offset = (digit  & 252)   ; -4 mod 256 (chars 48..57 → 52..61)
//     | (upper  &  65)   ; A..Z → 0..25
//     | (lower  &  71)   ; a..z → 26..51
//     | (plus   & 237)   ; -19 mod 256 (chars 43 → 62)
//     | (slash  & 240)   ; -16 mod 256 (chars 47 → 63)
//     idx = i8x16.sub(chars, offset)
//     Invalid input chars end up as garbage indexes (no mask fires),
//     so the caller is responsible for pre-validating input if needed.
//  3. Pack four consecutive 6-bit indexes back into 3 output bytes:
//     b0 = (i0 << 2) | (i1 >> 4)
//     b1 = (i1 << 4) | (i2 >> 2)
//     b2 = (i2 << 6) | i3
//     SIMD-lane implementation:
//     Precompute three shifted-left versions of idx (shl2, shl4, shl6)
//     and two shifted-right versions (shr4, shr2). Then build the
//     "left" and "right" halves with two shuffle merges each — the
//     shuffles interleave sources by output position:
//     left  pattern picks shl2[4g],  shl4[4g+1], shl6[4g+2]
//     right pattern picks shr4[4g+1], shr2[4g+2], idx [4g+3]
//     (idx serves as the "shr0" source since i8x16.shr_u by 0 is a
//     no-op.) The 12 output positions for g = 0..3 fall at byte
//     indices 0-2, 3-5, 6-8, 9-11; positions 12-15 hold junk that
//     the caller overwrites on the next store or discards.
//  4. result = left | right; v128.store.
//
// Signature:
//
//	(func $base64_decode (param $dstPtr i32) (param $srcPtr i32)
//	                     (param $nBlocks i32))
//
// Each block consumes 16 input chars and writes 16 output bytes (of
// which 12 are useful; positions 12-15 are junk). Caller advances
// dstPtr by 12 per block and reserves nBlocks*12 + 4 bytes of output
// space so the last block's junk store lands in-bounds. Sub-16-char
// input, padding characters, and invalid input are all the caller's
// problem — a validating decoder wraps this kernel + a scalar tail.
//
// Run with:
//
//	go run . > base64_decode.wat
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

	// Range endpoints and per-range offsets (as byte constants).
	c0 := broadcast('0')
	c9 := broadcast('9')
	cA := broadcast('A')
	cZ := broadcast('Z')
	ca := broadcast('a')
	cz := broadcast('z')
	cPlus := broadcast('+')
	cSlash := broadcast('/')
	off252 := broadcast(252) // digit  → -4  mod 256
	off65 := broadcast(65)   // upper  → 65
	off71 := broadcast(71)   // lower  → 71
	off237 := broadcast(237) // plus   → -19 mod 256
	off240 := broadcast(240) // slash  → -16 mod 256

	// left  merge patterns — assemble  shl2/shl4/shl6 per output byte.
	// First merge picks shl2 vs shl4 (positions 0/3/6/9 vs 1/4/7/10);
	// slot 2/5/8/11 hold don't-cares (overwritten by the second merge).
	patLeft23 := []byte{0, 17, 0, 4, 21, 0, 8, 25, 0, 12, 29, 0, 0, 0, 0, 0}
	// Second merge picks left_23 vs shl6 (positions 2/5/8/11).
	patLeft6 := []byte{0, 1, 18, 3, 4, 22, 6, 7, 26, 9, 10, 30, 0, 0, 0, 0}
	// right merge patterns — same shape but from shr4/shr2/idx.
	patRight24 := []byte{1, 18, 0, 5, 22, 0, 9, 26, 0, 13, 30, 0, 0, 0, 0, 0}
	patRight0 := []byte{0, 1, 19, 3, 4, 23, 6, 7, 27, 9, 10, 31, 0, 0, 0, 0}

	fn := wasm.NewFunction("base64_decode", "base64_decode",
		[]wasm.Param{
			{Name: "dstPtr", Type: wasm.I32},
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		nil,
	)
	fn.Local("i", wasm.I32)
	fn.Local("chars", wasm.V128)
	fn.Local("idx", wasm.V128)
	fn.Local("shl2", wasm.V128)
	fn.Local("shl4", wasm.V128)
	fn.Local("shl6", wasm.V128)
	fn.Local("shr4", wasm.V128)
	fn.Local("shr2", wasm.V128)
	fn.Local("left23", wasm.V128)
	fn.Local("right24", wasm.V128)

	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// chars = v128.load(srcPtr + i*16)
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.LocalSet("chars")

	// idx = chars - offset  where offset is per-range as documented above.
	// Push offset onto the stack, then chars, then i8x16.sub yields
	// (chars - offset).
	// Actually i8x16.sub semantics: (a, b) → a - b where b is on top.
	// So push chars first, then offset, then sub → chars - offset.
	fn.LocalGet("chars")

	// Build offset (fresh v128 each iteration; short but many ops):
	//   digit_mask = ge_s(chars, '0') & le_s(chars, '9'); offset |= digit & 252
	fn.LocalGet("chars")
	fn.V128Const16(c0)
	fn.I8x16GeS()
	fn.LocalGet("chars")
	fn.V128Const16(c9)
	fn.I8x16LeS()
	fn.V128And()
	fn.V128Const16(off252)
	fn.V128And()

	//   upper_mask = ge_s(chars, 'A') & le_s(chars, 'Z'); offset |= upper & 65
	fn.LocalGet("chars")
	fn.V128Const16(cA)
	fn.I8x16GeS()
	fn.LocalGet("chars")
	fn.V128Const16(cZ)
	fn.I8x16LeS()
	fn.V128And()
	fn.V128Const16(off65)
	fn.V128And()
	fn.V128Or()

	//   lower_mask = ge_s(chars, 'a') & le_s(chars, 'z'); offset |= lower & 71
	fn.LocalGet("chars")
	fn.V128Const16(ca)
	fn.I8x16GeS()
	fn.LocalGet("chars")
	fn.V128Const16(cz)
	fn.I8x16LeS()
	fn.V128And()
	fn.V128Const16(off71)
	fn.V128And()
	fn.V128Or()

	//   plus_mask  = eq(chars, '+');            offset |= plus  & 237
	fn.LocalGet("chars")
	fn.V128Const16(cPlus)
	fn.I8x16Eq()
	fn.V128Const16(off237)
	fn.V128And()
	fn.V128Or()

	//   slash_mask = eq(chars, '/');            offset |= slash & 240
	fn.LocalGet("chars")
	fn.V128Const16(cSlash)
	fn.I8x16Eq()
	fn.V128Const16(off240)
	fn.V128And()
	fn.V128Or()

	// Stack now: [chars, offset]. i8x16.sub → chars - offset.
	fn.I8x16Sub()
	fn.LocalSet("idx")

	// Precompute shifted versions.
	fn.LocalGet("idx")
	fn.I32Const(2)
	fn.I8x16Shl()
	fn.LocalSet("shl2")

	fn.LocalGet("idx")
	fn.I32Const(4)
	fn.I8x16Shl()
	fn.LocalSet("shl4")

	fn.LocalGet("idx")
	fn.I32Const(6)
	fn.I8x16Shl()
	fn.LocalSet("shl6")

	fn.LocalGet("idx")
	fn.I32Const(4)
	fn.I8x16ShrU()
	fn.LocalSet("shr4")

	fn.LocalGet("idx")
	fn.I32Const(2)
	fn.I8x16ShrU()
	fn.LocalSet("shr2")

	// left = shuffle( shuffle(shl2, shl4, patLeft23), shl6, patLeft6 )
	fn.LocalGet("shl2")
	fn.LocalGet("shl4")
	fn.I8x16Shuffle(patLeft23)
	fn.LocalSet("left23")

	// right = shuffle( shuffle(shr4, shr2, patRight24), idx, patRight0 )
	fn.LocalGet("shr4")
	fn.LocalGet("shr2")
	fn.I8x16Shuffle(patRight24)
	fn.LocalSet("right24")

	// Compute output address up front so the packed v128 is on top for store.
	fn.LocalGet("dstPtr")
	fn.LocalGet("i")
	fn.I32Const(12)
	fn.I32Mul()
	fn.I32Add()

	// left (final merge) — full left result:
	fn.LocalGet("left23")
	fn.LocalGet("shl6")
	fn.I8x16Shuffle(patLeft6)

	// | right (final merge)
	fn.LocalGet("right24")
	fn.LocalGet("idx")
	fn.I8x16Shuffle(patRight0)
	fn.V128Or()

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
