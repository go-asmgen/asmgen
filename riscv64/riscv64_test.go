package riscv64

import (
	"strings"
	"testing"
)

func TestLayoutWrapper(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []Type{Int64, Int64},
		[]string{"ret"}, []Type{Int64},
	)
	if sig.Args[1].Offset != 8 || sig.Rets[0].Offset != 16 || sig.ArgsSize != 24 {
		t.Errorf("unexpected layout: %+v", sig)
	}
}

// TestMoveSelection covers every branch of loadMnemonic and storeMnemonic. Note
// the riscv64-specific choices: MOV (not MOVD) for 8-byte ints, and MOVF/MOVD
// for floats.
func TestMoveSelection(t *testing.T) {
	cases := []struct {
		t           Type
		load, store string
	}{
		{Int8, "MOVB", "MOVB"},
		{Uint8, "MOVBU", "MOVB"},
		{Int16, "MOVH", "MOVH"},
		{Uint16, "MOVHU", "MOVH"},
		{Int32, "MOVW", "MOVW"},
		{Uint32, "MOVWU", "MOVW"},
		{Int64, "MOV", "MOV"},
		{Uint64, "MOV", "MOV"},
		{Ptr, "MOV", "MOV"},
		{Float32, "MOVF", "MOVF"},
		{Float64, "MOVD", "MOVD"},
	}
	for _, c := range cases {
		if got := loadMnemonic(c.t); got != c.load {
			t.Errorf("loadMnemonic(%+v)=%s want %s", c.t, got, c.load)
		}
		if got := storeMnemonic(c.t); got != c.store {
			t.Errorf("storeMnemonic(%+v)=%s want %s", c.t, got, c.store)
		}
	}
}

func TestBuilderEmitsExpectedText(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []Type{Int64, Int64},
		[]string{"ret"}, []Type{Int64},
	)
	b := NewFunc("add", sig, 0)
	b.LoadArg("a", "X5").
		LoadArg("b", "X6").
		Raw("ADD X6, X5, X7").
		StoreRet("X7", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"TEXT ·add(SB), NOSPLIT, $0-24",
		"\tMOV a+0(FP), X5",
		"\tMOV b+8(FP), X6",
		"\tADD X6, X5, X7",
		"\tMOV X7, ret+16(FP)",
		"\tRET",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("emitted text missing line %q\nfull:\n%s", line, got)
		}
	}
}

// TestBuilderEmitsTypedMoves checks float and sub-word move selection plus the
// Raw format escape hatch.
func TestBuilderEmitsTypedMoves(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []Type{Float32, Float32},
		[]string{"ret"}, []Type{Int16},
	)
	b := NewFunc("mix", sig, 0)
	b.LoadArg("a", "F0").
		LoadArg("b", "F1").
		Raw("%s F1, F0, F2", "FADDS").
		StoreRet("X5", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"\tMOVF a+0(FP), F0",
		"\tMOVF b+4(FP), F1",
		"\tFADDS F1, F0, F2",
		"\tMOVH X5, ret+8(FP)",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("emitted text missing line %q\nfull:\n%s", line, got)
		}
	}
}

func TestLoadArgPanicsOnUnknownArg(t *testing.T) {
	sig := Layout([]string{"a"}, []Type{Int64}, nil, nil)
	b := NewFunc("f", sig, 0)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on unknown arg, got none")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "unknown param") {
			t.Errorf("panic = %v want message containing \"unknown param\"", r)
		}
	}()
	b.LoadArg("nonexistent", "X5")
}

func TestStoreRetPanicsOnUnknownRet(t *testing.T) {
	sig := Layout([]string{"a"}, []Type{Int64}, []string{"ret"}, []Type{Int64})
	b := NewFunc("f", sig, 0)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on unknown ret, got none")
		}
	}()
	b.StoreRet("X5", "nope")
}
