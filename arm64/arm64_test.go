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
		[]string{"a", "b"}, []int{8, 8},
		[]string{"ret"}, []int{8},
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

// TestLayoutAlignmentPadding exercises the align() branch inside Layout: a
// 1-byte arg followed by an 8-byte arg must pad the second to offset 8, and the
// whole region rounds up to a word boundary.
func TestLayoutAlignmentPadding(t *testing.T) {
	sig := Layout(
		[]string{"flag", "val"}, []int{1, 8},
		[]string{"ret"}, []int{4},
	)
	if sig.Args[0].Offset != 0 {
		t.Errorf("flag offset = %d want 0", sig.Args[0].Offset)
	}
	if sig.Args[1].Offset != 8 { // 1 -> padded up to 8
		t.Errorf("val offset = %d want 8", sig.Args[1].Offset)
	}
	if sig.Rets[0].Offset != 16 { // after val (8+8=16), 4-aligned already
		t.Errorf("ret offset = %d want 16", sig.Rets[0].Offset)
	}
	if sig.ArgsSize != 24 { // 16+4=20 -> word-aligned to 24
		t.Errorf("ArgsSize = %d want 24", sig.ArgsSize)
	}
}

func TestBuilderEmitsExpectedText(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []int{8, 8},
		[]string{"ret"}, []int{8},
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

func TestLookupPanicsOnUnknownArg(t *testing.T) {
	sig := Layout([]string{"a"}, []int{8}, nil, nil)
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
	sig := Layout([]string{"a"}, []int{8}, []string{"ret"}, []int{8})
	b := NewFunc("f", sig, 0)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on unknown ret, got none")
		}
	}()
	b.StoreRet("R0", "nope")
}
