//go:build ignore

// Command gen produces aggregate_amd64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	f := emit.NewFile("amd64")

	// func pairSum(p Pair) int64 { return p.A + p.B }
	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Struct("p", abi.Field{Name: "A", Type: abi.Int64}, abi.Field{Name: "B", Type: abi.Int64})},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	pairSum := amd64.NewFunc("pairSum", sig, 0)
	pairSum.LoadArg("p_A", "AX").LoadArg("p_B", "BX").
		Raw("ADDQ BX, AX").StoreRet("AX", "ret").Ret()
	f.Add(pairSum.Func())

	// func mixedN(m Mixed) int64 { return m.N }  — N at padded offset 8.
	sig = abi.LayoutArgs(
		[]abi.Arg{abi.Struct("m", abi.Field{Name: "Flag", Type: abi.Int8}, abi.Field{Name: "N", Type: abi.Int64})},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	mixedN := amd64.NewFunc("mixedN", sig, 0)
	mixedN.LoadArg("m_N", "AX").StoreRet("AX", "ret").Ret()
	f.Add(mixedN.Func())

	// func sliceLen(s []int64) int { return len(s) }
	sig = abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	sliceLen := amd64.NewFunc("sliceLen", sig, 0)
	sliceLen.LoadArg("s_len", "AX").StoreRet("AX", "ret").Ret()
	f.Add(sliceLen.Func())

	// func sliceFirst(s []int64) int64 { return s[0] }  — load base, dereference.
	sig = abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	sliceFirst := amd64.NewFunc("sliceFirst", sig, 0)
	sliceFirst.LoadArg("s_base", "AX").Raw("MOVQ (AX), BX").StoreRet("BX", "ret").Ret()
	f.Add(sliceFirst.Func())

	// func strLen(x string) int { return len(x) }
	sig = abi.LayoutArgs([]abi.Arg{abi.String("x")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	strLen := amd64.NewFunc("strLen", sig, 0)
	strLen.LoadArg("x_len", "AX").StoreRet("AX", "ret").Ret()
	f.Add(strLen.Func())

	if err := os.WriteFile("aggregate_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote aggregate_amd64.s")
}
