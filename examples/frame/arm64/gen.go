//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	sig := arm64.Layout(
		[]string{"a", "b"}, []arm64.Type{arm64.Int64, arm64.Int64},
		[]string{"ret"}, []arm64.Type{arm64.Int64},
	)
	// 16-byte frame (two int64 locals, addressed name-N(SP)); "" flags == no
	// NOSPLIT, so the assembler inserts the stack-growth preamble.
	b := arm64.NewFuncFlags("spillSum", sig, 16, "")
	b.LoadArg("a", "R0").
		Raw("MOVD R0, s0-8(SP)"). // spill a to local 0
		LoadArg("b", "R1").
		Raw("MOVD R1, s1-16(SP)"). // spill b to local 1
		Raw("MOVD s0-8(SP), R2").  // reload
		Raw("MOVD s1-16(SP), R3").
		Raw("ADD R3, R2, R2").
		StoreRet("R2", "ret").
		Ret()

	f := emit.NewFile("arm64")
	f.Add(b.Func())
	if err := os.WriteFile("frame_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote frame_arm64.s")
}
