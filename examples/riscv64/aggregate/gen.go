//go:build ignore

// Command gen produces aggregate_riscv64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/riscv64"
)

func main() {
	f := emit.NewFile("riscv64")

	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Struct("p", abi.Field{Name: "A", Type: abi.Int64}, abi.Field{Name: "B", Type: abi.Int64})},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	pairSum := riscv64.NewFunc("pairSum", sig, 0)
	pairSum.LoadArg("p_A", "X5").LoadArg("p_B", "X6").
		Raw("ADD X6, X5, X7").StoreRet("X7", "ret").Ret()
	f.Add(pairSum.Func())

	sig = abi.LayoutArgs(
		[]abi.Arg{abi.Struct("m", abi.Field{Name: "Flag", Type: abi.Int8}, abi.Field{Name: "N", Type: abi.Int64})},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	mixedN := riscv64.NewFunc("mixedN", sig, 0)
	mixedN.LoadArg("m_N", "X5").StoreRet("X5", "ret").Ret()
	f.Add(mixedN.Func())

	sig = abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	sliceLen := riscv64.NewFunc("sliceLen", sig, 0)
	sliceLen.LoadArg("s_len", "X5").StoreRet("X5", "ret").Ret()
	f.Add(sliceLen.Func())

	sig = abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	sliceFirst := riscv64.NewFunc("sliceFirst", sig, 0)
	sliceFirst.LoadArg("s_base", "X5").Raw("MOV (X5), X6").StoreRet("X6", "ret").Ret()
	f.Add(sliceFirst.Func())

	sig = abi.LayoutArgs([]abi.Arg{abi.String("x")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	strLen := riscv64.NewFunc("strLen", sig, 0)
	strLen.LoadArg("x_len", "X5").StoreRet("X5", "ret").Ret()
	f.Add(strLen.Func())

	if err := os.WriteFile("aggregate_riscv64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote aggregate_riscv64.s")
}
