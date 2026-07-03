// Command toupper builds toupper.wat programmatically via the wasm-emit
// package — the fourth kernel after matchlen, hex, and popcount. It showcases
// the text-processing range-check pattern common in encoding, parsing, and
// case-fold pipelines: a pair of signed-compare v128 ops (i8x16.ge_s and
// i8x16.le_s) identifies bytes inside a range, and the mask drives a per-lane
// arithmetic delta.
//
// Algorithm (per 16 input bytes → 16 output bytes):
//
//	bytes  = v128.load(src + 16*i)
//	geA    = i8x16.ge_s(bytes, 'a' × 16)   // 0xff where >= 'a'
//	leZ    = i8x16.le_s(bytes, 'z' × 16)   // 0xff where <= 'z'
//	mask   = geA & leZ                     // 0xff where in [a..z]
//	delta  = mask & (0x20 × 16)            // 0x20 where lowercase, else 0
//	upper  = i8x16.sub(bytes, delta)       // subtract 32 to uppercase
//	v128.store upper @ dst + 16*i
//
// Every byte outside [a..z] is passed through untouched — matches Go's
// strings.ToUpper for ASCII-only input.
//
// Signature:
//
//	(func $toupper (param $dstPtr i32) (param $srcPtr i32) (param $nBlocks i32))
//
// Each block processes 16 bytes. Caller handles the tail with a scalar loop.
//
// Run with:
//
//	go run . > toupper.wat
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

	// Broadcast constants: 'a'×16, 'z'×16, 0x20×16.
	aVec := make([]byte, 16)
	zVec := make([]byte, 16)
	dVec := make([]byte, 16)
	for i := range aVec {
		aVec[i] = 'a'
		zVec[i] = 'z'
		dVec[i] = 0x20
	}

	fn := wasm.NewFunction("toupper", "toupper",
		[]wasm.Param{
			{Name: "dstPtr", Type: wasm.I32},
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		nil,
	)
	fn.Local("i", wasm.I32)
	fn.Local("bytes", wasm.V128)
	fn.Local("mask", wasm.V128)

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

	// mask = (bytes >= 'a') & (bytes <= 'z')
	fn.LocalGet("bytes")
	fn.V128Const16(aVec)
	fn.I8x16GeS()
	fn.LocalGet("bytes")
	fn.V128Const16(zVec)
	fn.I8x16LeS()
	fn.V128And()
	fn.LocalSet("mask")

	// out = bytes - (mask & 0x20)  → store @ dstPtr + i*16
	fn.LocalGet("dstPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.LocalGet("bytes")
	fn.LocalGet("mask")
	fn.V128Const16(dVec)
	fn.V128And()
	fn.I8x16Sub()
	fn.V128Store(0)

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
