//go:build ignore

// Command gen produces array_amd64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	// func sumI64x3(v [3]int64) int64
	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Array("v", abi.Int64, 3)},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	b := amd64.NewFunc("sumI64x3", sig, 0)
	b.LoadArg("v_0", "AX").
		LoadArg("v_1", "BX").
		LoadArg("v_2", "CX").
		Raw("ADDQ BX, AX").
		Raw("ADDQ CX, AX").
		StoreRet("AX", "ret").
		Ret()

	f := emit.NewFile("amd64")
	f.Add(b.Func())
	if err := os.WriteFile("array_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote array_amd64.s")
}
