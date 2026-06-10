//go:build ignore

// Command gen produces array_loong64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/loong64"
)

func main() {
	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Array("v", abi.Int64, 3)},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	b := loong64.NewFunc("sumI64x3", sig, 0)
	b.LoadArg("v_0", "R4").
		LoadArg("v_1", "R5").
		LoadArg("v_2", "R6").
		Raw("ADDV R5, R4, R4").
		Raw("ADDV R6, R4, R4").
		StoreRet("R4", "ret").
		Ret()

	f := emit.NewFile("loong64")
	f.Add(b.Func())
	if err := os.WriteFile("array_loong64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote array_loong64.s")
}
