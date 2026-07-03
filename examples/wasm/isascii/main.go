// Command isascii builds isascii.wat programmatically via the wasm-emit
// package — the sixth kernel after matchlen, hex, popcount, toupper, and
// memchr. It's the wasm-SIMD preflight every parser wants: given a buffer,
// return 1 iff every byte is ASCII (< 128), else 0.
//
// The kernel takes advantage of `i8x16.bitmask` interpreting each lane
// signed — the "sign bit" of a byte and its "is >= 128" bit are the same
// bit. So bitmask(bytes) is directly the set of lanes whose value is
// non-ASCII: mask == 0 iff every byte in the block is in the 7-bit ASCII
// range.
//
// Algorithm (per 16-byte block):
//
//	for i in 0..nBlocks:
//	  bytes = v128.load(src + 16*i)
//	  mask  = i8x16.bitmask(bytes)   // 1 bit per lane, set if MSB is 1
//	  if mask != 0:
//	    return 0     // saw a non-ASCII byte in this block
//	return 1          // every scanned byte was ASCII
//
// Same overall shape as memchr but simpler — no needle to broadcast, no
// splat, no i32.ctz on the found path (we don't need the position, just
// the yes/no).
//
// Signature:
//
//	(func $isascii (param $srcPtr i32) (param $nBlocks i32) (result i32))
//
// The caller passes `nBlocks = len(buf)/16` and handles the tail with a
// scalar 1-byte-per-iter loop (or by pre-padding to a 16-byte boundary).
//
// Run with:
//
//	go run . > isascii.wat
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

	fn := wasm.NewFunction("isascii", "isascii",
		[]wasm.Param{
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		[]wasm.ValueType{wasm.I32},
	)
	fn.Local("i", wasm.I32)

	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit with 1 (all ASCII)
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// mask = i8x16.bitmask(v128.load(srcPtr + i*16))
	// if mask != 0 → return 0 (non-ASCII byte found)
	//
	// The inner "found non-ASCII" block is empty-typed ([] -> []): the
	// bitmask + eqz + br_if stack work stays inside it. On mask == 0 we
	// fall through and advance; on mask != 0 we return 0 directly.
	notFound := fn.Block("notFound")
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.I8x16Bitmask()
	fn.I32Eqz()
	fn.BrIf(notFound)

	// Non-ASCII path: push 0, return.
	fn.I32Const(0)
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

	// All blocks scanned without a non-ASCII byte → return 1.
	fn.I32Const(1)

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
