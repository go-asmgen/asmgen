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

// byName indexes Params by name for offset assertions.
func byName(ps []Param) map[string]int {
	m := make(map[string]int, len(ps))
	for _, p := range ps {
		m[p.Name] = p.Offset
	}
	return m
}

func TestLayoutArgsStruct(t *testing.T) {
	// func f(p struct{A, B int64}) int64
	sig := LayoutArgs(
		[]Arg{Struct("p", Field{"A", Int64}, Field{"B", Int64})},
		[]Arg{Scalar("ret", Int64)},
	)
	off := byName(sig.Args)
	if off["p_A"] != 0 || off["p_B"] != 8 {
		t.Errorf("struct field offsets = %v want p_A@0 p_B@8", off)
	}
	if sig.Rets[0].Offset != 16 || sig.ArgsSize != 24 {
		t.Errorf("ret@%d size %d want ret@16 size 24", sig.Rets[0].Offset, sig.ArgsSize)
	}
}

func TestLayoutArgsStructPadding(t *testing.T) {
	// func f(m struct{Flag int8; N int64}) int64 — N padded to 8, struct size 16.
	sig := LayoutArgs(
		[]Arg{Struct("m", Field{"Flag", Int8}, Field{"N", Int64})},
		[]Arg{Scalar("ret", Int64)},
	)
	off := byName(sig.Args)
	if off["m_Flag"] != 0 || off["m_N"] != 8 {
		t.Errorf("padded struct offsets = %v want m_Flag@0 m_N@8", off)
	}
	if sig.Rets[0].Offset != 16 { // struct occupies [0,16)
		t.Errorf("ret offset = %d want 16", sig.Rets[0].Offset)
	}
}

func TestLayoutArgsSlice(t *testing.T) {
	// func f(s []int64) int64
	sig := LayoutArgs([]Arg{Slice("s")}, []Arg{Scalar("ret", Int64)})
	off := byName(sig.Args)
	if off["s_base"] != 0 || off["s_len"] != 8 || off["s_cap"] != 16 {
		t.Errorf("slice header offsets = %v want base@0 len@8 cap@16", off)
	}
	if sig.Rets[0].Offset != 24 || sig.ArgsSize != 32 {
		t.Errorf("ret@%d size %d want ret@24 size 32", sig.Rets[0].Offset, sig.ArgsSize)
	}
}

func TestLayoutArgsString(t *testing.T) {
	// func f(x string) int
	sig := LayoutArgs([]Arg{String("x")}, []Arg{Scalar("ret", Int64)})
	off := byName(sig.Args)
	if off["x_base"] != 0 || off["x_len"] != 8 {
		t.Errorf("string header offsets = %v want base@0 len@8", off)
	}
	if sig.Rets[0].Offset != 16 {
		t.Errorf("ret offset = %d want 16", sig.Rets[0].Offset)
	}
}

func TestLayoutArgsArray(t *testing.T) {
	// func f(a, b [4]int32) [4]int32 — each array is 16 bytes, 4-aligned.
	sig := LayoutArgs(
		[]Arg{Array("a", Int32, 4), Array("b", Int32, 4)},
		[]Arg{Array("ret", Int32, 4)},
	)
	off := byName(sig.Args)
	if off["a_0"] != 0 || off["a_3"] != 12 || off["b_0"] != 16 || off["b_3"] != 28 {
		t.Errorf("array element offsets = %v", off)
	}
	if sig.Rets[0].Name != "ret_0" || sig.Rets[0].Offset != 32 {
		t.Errorf("ret_0 = %+v want offset 32", sig.Rets[0])
	}
	if sig.ArgsSize != 48 { // 16+16 args, ret 16 at 32..48
		t.Errorf("ArgsSize = %d want 48", sig.ArgsSize)
	}
}

func TestSlot(t *testing.T) {
	sig := LayoutArgs(
		[]Arg{Scalar("n", Int64), Array("v", Int64, 3)},
		[]Arg{Scalar("ret", Int64)},
	)
	// v's whole value begins at its element 0; n is 8 bytes, so v_0 is at 8.
	if p, ok := sig.Slot("v_0"); !ok || p.Offset != 8 {
		t.Errorf("Slot(v_0) = %+v,%v want offset 8", p, ok)
	}
	if p, ok := sig.Slot("ret"); !ok || p.Offset != 32 { // n(8)+v(24)=32
		t.Errorf("Slot(ret) = %+v,%v want offset 32", p, ok)
	}
	if _, ok := sig.Slot("nope"); ok {
		t.Error("Slot(nope) found unexpectedly")
	}
}

func TestLayoutArgsScalarThenAggregate(t *testing.T) {
	// func f(n int32, s []int64) int64 — slice region aligned to 8 after n.
	sig := LayoutArgs(
		[]Arg{Scalar("n", Int32), Slice("s")},
		[]Arg{Scalar("ret", Int64)},
	)
	off := byName(sig.Args)
	if off["n"] != 0 || off["s_base"] != 8 || off["s_len"] != 16 || off["s_cap"] != 24 {
		t.Errorf("offsets = %v want n@0 s_base@8 s_len@16 s_cap@24", off)
	}
	if sig.Rets[0].Offset != 32 || sig.ArgsSize != 40 {
		t.Errorf("ret@%d size %d want ret@32 size 40", sig.Rets[0].Offset, sig.ArgsSize)
	}
}
