//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	sig := abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	b := amd64.NewFunc("sumI64", sig, 0)
	b.LoadArg("s_base", "AX").
		LoadArg("s_len", "BX").
		Raw("MOVQ $0, CX").
		Label("loop").
		Raw("TESTQ BX, BX").
		Raw("JZ done").
		LoadIndirect("AX", amd64.Int64, "DX").
		Raw("ADDQ DX, CX").
		Raw("ADDQ $8, AX").
		Raw("DECQ BX").
		Raw("JMP loop").
		Label("done").
		StoreRet("CX", "ret").
		Ret()
	f := emit.NewFile("amd64")
	f.Add(b.Func())
	if err := os.WriteFile("loop_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote loop_amd64.s")
}
