//go:build ignore

// Command gen produces types_loong64.s. Run with: go run gen.go
//
// Same structure as the arm64 and riscv64 examples, driving the loong64 builder
// — the same shared ABI0 layout, a third move table.
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/loong64"
)

func main() {
	f := emit.NewFile("loong64")

	f.Add(intBinOp("addInt64", loong64.Int64, "ADDV"))
	// loong64's assembler has no 3-register ADDW; a 64-bit ADDV with a MOVW
	// store yields the correct low-32-bit result. The example exercises the
	// MOVW load/store move selection regardless of the arithmetic width.
	f.Add(intBinOp("addInt32", loong64.Int32, "ADDV"))

	f.Add(widen("widenInt8", loong64.Int8, loong64.Int64))
	f.Add(widen("widenUint8", loong64.Uint8, loong64.Uint64))

	f.Add(floatBinOp("addFloat64", loong64.Float64, "ADDD"))
	f.Add(floatBinOp("addFloat32", loong64.Float32, "ADDF"))

	write("types_loong64.s", f)
}

// intBinOp builds func(a, b T) T computing a (op) b in integer registers
// (R4, R5 -> R6).
func intBinOp(name string, t loong64.Type, op string) *emit.Function {
	sig := loong64.Layout(
		[]string{"a", "b"}, []loong64.Type{t, t},
		[]string{"ret"}, []loong64.Type{t},
	)
	b := loong64.NewFunc(name, sig, 0)
	b.LoadArg("a", "R4").
		LoadArg("b", "R5").
		Raw("%s R5, R4, R6", op).
		StoreRet("R6", "ret").
		Ret()
	return b.Func()
}

// floatBinOp builds func(a, b T) T computing a (op) b in float registers
// (F0, F1 -> F2).
func floatBinOp(name string, t loong64.Type, op string) *emit.Function {
	sig := loong64.Layout(
		[]string{"a", "b"}, []loong64.Type{t, t},
		[]string{"ret"}, []loong64.Type{t},
	)
	b := loong64.NewFunc(name, sig, 0)
	b.LoadArg("a", "F0").
		LoadArg("b", "F1").
		Raw("%s F1, F0, F2", op).
		StoreRet("F2", "ret").
		Ret()
	return b.Func()
}

// widen builds func(a IN) OUT that re-stores the loaded (and extended) value at
// the wider width.
func widen(name string, in, out loong64.Type) *emit.Function {
	sig := loong64.Layout(
		[]string{"a"}, []loong64.Type{in},
		[]string{"ret"}, []loong64.Type{out},
	)
	b := loong64.NewFunc(name, sig, 0)
	b.LoadArg("a", "R4").
		StoreRet("R4", "ret").
		Ret()
	return b.Func()
}

func write(path string, f *emit.File) {
	if err := os.WriteFile(path, []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", path)
}
