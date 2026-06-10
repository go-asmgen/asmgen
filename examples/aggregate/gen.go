//go:build ignore

// Command gen produces aggregate_arm64.s. Run with: go run gen.go
//
// It demonstrates aggregate (struct / slice / string) ABI0 parameters. The
// layout is computed by abi.LayoutArgs, which flattens each aggregate into one
// field-named slot (p_A, s_base, x_len, ...); the arm64 builder loads those
// slots unchanged, and pointer dereference uses the Raw escape hatch.
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	f := emit.NewFile("arm64")

	// func pairSum(p Pair) int64 { return p.A + p.B }
	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Struct("p", abi.Field{Name: "A", Type: abi.Int64}, abi.Field{Name: "B", Type: abi.Int64})},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	pairSum := arm64.NewFunc("pairSum", sig, 0)
	pairSum.LoadArg("p_A", "R0").
		LoadArg("p_B", "R1").
		Raw("ADD R1, R0, R2").
		StoreRet("R2", "ret").
		Ret()
	f.Add(pairSum.Func())

	// func mixedN(m Mixed) int64 { return m.N }  — N is at the padded offset 8.
	sig = abi.LayoutArgs(
		[]abi.Arg{abi.Struct("m", abi.Field{Name: "Flag", Type: abi.Int8}, abi.Field{Name: "N", Type: abi.Int64})},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	mixedN := arm64.NewFunc("mixedN", sig, 0)
	mixedN.LoadArg("m_N", "R0").
		StoreRet("R0", "ret").
		Ret()
	f.Add(mixedN.Func())

	// func sliceLen(s []int64) int { return len(s) }
	sig = abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	sliceLen := arm64.NewFunc("sliceLen", sig, 0)
	sliceLen.LoadArg("s_len", "R0").
		StoreRet("R0", "ret").
		Ret()
	f.Add(sliceLen.Func())

	// func sliceFirst(s []int64) int64 { return s[0] }  — load base, dereference.
	sig = abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	sliceFirst := arm64.NewFunc("sliceFirst", sig, 0)
	sliceFirst.LoadArg("s_base", "R0").
		Raw("MOVD (R0), R1").
		StoreRet("R1", "ret").
		Ret()
	f.Add(sliceFirst.Func())

	// func strLen(x string) int { return len(x) }
	sig = abi.LayoutArgs([]abi.Arg{abi.String("x")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	strLen := arm64.NewFunc("strLen", sig, 0)
	strLen.LoadArg("x_len", "R0").
		StoreRet("R0", "ret").
		Ret()
	f.Add(strLen.Func())

	if err := os.WriteFile("aggregate_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote aggregate_arm64.s")
}
