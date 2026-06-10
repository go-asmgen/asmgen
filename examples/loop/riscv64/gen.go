//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/riscv64"
)

func main() {
	sig := abi.LayoutArgs([]abi.Arg{abi.Slice("s")}, []abi.Arg{abi.Scalar("ret", abi.Int64)})
	b := riscv64.NewFunc("sumI64", sig, 0)
	b.LoadArg("s_base", "X5").
		LoadArg("s_len", "X6").
		Raw("MOV $0, X7").
		Raw("loop:").
		Raw("BEQZ X6, done").
		Raw("MOV (X5), X8").
		Raw("ADD X8, X7, X7").
		Raw("ADD $8, X5, X5").
		Raw("ADD $-1, X6, X6").
		Raw("JMP loop").
		Raw("done:").
		StoreRet("X7", "ret").
		Ret()
	f := emit.NewFile("riscv64")
	f.Add(b.Func())
	if err := os.WriteFile("loop_riscv64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote loop_riscv64.s")
}
