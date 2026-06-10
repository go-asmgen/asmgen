//go:build ignore

// Command gen produces types_riscv64.s. Run with: go run gen.go
//
// It mirrors the arm64 types example, but drives the riscv64 builder — the same
// ABI0 layout, a different move table — to show how cheap a second architecture
// is once the layout model is shared.
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/internal/emit"
	"github.com/go-asmgen/asmgen/riscv64"
)

func main() {
	f := emit.NewFile("riscv64")

	f.Add(intBinOp("addInt64", riscv64.Int64, "ADD"))
	f.Add(intBinOp("addInt32", riscv64.Int32, "ADDW"))

	f.Add(widen("widenInt8", riscv64.Int8, riscv64.Int64))
	f.Add(widen("widenUint8", riscv64.Uint8, riscv64.Uint64))

	f.Add(floatBinOp("addFloat64", riscv64.Float64, "FADDD"))
	f.Add(floatBinOp("addFloat32", riscv64.Float32, "FADDS"))

	write("types_riscv64.s", f)
}

// intBinOp builds func(a, b T) T computing a (op) b in integer registers
// (X5, X6 -> X7).
func intBinOp(name string, t riscv64.Type, op string) *emit.Function {
	sig := riscv64.Layout(
		[]string{"a", "b"}, []riscv64.Type{t, t},
		[]string{"ret"}, []riscv64.Type{t},
	)
	b := riscv64.NewFunc(name, sig, 0)
	b.LoadArg("a", "X5").
		LoadArg("b", "X6").
		Raw("%s X6, X5, X7", op).
		StoreRet("X7", "ret").
		Ret()
	return b.Func()
}

// floatBinOp builds func(a, b T) T computing a (op) b in float registers
// (F0, F1 -> F2).
func floatBinOp(name string, t riscv64.Type, op string) *emit.Function {
	sig := riscv64.Layout(
		[]string{"a", "b"}, []riscv64.Type{t, t},
		[]string{"ret"}, []riscv64.Type{t},
	)
	b := riscv64.NewFunc(name, sig, 0)
	b.LoadArg("a", "F0").
		LoadArg("b", "F1").
		Raw("%s F1, F0, F2", op).
		StoreRet("F2", "ret").
		Ret()
	return b.Func()
}

// widen builds func(a IN) OUT that re-stores the loaded (and extended) value at
// the wider width.
func widen(name string, in, out riscv64.Type) *emit.Function {
	sig := riscv64.Layout(
		[]string{"a"}, []riscv64.Type{in},
		[]string{"ret"}, []riscv64.Type{out},
	)
	b := riscv64.NewFunc(name, sig, 0)
	b.LoadArg("a", "X5").
		StoreRet("X5", "ret").
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
