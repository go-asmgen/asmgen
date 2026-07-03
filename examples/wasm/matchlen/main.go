// Command matchlen builds matchlen.wat programmatically via the wasm-emit
// package — proving out the go-asmgen-style flow for wasm-SIMD. The output is
// the byte-exact WAT text that we hand-wrote in go-simd/matchlen-wasm.
//
// Run with:
//
//	go run . > matchlen.wat
//
// Then verify by round-tripping through wat2wasm + running the wazero
// end-to-end tests from go-simd/matchlen-wasm.
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

	fn := wasm.NewFunction("matchlen16", "matchlen16",
		[]wasm.Param{
			{Name: "ptrA", Type: wasm.I32},
			{Name: "ptrB", Type: wasm.I32},
			{Name: "limit", Type: wasm.I32},
		},
		[]wasm.ValueType{wasm.I32},
	)
	fn.Local("i", wasm.I32)
	fn.Local("matched", wasm.I32)
	fn.Local("eq", wasm.V128)

	fn.I32Const(0)
	fn.LocalSet("matched")
	fn.I32Const(0)
	fn.LocalSet("i")

	// Chunk loop: process 16 bytes per iteration.
	chunkDone := fn.Block("chunk_done")
	chunkLoop := fn.Loop("chunk_loop")

	// if i + 16 > limit → exit
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Add()
	fn.LocalGet("limit")
	fn.I32GtS()
	fn.BrIf(chunkDone)

	// eq = i8x16.eq( v128.load(ptrA + i), v128.load(ptrB + i) )
	fn.LocalGet("ptrA")
	fn.LocalGet("i")
	fn.I32Add()
	fn.V128Load(0)
	fn.LocalGet("ptrB")
	fn.LocalGet("i")
	fn.I32Add()
	fn.V128Load(0)
	fn.I8x16Eq()
	fn.LocalSet("eq")

	// if NOT all_true(eq) → break out
	fn.LocalGet("eq")
	fn.I8x16AllTrue()
	fn.I32Eqz()
	fn.BrIf(chunkDone)

	// matched += 16; i += 16
	fn.LocalGet("matched")
	fn.I32Const(16)
	fn.I32Add()
	fn.LocalSet("matched")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Add()
	fn.LocalSet("i")

	fn.Br(chunkLoop)
	fn.End() // loop
	fn.End() // block

	// Byte tail: find the exact first-mismatch offset.
	byteDone := fn.Block("byte_done")
	byteLoop := fn.Loop("byte_loop")

	// if matched >= limit → exit
	fn.LocalGet("matched")
	fn.LocalGet("limit")
	fn.I32GeS()
	fn.BrIf(byteDone)

	// if load8_u(ptrA + matched) != load8_u(ptrB + matched) → exit
	fn.LocalGet("ptrA")
	fn.LocalGet("matched")
	fn.I32Add()
	fn.I32Load8U()
	fn.LocalGet("ptrB")
	fn.LocalGet("matched")
	fn.I32Add()
	fn.I32Load8U()
	fn.I32Ne()
	fn.BrIf(byteDone)

	// matched += 1
	fn.LocalGet("matched")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("matched")

	fn.Br(byteLoop)
	fn.End() // loop
	fn.End() // block

	// Return matched (implicit — top of stack).
	fn.LocalGet("matched")

	m.Add(fn)

	_, err := fmt.Fprint(out, m.String())
	return err
}
