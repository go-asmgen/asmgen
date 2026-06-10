//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/loong64"
)

func main() {
	f := emit.NewFile("loong64")

	sig := loong64.Layout(
		[]string{"a", "b", "out"}, []loong64.Type{loong64.Ptr, loong64.Ptr, loong64.Ptr},
		nil, nil,
	)
	b := loong64.NewFunc("addI32x4", sig, 0)
	b.LoadArg("a", "R4").
		LoadArg("b", "R5").
		LoadArg("out", "R6").
		Raw("VMOVQ (R4), V0"). // load 128-bit (4 int32)
		Raw("VMOVQ (R5), V1").
		Raw("VADDW V0, V1, V2"). // packed add, 32-bit lanes
		Raw("VMOVQ V2, (R6)").   // store 128-bit
		Ret()
	f.Add(b.Func())

	if err := os.WriteFile("simd_loong64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_loong64.s")
}
