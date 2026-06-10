//go:build ignore

// Command gen produces simd_arm64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	f := emit.NewFile("arm64")

	// func addI32x4(a, b, out *[4]int32) — NEON packed 32-bit add (4 lanes).
	sig := arm64.Layout(
		[]string{"a", "b", "out"}, []arm64.Type{arm64.Ptr, arm64.Ptr, arm64.Ptr},
		nil, nil,
	)
	b := arm64.NewFunc("addI32x4", sig, 0)
	b.LoadArg("a", "R0").
		LoadArg("b", "R1").
		LoadArg("out", "R2").
		Raw("VLD1 (R0), [V0.S4]"). // load 4 int32 lanes
		Raw("VLD1 (R1), [V1.S4]").
		Raw("VADD V0.S4, V1.S4, V2.S4"). // packed add, 32-bit lanes
		Raw("VST1 [V2.S4], (R2)").       // store 4 lanes
		Ret()
	f.Add(b.Func())

	if err := os.WriteFile("simd_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_arm64.s")
}
