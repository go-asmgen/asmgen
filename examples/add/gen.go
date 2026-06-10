//go:build ignore

// Command gen produces add_arm64.s. Run with: go run gen.go
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	// func add(a, b int64) int64
	sig := arm64.Layout(
		[]string{"a", "b"}, []arm64.Type{arm64.Int64, arm64.Int64},
		[]string{"ret"}, []arm64.Type{arm64.Int64},
	)

	b := arm64.NewFunc("add", sig, 0)
	b.LoadArg("a", "R0").
		LoadArg("b", "R1").
		Raw("ADD R1, R0, R2").
		StoreRet("R2", "ret").
		Ret()

	f := emit.NewFile("arm64")
	f.Add(b.Func())

	if err := os.WriteFile("add_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote add_arm64.s")
}
