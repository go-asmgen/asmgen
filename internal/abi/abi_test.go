package abi

import "testing"

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
		if got := Align(c.n, c.to); got != c.want {
			t.Errorf("Align(%d,%d)=%d want %d", c.n, c.to, got, c.want)
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

// TestLayoutMixedSizeAlignment exercises Align inside Layout with mixed widths:
// a 1-byte arg, a 4-byte float padded to offset 4, an 8-byte int padded to 8,
// and the result area word-aligned to 16 with no final rounding.
func TestLayoutMixedSizeAlignment(t *testing.T) {
	sig := Layout(
		[]string{"flag", "f", "n"}, []Type{Int8, Float32, Int64},
		[]string{"ret"}, []Type{Int16},
	)
	wantArg := []int{0, 4, 8}
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

// TestLayoutWordAlignsResults pins the rule that trips people up: results start
// word-aligned even when all values are narrower than a word.
func TestLayoutWordAlignsResults(t *testing.T) {
	sig := Layout(
		[]string{"a", "b"}, []Type{Int16, Int16},
		[]string{"ret"}, []Type{Int16},
	)
	if sig.Args[1].Offset != 2 {
		t.Errorf("b offset = %d want 2", sig.Args[1].Offset)
	}
	if sig.Rets[0].Offset != 8 {
		t.Errorf("ret offset = %d want 8 (word-aligned)", sig.Rets[0].Offset)
	}
	if sig.ArgsSize != 10 {
		t.Errorf("ArgsSize = %d want 10", sig.ArgsSize)
	}
}
