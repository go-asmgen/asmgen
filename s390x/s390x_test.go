package s390x

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
// the s390x-specific choices: MOVD for 8-byte ints/pointers, the Z-suffixed
// zero-extending forms (MOVBZ/MOVHZ/MOVWZ) for unsigned sub-word loads, and
// FMOVS/FMOVD for floats.
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
	b.LoadArg("a", "R1").
		LoadArg("b", "R2").
		Raw("ADD R1, R2, R3").
		StoreRet("R3", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"TEXT ·add(SB), NOSPLIT, $0-24",
		"\tMOVD a+0(FP), R1",
		"\tMOVD b+8(FP), R2",
		"\tADD R1, R2, R3",
		"\tMOVD R3, ret+16(FP)",
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
		Raw("%s F1, F0", "FADDS").
		StoreRet("R1", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"\tFMOVS a+0(FP), F0",
		"\tFMOVS b+4(FP), F1",
		"\tFADDS F1, F0",
		"\tMOVH R1, ret+8(FP)",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("emitted text missing line %q\nfull:\n%s", line, got)
		}
	}
}

// TestNewFuncFlags exercises the explicit-flags constructor (including the
// empty-flags path, which omits the flags field from the TEXT header).
func TestNewFuncFlags(t *testing.T) {
	sig := Layout([]string{"a"}, []Type{Int64}, nil, nil)
	got := NewFuncFlags("f", sig, 0, "").Ret().Func().String()
	if !strings.Contains(got, "TEXT ·f(SB), $0-8\n") {
		t.Errorf("empty flags not handled:\n%s", got)
	}
	got2 := NewFuncFlags("g", sig, 16, "NOSPLIT|NOFRAME").Ret().Func().String()
	if !strings.Contains(got2, "TEXT ·g(SB), NOSPLIT|NOFRAME, $16-8\n") {
		t.Errorf("explicit flags not handled:\n%s", got2)
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
	b.LoadArg("nonexistent", "R1")
}

func TestLabel(t *testing.T) {
	got := NewFunc("f", Layout(nil, nil, nil, nil), 0).Label("loop").Ret().Func().String()
	if !strings.Contains(got, "\nloop:\n") {
		t.Errorf("label not emitted at column 0:\n%s", got)
	}
}

func TestIndirect(t *testing.T) {
	got := NewFunc("f", Layout(nil, nil, nil, nil), 0).
		LoadIndirect("R2", Int64, "R3").  // MOVD (R2), R3
		StoreIndirect("R3", "R2", Uint8). // MOVB R3, (R2)
		Ret().Func().String()
	for _, line := range []string{"\tMOVD (R2), R3\n", "\tMOVB R3, (R2)\n"} {
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
	b.StoreRet("R1", "nope")
}

// TestRawVectorBody exercises the SIMD escape hatch with the s390x vector
// facility operand order (sources-then-destination): a 16-byte vector XOR.
func TestRawVectorBody(t *testing.T) {
	sig := Layout(
		[]string{"a", "b", "out"}, []Type{Ptr, Ptr, Ptr},
		nil, nil,
	)
	b := NewFunc("xor16", sig, 0)
	b.LoadArg("a", "R1").
		LoadArg("b", "R2").
		LoadArg("out", "R3").
		Raw("VL (R1), V0").
		Raw("VL (R2), V1").
		Raw("VX V0, V1, V2").
		Raw("VST V2, (R3)").
		Ret()
	got := b.Func().String()
	for _, line := range []string{
		"\tMOVD a+0(FP), R1\n",
		"\tMOVD b+8(FP), R2\n",
		"\tMOVD out+16(FP), R3\n",
		"\tVL (R1), V0\n",
		"\tVL (R2), V1\n",
		"\tVX V0, V1, V2\n",
		"\tVST V2, (R3)\n",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %q\n%s", line, got)
		}
	}
}
