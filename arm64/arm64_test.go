package arm64

import (
	"strings"
	"testing"
)

func TestAlign(t *testing.T) {
	cases := []struct {
		n, to, want int
	}{
		{0, 8, 0},
		{1, 8, 8},
		{8, 8, 8},
		{9, 8, 16},
		{3, 4, 4},
		{4, 4, 4},
		{5, 1, 5}, // align-to-1 is identity
	}
	for _, c := range cases {
		if got := align(c.n, c.to); got != c.want {
			t.Errorf("align(%d,%d)=%d want %d", c.n, c.to, got, c.want)
		}
	}
}

func TestLayoutEightByteSequence(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []Type{Int64, Int64},
		[]string{"ret"}, []Type{Int64},
	)
	if len(sig.Args) != 2 || len(sig.Rets) != 1 {
		t.Fatalf("unexpected param counts: %+v", sig)
	}
	if sig.Args[0].Offset != 0 || sig.Args[1].Offset != 8 {
		t.Errorf("arg offsets = %d,%d want 0,8", sig.Args[0].Offset, sig.Args[1].Offset)
	}
	if sig.Rets[0].Offset != 16 {
		t.Errorf("ret offset = %d want 16", sig.Rets[0].Offset)
	}
	if sig.ArgsSize != 24 {
		t.Errorf("ArgsSize = %d want 24", sig.ArgsSize)
	}
}

// TestLayoutMixedSizeAlignment exercises the align() branch inside Layout with a
// mix of widths: a 1-byte arg, then a 4-byte float (padded to offset 4), then an
// 8-byte int (padded to offset 8), with a 2-byte result rounded into a word.
func TestLayoutMixedSizeAlignment(t *testing.T) {
	sig := Layout(
		[]string{"flag", "f", "n"}, []Type{Int8, Float32, Int64},
		[]string{"ret"}, []Type{Int16},
	)
	wantArg := []int{0, 4, 8} // flag@0, f padded 1->4, n padded 5->8
	for i, w := range wantArg {
		if sig.Args[i].Offset != w {
			t.Errorf("arg %q offset = %d want %d", sig.Args[i].Name, sig.Args[i].Offset, w)
		}
	}
	if sig.Rets[0].Offset != 16 { // args end at 16 (already word-aligned)
		t.Errorf("ret offset = %d want 16", sig.Rets[0].Offset)
	}
	if sig.ArgsSize != 18 { // ret int16 at 16 + 2; no final rounding
		t.Errorf("ArgsSize = %d want 18", sig.ArgsSize)
	}
}

// TestMoveSelection covers every branch of loadMnemonic and storeMnemonic.
func TestMoveSelection(t *testing.T) {
	cases := []struct {
		t          Type
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
