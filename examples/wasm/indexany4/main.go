// Command indexany4 builds indexany4.wat programmatically via the wasm-emit
// package — the twelfth kernel: a 4-needle variant of memchr that shapes
// like bytes.IndexAny with a fixed-size needle set. Given a buffer and
// four search bytes n1..n4, return the index of the first byte in the
// buffer that equals any of the four, or -1 if none match.
//
// The caller duplicates entries in n1..n4 when they have fewer than four
// distinct needles (e.g. IndexAny(buf, ",") passes n1=n2=n3=n4=','). For
// more than four needles the caller either loops the kernel per set of
// four or falls back to scalar bytes.IndexAny — both remain far cheaper
// than the boundary cost for buffers past ~1 KiB.
//
// Algorithm per 16-byte block (all four needles broadcast to v128 once
// before the loop):
//
//	bytes = v128.load(src + 16*i)
//	m     = i8x16.eq(bytes, key1)
//	      | i8x16.eq(bytes, key2)
//	      | i8x16.eq(bytes, key3)
//	      | i8x16.eq(bytes, key4)
//	bm    = i8x16.bitmask(m)   ; MSB-of-lane bits into i32 (0..0xffff)
//	if bm != 0:
//	  return i*16 + i32.ctz(bm)
//	; else advance
//
// Same shape as memchr but with 4 broadcast-key vectors and 4 compares
// OR-ed together. No new emit surface: reuses i8x16.splat, i8x16.eq,
// v128.or, i8x16.bitmask, i32.ctz, i32.eqz, i32.mul, i32.add.
//
// Signature:
//
//	(func $indexany4 (param $srcPtr i32) (param $nBlocks i32)
//	                 (param $n1 i32) (param $n2 i32)
//	                 (param $n3 i32) (param $n4 i32)
//	                 (result i32))
//
// Returns >=0 (byte offset of first match) or -1 (no match).
//
// Run with:
//
//	go run . > indexany4.wat
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

	fn := wasm.NewFunction("indexany4", "indexany4",
		[]wasm.Param{
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
			{Name: "n1", Type: wasm.I32},
			{Name: "n2", Type: wasm.I32},
			{Name: "n3", Type: wasm.I32},
			{Name: "n4", Type: wasm.I32},
		},
		[]wasm.ValueType{wasm.I32},
	)
	fn.Local("i", wasm.I32)
	fn.Local("bytes", wasm.V128)
	fn.Local("k1", wasm.V128)
	fn.Local("k2", wasm.V128)
	fn.Local("k3", wasm.V128)
	fn.Local("k4", wasm.V128)
	fn.Local("mask", wasm.I32)

	// Broadcast each needle once, before the loop.
	fn.LocalGet("n1")
	fn.I8x16Splat()
	fn.LocalSet("k1")
	fn.LocalGet("n2")
	fn.I8x16Splat()
	fn.LocalSet("k2")
	fn.LocalGet("n3")
	fn.I8x16Splat()
	fn.LocalSet("k3")
	fn.LocalGet("n4")
	fn.I8x16Splat()
	fn.LocalSet("k4")

	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit with -1 (fall through to trailing i32.const -1).
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// bytes = v128.load(src + i*16)
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.LocalSet("bytes")

	// mask = i8x16.bitmask(
	//   eq(bytes,k1) | eq(bytes,k2) | eq(bytes,k3) | eq(bytes,k4) )
	fn.LocalGet("bytes")
	fn.LocalGet("k1")
	fn.I8x16Eq()
	fn.LocalGet("bytes")
	fn.LocalGet("k2")
	fn.I8x16Eq()
	fn.V128Or()
	fn.LocalGet("bytes")
	fn.LocalGet("k3")
	fn.I8x16Eq()
	fn.V128Or()
	fn.LocalGet("bytes")
	fn.LocalGet("k4")
	fn.I8x16Eq()
	fn.V128Or()
	fn.I8x16Bitmask()
	fn.LocalSet("mask")

	// If mask != 0 → return i*16 + ctz(mask). Empty-typed inner block:
	// on mask==0 br past the found-path; on mask!=0 execute return.
	notFound := fn.Block("notFound")
	fn.LocalGet("mask")
	fn.I32Eqz()
	fn.BrIf(notFound)

	// Found path.
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.LocalGet("mask")
	fn.I32Ctz()
	fn.I32Add()
	fn.Return()

	fn.End() // notFound block

	// i += 1
	fn.LocalGet("i")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("i")
	fn.Br(loop)

	fn.End() // loop
	fn.End() // done block

	// No match found.
	fn.I32Const(-1)

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
