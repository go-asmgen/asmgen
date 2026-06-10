package amd64

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
// the amd64-specific sign/zero-extending sub-word loads and the SSE float moves.
func TestMoveSelection(t *testing.T) {
	cases := []struct {
		t           Type
		load, store string
	}{
		{Int8, "MOVBQSX", "MOVB"},
		{Uint8, "MOVBQZX", "MOVB"},
		{Int16, "MOVWQSX", "MOVW"},
		{Uint16, "MOVWQZX", "MOVW"},
		{Int32, "MOVLQSX", "MOVL"},
		{Uint32, "MOVLQZX", "MOVL"},
		{Int64, "MOVQ", "MOVQ"},
		{Uint64, "MOVQ", "MOVQ"},
		{Ptr, "MOVQ", "MOVQ"},
		{Float32, "MOVSS", "MOVSS"},
		{Float64, "MOVSD", "MOVSD"},
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
	b.LoadArg("a", "AX").
		LoadArg("b", "BX").
		Raw("ADDQ BX, AX").
		StoreRet("AX", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"TEXT ·add(SB), NOSPLIT, $0-24",
		"\tMOVQ a+0(FP), AX",
		"\tMOVQ b+8(FP), BX",
		"\tADDQ BX, AX",
		"\tMOVQ AX, ret+16(FP)",
		"\tRET",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("emitted text missing line %q\nfull:\n%s", line, got)
		}
	}
}

// TestBuilderEmitsTypedMoves checks the SSE float move and a sub-word store plus
// the Raw escape hatch.
func TestBuilderEmitsTypedMoves(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []Type{Float32, Float32},
		[]string{"ret"}, []Type{Int16},
	)
	b := NewFunc("mix", sig, 0)
	b.LoadArg("a", "X0").
		LoadArg("b", "X1").
		Raw("%s X1, X0", "ADDSS").
		StoreRet("AX", "ret").
		Ret()

	got := b.Func().String()
	for _, line := range []string{
		"\tMOVSS a+0(FP), X0",
		"\tMOVSS b+4(FP), X1",
		"\tADDSS X1, X0",
		"\tMOVW AX, ret+8(FP)",
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
	b.LoadArg("nonexistent", "AX")
}

func TestLabel(t *testing.T) {
	got := NewFunc("f", Layout(nil, nil, nil, nil), 0).Label("loop").Ret().Func().String()
	if !strings.Contains(got, "\nloop:\n") {
		t.Errorf("label not emitted at column 0:\n%s", got)
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
	b.StoreRet("AX", "nope")
}
