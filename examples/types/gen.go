//go:build ignore

// Command gen produces types_arm64.s. Run with: go run gen.go
//
// It generates one function per type case so the emitted assembly exercises the
// full move-selection matrix of the arm64 builder.
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/internal/emit"
)

func main() {
	f := emit.NewFile("arm64")

	// Integer binary ops: load both args, apply the op, store the result.
	// 32-bit ints use ADDW (32-bit add); narrower ints use ADD on the
	// sign/zero-extended values and store back the low bytes.
	f.Add(intBinOp("addInt32", arm64.Int32, "ADDW"))
	f.Add(intBinOp("addUint32", arm64.Uint32, "ADDW"))
	f.Add(intBinOp("addInt16", arm64.Int16, "ADD"))
	f.Add(intBinOp("addInt8", arm64.Int8, "ADD"))

	// Sub-word -> wide: the load's sign/zero extension is the whole point, so
	// the body is just load-then-store.
	f.Add(widen("widenInt8", arm64.Int8, arm64.Int64))
	f.Add(widen("widenUint8", arm64.Uint8, arm64.Uint64))

	// Floats use the F register file and FADD{S,D}.
	f.Add(floatBinOp("addFloat64", arm64.Float64, "FADDD"))
	f.Add(floatBinOp("addFloat32", arm64.Float32, "FADDS"))

	write("types_arm64.s", f)
}

// intBinOp builds func(a, b T) T computing a (op) b in integer registers.
func intBinOp(name string, t arm64.Type, op string) *emit.Function {
	sig := arm64.Layout(
		[]string{"a", "b"}, []arm64.Type{t, t},
		[]string{"ret"}, []arm64.Type{t},
	)
	b := arm64.NewFunc(name, sig, 0)
	b.LoadArg("a", "R0").
		LoadArg("b", "R1").
		Raw("%s R1, R0, R2", op).
		StoreRet("R2", "ret").
		Ret()
	return b.Func()
}

// floatBinOp builds func(a, b T) T computing a (op) b in float registers.
func floatBinOp(name string, t arm64.Type, op string) *emit.Function {
	sig := arm64.Layout(
		[]string{"a", "b"}, []arm64.Type{t, t},
		[]string{"ret"}, []arm64.Type{t},
	)
	b := arm64.NewFunc(name, sig, 0)
	b.LoadArg("a", "F0").
		LoadArg("b", "F1").
		Raw("%s F1, F0, F2", op).
		StoreRet("F2", "ret").
		Ret()
	return b.Func()
}

// widen builds func(a IN) OUT that just re-stores the loaded (and extended)
// value at the wider width.
func widen(name string, in, out arm64.Type) *emit.Function {
	sig := arm64.Layout(
		[]string{"a"}, []arm64.Type{in},
		[]string{"ret"}, []arm64.Type{out},
	)
	b := arm64.NewFunc(name, sig, 0)
	b.LoadArg("a", "R0").
		StoreRet("R0", "ret").
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
