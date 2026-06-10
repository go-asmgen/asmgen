//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	// func sumI64x3(v [3]int64) int64
	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Array("v", abi.Int64, 3)},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	b := arm64.NewFunc("sumI64x3", sig, 0)
	b.LoadArg("v_0", "R0").
		LoadArg("v_1", "R1").
		LoadArg("v_2", "R2").
		Raw("ADD R1, R0, R0").
		Raw("ADD R2, R0, R0").
		StoreRet("R0", "ret").
		Ret()

	f := emit.NewFile("arm64")
	f.Add(b.Func())
	if err := os.WriteFile("array_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote array_arm64.s")
}
