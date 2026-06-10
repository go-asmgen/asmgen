//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/loong64"
)

func main() {
	sig := abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	b := loong64.NewFunc("sumI64", sig, 0)
	b.LoadArg("s_base", "R4").
		LoadArg("s_len", "R5").
		Raw("MOVV $0, R6").
		Raw("loop:").
		Raw("BEQ R5, R0, done").
		Raw("MOVV (R4), R7").
		Raw("ADDV R7, R6, R6").
		Raw("ADDV $8, R4, R4").
		Raw("ADDV $-1, R5, R5").
		Raw("JMP loop").
		Raw("done:").
		StoreRet("R6", "ret").
		Ret()
	f := emit.NewFile("loong64")
	f.Add(b.Func())
	if err := os.WriteFile("loop_loong64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote loop_loong64.s")
}
