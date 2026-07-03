// Command utf8len builds utf8len.wat programmatically via the wasm-emit
// package — the seventh kernel after matchlen, hex, popcount, toupper,
// memchr, and isascii. It shapes like `utf8.RuneCountInString`: given a
// valid UTF-8 buffer, return the number of runes (Unicode code points)
// in the scanned bytes.
//
// The technique is a two-step horizontal reduction that mirrors popcount
// but at a coarser grain. For each 16-byte block:
//
//   1. AND the input with 0xc0 × 16 to isolate the top two bits of each
//      byte, then compare against 0x80 × 16. UTF-8 continuation bytes
//      have the exact 10xxxxxx pattern, so this compare produces 0xff
//      exactly on the continuation lanes.
//   2. i8x16.bitmask compresses those 16 per-lane truth bits into a
//      scalar i32 (bits 0..15 set on continuation lanes).
//   3. i32.popcnt on that i32 counts how many bytes in the block were
//      continuations. The rune count is the number of non-continuation
//      bytes: 16 - popcnt(mask). Accumulate across blocks.
//
// This is exact for valid UTF-8 inputs — every rune has exactly one
// non-continuation leader byte, so the count of leaders equals the
// count of runes. Malformed inputs give an approximation (still equals
// the count of bytes whose top two bits aren't 10).
//
// Algorithm:
//
//	count = 0
//	cMask = 0xc0 × 16
//	tMask = 0x80 × 16
//	for i in 0..nBlocks:
//	  bytes = v128.load(src + 16*i)
//	  bit10 = i8x16.eq(v128.and(bytes, cMask), tMask)  // 0xff on cont bytes
//	  cont  = i32.popcnt(i8x16.bitmask(bit10))         // count in block
//	  count += 16 - cont                                 // rune starts
//	return count
//
// Signature:
//
//	(func $utf8len (param $srcPtr i32) (param $nBlocks i32) (result i32))
//
// Each block processes 16 input bytes. Caller handles the tail with a
// scalar loop.
//
// Run with:
//
//	go run . > utf8len.wat
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

	cVec := make([]byte, 16) // 0xc0 × 16 — the top-two-bits mask
	tVec := make([]byte, 16) // 0x80 × 16 — the continuation-byte tag
	for i := range cVec {
		cVec[i] = 0xc0
		tVec[i] = 0x80
	}

	fn := wasm.NewFunction("utf8len", "utf8len",
		[]wasm.Param{
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		[]wasm.ValueType{wasm.I32},
	)
	fn.Local("i", wasm.I32)
	fn.Local("count", wasm.I32)
	fn.Local("cont", wasm.I32)

	fn.I32Const(0)
	fn.LocalSet("count")
	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// bit10 = i8x16.eq( v128.load(srcPtr + i*16) & cVec, tVec )
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.V128Const16(cVec)
	fn.V128And()
	fn.V128Const16(tVec)
	fn.I8x16Eq()

	// cont = i32.popcnt( i8x16.bitmask(bit10) )   — continuations in this block
	fn.I8x16Bitmask()
	fn.I32Popcnt()
	fn.LocalSet("cont")

	// count = (count + 16) - cont
	// wasm i32.sub pops top - second-from-top, so pushing (count+16)
	// first and cont on top gives the correct evaluation:
	// (count + 16) is second-from-top, cont is on top → (count+16) - cont.
	fn.LocalGet("count")
	fn.I32Const(16)
	fn.I32Add()
	fn.LocalGet("cont")
	fn.I32Sub()
	fn.LocalSet("count")

	// i += 1
	fn.LocalGet("i")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("i")
	fn.Br(loop)

	fn.End() // loop
	fn.End() // done block

	fn.LocalGet("count")

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
