// Command json_clean builds json_clean.wat programmatically via the
// wasm-emit package — the ninth kernel. It's the JSON-string preflight
// every parser wants: given a buffer, return 1 iff every byte can be
// copied verbatim into the current JSON string body — no `"` (0x22)
// closing the string, no `\` (0x5c) starting an escape, no control
// character (0x00..0x1f) that would need \uXXXX escaping.
//
// A parser can then take the fast path (`memcpy`-style bulk-copy the
// scanned bytes into its output buffer) when this returns 1, and drop
// to the byte-by-byte slow path when it returns 0. The kernel is the
// hottest inner check in almost every JSON string-parsing loop.
//
// Algorithm (per 16-byte block):
//
//	bytes    = v128.load(src + 16*i)
//	quotes   = i8x16.eq(bytes, 0x22 × 16)                ; 0xff on `"`
//	slashes  = i8x16.eq(bytes, 0x5c × 16)                ; 0xff on `\`
//	controls = i8x16.eq(v128.and(bytes, 0xe0 × 16),
//	                    v128.zero)                       ; 0xff on 0x00..0x1f
//	dirty    = quotes | slashes | controls               ; 0xff on any bad lane
//	if i8x16.bitmask(dirty) != 0:
//	  return 0
//	; else advance i
//	... on loop exit: return 1
//
// The control-char check uses AND-and-compare-to-zero instead of the
// obvious `i8x16.le_s(bytes, 0x1f)` because signed less-than would
// misclassify high-bit bytes (0x80..0xff, which are signed-negative) as
// control chars. `bytes & 0xe0 == 0` is true iff the top three bits are
// all clear, which happens exactly for byte values 0x00..0x1f.
//
// Shape mirrors isascii — a per-block classify + bitmask + i32.eqz test —
// but with three OR-combined predicates instead of one. No new emit-
// surface ops are needed.
//
// Signature:
//
//	(func $json_clean (param $srcPtr i32) (param $nBlocks i32) (result i32))
//
// Each block processes 16 bytes. Caller handles the tail with a scalar
// 1-byte-per-iter loop.
//
// Run with:
//
//	go run . > json_clean.wat
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

	broadcast := func(b byte) []byte {
		v := make([]byte, 16)
		for i := range v {
			v[i] = b
		}
		return v
	}
	quoteVec := broadcast(0x22) // '"'
	slashVec := broadcast(0x5c) // '\\'
	hi3Vec := broadcast(0xe0)   // mask for "byte < 0x20" check

	fn := wasm.NewFunction("json_clean", "json_clean",
		[]wasm.Param{
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		[]wasm.ValueType{wasm.I32},
	)
	fn.Local("i", wasm.I32)
	fn.Local("bytes", wasm.V128)

	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit with 1 (clean)
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

	// Compute dirty = (bytes == '"') | (bytes == '\\') | (bytes <= 0x1f)
	// then test if i8x16.bitmask(dirty) == 0. On dirty block, return 0
	// through the "notClean" nested block (empty-typed).
	notClean := fn.Block("notClean")

	// quotes
	fn.LocalGet("bytes")
	fn.V128Const16(quoteVec)
	fn.I8x16Eq()

	// | slashes
	fn.LocalGet("bytes")
	fn.V128Const16(slashVec)
	fn.I8x16Eq()
	fn.V128Or()

	// | controls  ((bytes & 0xe0) == 0 iff byte in 0x00..0x1f; using AND +
	//              compare-to-zero avoids the signed-less-than trap where
	//              high-bit bytes (0x80..0xff) look "negative" and would
	//              be misclassified as controls under i8x16.le_s)
	fn.LocalGet("bytes")
	fn.V128Const16(hi3Vec)
	fn.V128And()
	fn.V128Zero()
	fn.I8x16Eq()
	fn.V128Or()

	// i8x16.bitmask(dirty) == 0 → clean, br past return-0 path
	fn.I8x16Bitmask()
	fn.I32Eqz()
	fn.BrIf(notClean)

	// Dirty path: push 0, return.
	fn.I32Const(0)
	fn.Return()

	fn.End() // notClean block

	// i += 1
	fn.LocalGet("i")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("i")
	fn.Br(loop)

	fn.End() // loop
	fn.End() // done block

	// All blocks scanned without a dirty byte → return 1.
	fn.I32Const(1)

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
