// Command hex builds hex.wat programmatically via the wasm-emit package — the
// second kernel after matchlen, showing the emitter generalises to a wider
// v128 op mix (const tables, shr/and, swizzle-LUT, shuffle-interleave).
//
// Algorithm (same as the amd64 SSSE3 kernel in go-simd/hex):
//
//	for i in 0..nBlocks:
//	  bytes = v128.load(src + 16*i)
//	  hi    = (bytes >> 4) & 0x0f     // high nibbles
//	  lo    = bytes & 0x0f            // low nibbles
//	  hiCh  = swizzle(LUT, hi)        // ASCII high-nibble chars
//	  loCh  = swizzle(LUT, lo)        // ASCII low-nibble chars
//	  out0  = interleaveLow(hiCh, loCh)   // chars for bytes 0..7
//	  out1  = interleaveHigh(hiCh, loCh)  // chars for bytes 8..15
//	  store out0 @ dst + 32*i
//	  store out1 @ dst + 32*i + 16
//
// The interleave uses i8x16.shuffle with fixed immediates that map to
// SSSE3 PUNPCKLBW / PUNPCKHBW. See the shuffle constants below.
//
// Signature (as seen by //go:wasmimport consumers):
//
//	(func $hex_encode (param $dstPtr i32) (param $srcPtr i32)
//	                  (param $nBlocks i32))
//
// Each block converts 16 input bytes → 32 hex chars. The Go caller passes
// nBlocks = len(src)/16 and handles the tail with a scalar 1-byte-per-iter
// loop.
//
// Run with:
//
//	go run . > hex.wat
//
// Verify by round-tripping through wat2wasm and running the wazero test in
// this repo's e2e harness (see hex_test.go once it lands).
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

	// LUT[i] = ASCII for nibble i.
	lut := []byte("0123456789abcdef")
	// Low-nibble mask (0x0f × 16).
	lowMask := make([]byte, 16)
	for i := range lowMask {
		lowMask[i] = 0x0f
	}
	// PUNPCKLBW-equivalent shuffle: interleave the LOW 8 bytes of hi and lo
	// as (hi[0], lo[0], hi[1], lo[1], …, hi[7], lo[7]).
	// wasm i8x16.shuffle takes (a, b) — we push hi first, then lo, so a=hi,
	// b=lo and indices 0..15 pick from hi[0..15], 16..31 pick from lo[0..15].
	shuffleLo := []byte{0, 16, 1, 17, 2, 18, 3, 19, 4, 20, 5, 21, 6, 22, 7, 23}
	// PUNPCKHBW-equivalent: (hi[8], lo[8], …, hi[15], lo[15]).
	shuffleHi := []byte{8, 24, 9, 25, 10, 26, 11, 27, 12, 28, 13, 29, 14, 30, 15, 31}

	fn := wasm.NewFunction("hex_encode", "hex_encode",
		[]wasm.Param{
			{Name: "dstPtr", Type: wasm.I32},
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		nil,
	)
	fn.Local("i", wasm.I32)
	fn.Local("bytes", wasm.V128)
	fn.Local("hi", wasm.V128)
	fn.Local("lo", wasm.V128)
	fn.Local("hiCh", wasm.V128)
	fn.Local("loCh", wasm.V128)

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

	// hi = (bytes >> 4) & lowMask
	fn.LocalGet("bytes")
	fn.I32Const(4)
	fn.I8x16ShrU()
	fn.V128Const16(lowMask)
	fn.V128And()
	fn.LocalSet("hi")

	// lo = bytes & lowMask
	fn.LocalGet("bytes")
	fn.V128Const16(lowMask)
	fn.V128And()
	fn.LocalSet("lo")

	// hiCh = swizzle(LUT, hi) — table on top of stack, indices below? No,
	// wasm i8x16.swizzle pops indices then table (or vice versa) — per spec:
	// (i8x16.swizzle a b) where a is the vector to shuffle by b's indices.
	// So push LUT (the table) first, then hi (the indices).
	fn.V128Const16(lut)
	fn.LocalGet("hi")
	fn.I8x16Swizzle()
	fn.LocalSet("hiCh")

	// loCh = swizzle(LUT, lo)
	fn.V128Const16(lut)
	fn.LocalGet("lo")
	fn.I8x16Swizzle()
	fn.LocalSet("loCh")

	// out0 = shuffleLo(hiCh, loCh) → store @ dstPtr + i*32
	fn.LocalGet("dstPtr")
	fn.LocalGet("i")
	fn.I32Const(32)
	fn.I32Mul()
	fn.I32Add()
	fn.LocalGet("hiCh")
	fn.LocalGet("loCh")
	fn.I8x16Shuffle(shuffleLo)
	fn.V128Store(0)

	// out1 = shuffleHi(hiCh, loCh) → store @ dstPtr + i*32 + 16
	fn.LocalGet("dstPtr")
	fn.LocalGet("i")
	fn.I32Const(32)
	fn.I32Mul()
	fn.I32Add()
	fn.LocalGet("hiCh")
	fn.LocalGet("loCh")
	fn.I8x16Shuffle(shuffleHi)
	fn.V128Store(16)

	// i += 1
	fn.LocalGet("i")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("i")
	fn.Br(loop)

	fn.End() // loop
	fn.End() // block

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
