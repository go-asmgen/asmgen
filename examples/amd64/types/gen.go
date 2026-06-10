//go:build ignore

// Command gen produces types_amd64.s. Run with: go run gen.go
//
// Same structure as the other architecture examples, driving the amd64 builder.
// amd64 arithmetic is two-operand (ADDQ b, a computes a += b), so the binary
// helpers accumulate into the first register.
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	f := emit.NewFile("amd64")

	f.Add(intBinOp("addInt64", amd64.Int64, "ADDQ"))
	f.Add(intBinOp("addInt32", amd64.Int32, "ADDL"))

	f.Add(widen("widenInt8", amd64.Int8, amd64.Int64))
	f.Add(widen("widenUint8", amd64.Uint8, amd64.Uint64))

	f.Add(floatBinOp("addFloat64", amd64.Float64, "ADDSD"))
	f.Add(floatBinOp("addFloat32", amd64.Float32, "ADDSS"))

	write("types_amd64.s", f)
}

// intBinOp builds func(a, b T) T computing a (op) b in integer registers
// (AX += BX).
func intBinOp(name string, t amd64.Type, op string) *emit.Function {
	sig := amd64.Layout(
		[]string{"a", "b"}, []amd64.Type{t, t},
		[]string{"ret"}, []amd64.Type{t},
	)
	b := amd64.NewFunc(name, sig, 0)
	b.LoadArg("a", "AX").
		LoadArg("b", "BX").
		Raw("%s BX, AX", op).
		StoreRet("AX", "ret").
		Ret()
	return b.Func()
}

// floatBinOp builds func(a, b T) T computing a (op) b in SSE registers
// (X0 += X1).
func floatBinOp(name string, t amd64.Type, op string) *emit.Function {
	sig := amd64.Layout(
		[]string{"a", "b"}, []amd64.Type{t, t},
		[]string{"ret"}, []amd64.Type{t},
	)
	b := amd64.NewFunc(name, sig, 0)
	b.LoadArg("a", "X0").
		LoadArg("b", "X1").
		Raw("%s X1, X0", op).
		StoreRet("X0", "ret").
		Ret()
	return b.Func()
}

// widen builds func(a IN) OUT that re-stores the loaded (and extended) value at
// the wider width.
func widen(name string, in, out amd64.Type) *emit.Function {
	sig := amd64.Layout(
		[]string{"a"}, []amd64.Type{in},
		[]string{"ret"}, []amd64.Type{out},
	)
	b := amd64.NewFunc(name, sig, 0)
	b.LoadArg("a", "AX").
		StoreRet("AX", "ret").
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
