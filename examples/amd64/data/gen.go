//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	sig := amd64.Layout(
		[]string{"in", "out"}, []amd64.Type{amd64.Ptr, amd64.Ptr},
		nil, nil,
	)
	f := emit.NewFile("amd64")
	// {10, 20, 30, 40} as four little-endian int32 — a read-only constant table.
	sym := f.Data("addend", []byte{
		0x0a, 0, 0, 0, 0x14, 0, 0, 0, 0x1e, 0, 0, 0, 0x28, 0, 0, 0,
	})

	b := amd64.NewFunc("addVec", sig, 0)
	b.LoadArg("in", "AX").
		LoadArg("out", "BX").
		Raw("MOVOU (AX), X0").
		Raw("MOVOU %s+0(SB), X1", sym). // load the constant table
		Raw("PADDL X1, X0").
		Raw("MOVOU X0, (BX)").
		Ret()
	f.Add(b.Func())

	if err := os.WriteFile("data_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote data_amd64.s")
}
