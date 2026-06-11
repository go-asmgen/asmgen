package ppc64

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
// the ppc64-specific choices: MOVD for 8-byte ints/ptrs; zero-extending
// MOVBZ/MOVHZ/MOVWZ for unsigned sub-word loads; FMOVS/FMOVD for floats.
func TestMoveSelection(t *testing.T) {
	cases := []struct {
		t           Type
		load, store string
	}{
		{Int8, "MOVB", "MOVB"},
		{Uint8, "MOVBZ", "MOVB"},
		{Int16, "MOVH", "MOVH"},
		{Uint16, "MOVHZ", "MOVH"},
		{Int32, "MOVW", "MOVW"},
		{Uint32, "MOVWZ", "MOVW"},
		{Int64, "MOVD", "MOVD"},
		{Uint64, "MOVD", "MOVD"},
		{Ptr, "MOVD", "MOVD"},
		{Float32, "FMOVS", "FMOVS"},
		{Float64, "FMOVD", "FMOVD"},
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
	b.LoadArg("a", "R3").
		LoadArg("b", "R4").
		Raw("ADD R3, R4, R5").
		StoreRet("R5", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"TEXT ·add(SB), NOSPLIT, $0-24",
		"\tMOVD a+0(FP), R3",
		"\tMOVD b+8(FP), R4",
		"\tADD R3, R4, R5",
		"\tMOVD R5, ret+16(FP)",
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
		Raw("%s F0, F1, F2", "FADDS").
		StoreRet("R3", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"\tFMOVS a+0(FP), F0",
		"\tFMOVS b+4(FP), F1",
		"\tFADDS F0, F1, F2",
		"\tMOVH R3, ret+8(FP)",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("emitted text missing line %q\nfull:\n%s", line, got)
		}
	}
}

// TestNewFuncFlags exercises the explicit-flags constructor (including the
// empty-flags branch that omits the flag field from the TEXT header).
func TestNewFuncFlags(t *testing.T) {
	sig := Layout(nil, nil, nil, nil)
	got := NewFuncFlags("f", sig, 16, "").Ret().Func().String()
	if !strings.Contains(got, "TEXT ·f(SB), $16-0\n") {
		t.Errorf("empty flags header wrong:\n%s", got)
	}
	got = NewFuncFlags("g", sig, 0, "NOSPLIT|NOFRAME").Ret().Func().String()
	if !strings.Contains(got, "TEXT ·g(SB), NOSPLIT|NOFRAME, $0-0\n") {
		t.Errorf("explicit flags header wrong:\n%s", got)
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
	b.LoadArg("nonexistent", "R3")
}

func TestLabel(t *testing.T) {
	got := NewFunc("f", Layout(nil, nil, nil, nil), 0).Label("loop").Ret().Func().String()
	if !strings.Contains(got, "\nloop:\n") {
		t.Errorf("label not emitted at column 0:\n%s", got)
	}
}

func TestIndirect(t *testing.T) {
	got := NewFunc("f", Layout(nil, nil, nil, nil), 0).
		LoadIndirect("R3", Int64, "R4").  // MOVD (R3), R4
		StoreIndirect("R4", "R3", Uint8). // MOVB R4, (R3)
		Ret().Func().String()
	for _, line := range []string{"\tMOVD (R3), R4\n", "\tMOVB R4, (R3)\n"} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %q\n%s", line, got)
		}
	}
}

func TestStoreRetPanicsOnUnknownRet(t *testing.T) {
	sig := Layout([]string{"a"}, []Type{Int64}, []string{"ret"}, []Type{Int64})
	b := NewFunc("f", sig, 0)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on unknown ret, got none")
		}
	}()
	b.StoreRet("R3", "nope")
}
