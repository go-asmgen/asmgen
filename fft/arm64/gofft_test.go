package arm64

import (
	"fmt"
	"testing"

	"github.com/go-asmgen/asmgen/emit"
)

// golden are the representative kernels whose text is checked in under
// testdata: every builder, forward and inverse, the radices that reach each
// of its paths.
var golden = map[string]func(f *emit.File){
	"cmul":       func(f *emit.File) { CMulNEON(f, "cmul") },
	"pass2":      func(f *emit.File) { Pass(f, "pass2", 2, false) },
	"pass5_inv":  func(f *emit.File) { Pass(f, "pass5", 5, true) },
	"last3":      func(f *emit.File) { Last(f, "last3", 3, false) },
	"last8_inv":  func(f *emit.File) { Last(f, "last8", 8, true) },
	"batch4":     func(f *emit.File) { Batch(f, "batch4", 4, false) },
	"split2_is":  func(f *emit.File) { PassSplit(f, "pass2IS", 2, false, false) },
	"split3_ss":  func(f *emit.File) { PassSplit(f, "pass3SS", 3, true, true) },
	"lastsplit4": func(f *emit.File) { LastSplit(f, "last4SI", 4, false) },
	"pass32_8":   func(f *emit.File) { Pass32(f, "pass8", 8, false) },
	"last32_2":   func(f *emit.File) { Last32(f, "last2", 2, true) },
	"batch32_3":  func(f *emit.File) { Batch32(f, "batch3", 3, false) },
	"untangle32": func(f *emit.File) { Untangle32(f, "untangle", false); Untangle32(f, "retangle", true) },
	"untangle":   func(f *emit.File) { Untangle(f, "untangle", false); Untangle(f, "retangle", true) },
}

var radices = []int{2, 3, 4, 5, 8}

func inv(inverse bool) string {
	if inverse {
		return "Inv"
	}
	return ""
}

// goFFTFiles returns go-fft's arm64 files by name, their text as committed
// (internal/kernels/asmgen/arm64/gen.go).
func goFFTFiles() map[string]string {
	files := map[string]string{}
	f := emit.NewFile("arm64")
	CMulNEON(f, "cmulNEON")
	files["cmul_arm64.s"] = f.String()

	f = emit.NewFile("arm64")
	for _, inverse := range []bool{false, true} {
		for _, r := range radices {
			Pass(f, fmt.Sprintf("skPass%dNEON%s", r, inv(inverse)), r, inverse)
		}
		for _, r := range radices {
			Last(f, fmt.Sprintf("skLast%dNEON%s", r, inv(inverse)), r, inverse)
		}
	}
	for _, inverse := range []bool{false, true} {
		for _, r := range radices {
			Batch(f, fmt.Sprintf("skBatch%dNEON%s", r, inv(inverse)), r, inverse)
		}
	}
	for _, inverse := range []bool{false, true} {
		for _, inSplit := range []bool{false, true} {
			lay := map[bool]string{false: "IS", true: "SS"}[inSplit]
			for _, r := range radices {
				PassSplit(f, fmt.Sprintf("skPass%dNEON%s%s", r, inv(inverse), lay), r, inverse, inSplit)
			}
		}
		for _, r := range []int{2, 4, 8} {
			LastSplit(f, fmt.Sprintf("skLast%dNEON%sSI", r, inv(inverse)), r, inverse)
		}
	}
	files["stockham_arm64.s"] = f.String()

	f = emit.NewFile("arm64")
	for _, inverse := range []bool{false, true} {
		for _, r := range radices {
			Pass32(f, fmt.Sprintf("sk32Pass%dNEON%s", r, inv(inverse)), r, inverse)
		}
		for _, r := range radices {
			Last32(f, fmt.Sprintf("sk32Last%dNEON%s", r, inv(inverse)), r, inverse)
		}
	}
	files["stockham32_arm64.s"] = f.String()

	f = emit.NewFile("arm64")
	Untangle32(f, "f32rUntangleNEON", false)
	Untangle32(f, "f32rRetangleNEON", true)
	files["untangle32_arm64.s"] = f.String()

	f = emit.NewFile("arm64")
	for _, inverse := range []bool{false, true} {
		for _, r := range radices {
			Batch32(f, fmt.Sprintf("sk32Batch%dNEON%s", r, inv(inverse)), r, inverse)
		}
	}
	files["batch32_arm64.s"] = f.String()

	f = emit.NewFile("arm64")
	Untangle(f, "armrealUntangleNEON", false)
	Untangle(f, "armrealRetangleNEON", true)
	files["untangle_arm64.s"] = f.String()
	return files
}

func TestRadixRefused(t *testing.T) {
	for _, f := range []func(*emit.File){
		func(f *emit.File) { Pass(f, "x", 16, false) },
		func(f *emit.File) { Last(f, "x", 9, false) },
		func(f *emit.File) { Batch(f, "x", 7, true) },
		func(f *emit.File) { PassSplit(f, "x", 6, false, true) },
		func(f *emit.File) { LastSplit(f, "x", 3, false) },
		func(f *emit.File) { Pass32(f, "x", 12, false) },
		func(f *emit.File) { Last32(f, "x", 10, false) },
		func(f *emit.File) { Batch32(f, "x", 16, false) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("an unsupported radix was built instead of refused")
				}
			}()
			f(emit.NewFile("arm64"))
		}()
	}
}
