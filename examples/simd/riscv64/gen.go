//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/riscv64"
)

func main() {
	f := emit.NewFile("riscv64")

	sig := riscv64.Layout(
		[]string{"a", "b", "out"}, []riscv64.Type{riscv64.Ptr, riscv64.Ptr, riscv64.Ptr},
		nil, nil,
	)
	b := riscv64.NewFunc("addI32x4", sig, 0)
	b.LoadArg("a", "X5").
		LoadArg("b", "X6").
		LoadArg("out", "X7").
		Raw("VSETVLI $4, E32, M1, TA, MA, X8"). // vl = 4 lanes of 32-bit
		Raw("VLE32V (X5), V0").                 // load 4 int32
		Raw("VLE32V (X6), V1").
		Raw("VADDVV V0, V1, V2"). // packed add
		Raw("VSE32V V2, (X7)").   // store 4 int32
		Ret()
	f.Add(b.Func())

	if err := os.WriteFile("simd_riscv64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_riscv64.s")
}
