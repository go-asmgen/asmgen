package arm64

import (
	"strings"
	"testing"
)

// TestLayoutWrapper checks the arm64.Layout wrapper delegates to the shared
// abi layout (offsets are verified exhaustively in abi).
func TestLayoutWrapper(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []Type{Int64, Int64},
		[]string{"ret"}, []Type{Int64},
	)
	if sig.Args[1].Offset != 8 || sig.Rets[0].Offset != 16 || sig.ArgsSize != 24 {
		t.Errorf("unexpected layout: %+v", sig)
	}
}

// TestNewFuncFlags checks the TEXT directive honours explicit flags, including
// the empty-flags case (no NOSPLIT) used for larger frames or non-leaf functions.
func TestNewFuncFlags(t *testing.T) {
	sig := Layout([]string{"a"}, []Type{Int64}, []string{"ret"}, []Type{Int64})

	noSplit := NewFuncFlags("f", sig, 16, "").Func().String()
	if !strings.Contains(noSplit, "TEXT ·f(SB), $16-16\n") {
		t.Errorf("empty flags: got header\n%s", noSplit)
	}

	noFrame := NewFuncFlags("g", sig, 0, "NOSPLIT|NOFRAME").Func().String()
	if !strings.Contains(noFrame, "TEXT ·g(SB), NOSPLIT|NOFRAME, $0-16\n") {
		t.Errorf("NOSPLIT|NOFRAME: got header\n%s", noFrame)
	}
}

// TestMoveSelection covers every branch of loadMnemonic and storeMnemonic.
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
	b.LoadArg("a", "R0").
		LoadArg("b", "R1").
		Raw("ADD R1, R0, R2").
		StoreRet("R2", "ret").
		Ret()

	got := b.Func().String()
	wantLines := []string{
		"TEXT ·add(SB), NOSPLIT, $0-24",
		"\tMOVD a+0(FP), R0",
		"\tMOVD b+8(FP), R1",
		"\tADD R1, R0, R2",
		"\tMOVD R2, ret+16(FP)",
		"\tRET",
	}
	for _, line := range wantLines {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("emitted text missing line %q\nfull:\n%s", line, got)
		}
	}
}

// TestBuilderEmitsTypedMoves checks that the builder picks the float move and a
// sub-word store from the parameter types, including the Raw format escape hatch.
func TestBuilderEmitsTypedMoves(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []Type{Float32, Float32},
		[]string{"ret"}, []Type{Int16},
	)
	b := NewFunc("mix", sig, 0)
	b.LoadArg("a", "F0").
		LoadArg("b", "F1").
		Raw("%s F1, F0, F2", "FADDS").
		StoreRet("R0", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"\tFMOVS a+0(FP), F0",
		"\tFMOVS b+4(FP), F1",
		"\tFADDS F1, F0, F2",
		"\tMOVH R0, ret+8(FP)",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("emitted text missing line %q\nfull:\n%s", line, got)
		}
	}
}

func TestLookupPanicsOnUnknownArg(t *testing.T) {
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
	b.LoadArg("nonexistent", "R0")
}

func TestLabel(t *testing.T) {
	got := NewFunc("f", Layout(nil, nil, nil, nil), 0).Label("loop").Ret().Func().String()
	if !strings.Contains(got, "\nloop:\n") {
		t.Errorf("label not emitted at column 0:\n%s", got)
	}
}

func TestIndirect(t *testing.T) {
	got := NewFunc("f", Layout(nil, nil, nil, nil), 0).
		LoadIndirect("R0", Int64, "R1").  // MOVD (R0), R1
		StoreIndirect("R1", "R0", Uint8). // MOVB R1, (R0)
		Ret().Func().String()
	for _, line := range []string{"\tMOVD (R0), R1\n", "\tMOVB R1, (R0)\n"} {
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
	b.StoreRet("R0", "nope")
}
