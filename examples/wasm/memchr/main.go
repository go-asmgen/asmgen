// Command memchr builds memchr.wat programmatically via the wasm-emit
// package — the fifth kernel after matchlen, hex, popcount, and toupper.
// It's the wasm-SIMD counterpart of bytes.IndexByte / C memchr: given a
// buffer and a search byte, return the index of the first matching byte,
// or -1 if the byte is not present.
//
// This is the kernel where wasm-SIMD's `i8x16.bitmask` op earns its keep.
// SSE has PMOVMSKB with the same shape (MSB of each byte lane compressed
// into a scalar bitmask); wasm-SIMD gives us the same construction and
// pairing it with `i32.ctz` locates the first-set lane in one scalar op.
//
// Algorithm (per 16-byte block):
//
//	key    = i8x16.splat(needle)          // broadcast search byte
//	for i in 0..nBlocks:
//	  bytes = v128.load(src + 16*i)
//	  eq    = i8x16.eq(bytes, key)         // 0xff where match, 0 otherwise
//	  mask  = i8x16.bitmask(eq)            // MSB-of-lane bits → i32 (0..0xffff)
//	  if mask != 0:
//	     idx = i32.ctz(mask)               // 0..15, index of first match
//	     return i*16 + idx
//	return -1
//
// The caller passes `nBlocks = len(buf)/16` and handles the tail with a
// scalar 1-byte-per-iter loop (or by pre-padding to a 16-byte boundary).
//
// Signature (as seen by //go:wasmimport consumers):
//
//	(func $memchr (param $srcPtr i32) (param $nBlocks i32) (param $needle i32)
//	              (result i32))
//
// Returns:
//   - >= 0: byte offset of the first match within the searched blocks
//   - -1:   no match in any of the nBlocks × 16 scanned bytes
//
// Run with:
//
//	go run . > memchr.wat
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

	fn := wasm.NewFunction("memchr", "memchr",
		[]wasm.Param{
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
			{Name: "needle", Type: wasm.I32},
		},
		[]wasm.ValueType{wasm.I32},
	)
	fn.Local("i", wasm.I32)
	fn.Local("key", wasm.V128)
	fn.Local("mask", wasm.I32)

	// key = i8x16.splat(needle)
	fn.LocalGet("needle")
	fn.I8x16Splat()
	fn.LocalSet("key")

	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit with -1
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// mask = bitmask(v128.load(srcPtr + i*16) == key)
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.LocalGet("key")
	fn.I8x16Eq()
	fn.I8x16Bitmask()
	fn.LocalSet("mask")

	// The nested "notFound" block is empty-typed ([] -> []). All
	// stack work stays inside it; the block only carries control
	// flow. On mask==0 we branch past the return; on mask!=0 the
	// return exits the function directly.
	notFound := fn.Block("notFound")
	fn.LocalGet("mask")
	fn.I32Eqz()
	fn.BrIf(notFound)

	// Found path: return i*16 + ctz(mask)
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

	// No match → return -1
	fn.I32Const(-1)

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
