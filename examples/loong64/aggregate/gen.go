//go:build ignore

// Command gen produces aggregate_loong64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/loong64"
)

func main() {
	f := emit.NewFile("loong64")

	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Struct("p", abi.Field{Name: "A", Type: abi.Int64}, abi.Field{Name: "B", Type: abi.Int64})},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	pairSum := loong64.NewFunc("pairSum", sig, 0)
	pairSum.LoadArg("p_A", "R4").LoadArg("p_B", "R5").
		Raw("ADDV R5, R4, R6").StoreRet("R6", "ret").Ret()
	f.Add(pairSum.Func())

	sig = abi.LayoutArgs(
		[]abi.Arg{abi.Struct("m", abi.Field{Name: "Flag", Type: abi.Int8}, abi.Field{Name: "N", Type: abi.Int64})},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	mixedN := loong64.NewFunc("mixedN", sig, 0)
	mixedN.LoadArg("m_N", "R4").StoreRet("R4", "ret").Ret()
	f.Add(mixedN.Func())

	sig = abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	sliceLen := loong64.NewFunc("sliceLen", sig, 0)
	sliceLen.LoadArg("s_len", "R4").StoreRet("R4", "ret").Ret()
	f.Add(sliceLen.Func())

	sig = abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	sliceFirst := loong64.NewFunc("sliceFirst", sig, 0)
	sliceFirst.LoadArg("s_base", "R4").Raw("MOVV (R4), R5").StoreRet("R5", "ret").Ret()
	f.Add(sliceFirst.Func())

	sig = abi.LayoutArgs([]abi.Arg{abi.String("x")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	strLen := loong64.NewFunc("strLen", sig, 0)
	strLen.LoadArg("x_len", "R4").StoreRet("R4", "ret").Ret()
	f.Add(strLen.Func())

	if err := os.WriteFile("aggregate_loong64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote aggregate_loong64.s")
}
