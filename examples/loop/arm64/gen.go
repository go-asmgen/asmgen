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
	sig := abi.LayoutArgs(
		[]abi.Arg{abi.Slice("s")},
		[]abi.Arg{abi.Scalar("ret", abi.Int64)},
	)
	b := arm64.NewFunc("sumI64", sig, 0)
	b.LoadArg("s_base", "R0"). // ptr
					LoadArg("s_len", "R1"). // count
					Raw("MOVD $0, R2").     // acc = 0
					Raw("loop:").
					Raw("CBZ R1, done").  // if count == 0, done
					Raw("MOVD (R0), R3"). // *ptr
					Raw("ADD R3, R2, R2").
					Raw("ADD $8, R0, R0"). // ptr++
					Raw("SUB $1, R1, R1"). // count--
					Raw("JMP loop").
					Raw("done:").
					StoreRet("R2", "ret").
					Ret()

	f := emit.NewFile("arm64")
	f.Add(b.Func())
	if err := os.WriteFile("loop_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote loop_arm64.s")
}
