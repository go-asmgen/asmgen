//go:build ignore

// Command gen produces array_riscv64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/riscv64"
)

func main() {
	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Array("v", abi.Int64, 3)},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	b := riscv64.NewFunc("sumI64x3", sig, 0)
	b.LoadArg("v_0", "X5").
		LoadArg("v_1", "X6").
		LoadArg("v_2", "X7").
		Raw("ADD X6, X5, X5").
		Raw("ADD X7, X5, X5").
		StoreRet("X5", "ret").
		Ret()

	f := emit.NewFile("riscv64")
	f.Add(b.Func())
	if err := os.WriteFile("array_riscv64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote array_riscv64.s")
}
