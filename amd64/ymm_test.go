package amd64

import (
	"strings"
	"testing"
)

func ymmBuilder() *Builder {
	return NewFunc("k", Layout(nil, nil, nil, nil), 0)
}

func insns(b *Builder) []string {
	var out []string
	for _, l := range strings.Split(b.Func().String(), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "TEXT") && !strings.HasPrefix(l, "//") {
			out = append(out, l)
		}
	}
	return out
}

func TestYRegString(t *testing.T) {
	if Y0.String() != "Y0" || Y15.String() != "Y15" {
		t.Fatalf("got %s %s", Y0, Y15)
	}
	mustPanic(t, "invalid YMM", func() { _ = YReg(16).String() })
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if s, _ := r.(string); !strings.Contains(s, want) {
			t.Fatalf("panic %v, want %q", r, want)
		}
	}()
	f()
}

// TestYmmOperandOrder pins the Intel-to-Plan 9 operand reversal on every
// three-operand form: the destination is last, the second source first.
func TestYmmOperandOrder(t *testing.T) {
	b := ymmBuilder()
	b.VPXOR(Y0, Y1, Y2).VPAND(Y3, Y4, Y5).VPOR(Y6, Y7, Y8).VPANDN(Y9, Y10, Y11).
		VPADDD(Y12, Y13, Y14).VPCMPEQD(Y15, Y0, Y1).Ones(Y2)
	want := []string{
		"VPXOR Y2, Y1, Y0",
		"VPAND Y5, Y4, Y3",
		"VPOR Y8, Y7, Y6",
		"VPANDN Y11, Y10, Y9",
		"VPADDD Y14, Y13, Y12",
		"VPCMPEQD Y1, Y0, Y15",
		"VPCMPEQD Y2, Y2, Y2",
	}
	checkInsns(t, b, want)
}

func checkInsns(t *testing.T, b *Builder, want []string) {
	t.Helper()
	got := insns(b)
	if len(got) != len(want) {
		t.Fatalf("got %d instructions %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("insn %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestYmmShiftsRotatesMoves(t *testing.T) {
	b := ymmBuilder()
	b.VPSLLD(Y1, Y0, 3).VPSRLD(Y2, Y0, 31).RotL32(Y0, Y0, 13, Y5).
		VMOVDQA(Y3, Y4).VMOVDQU(Y6, "32(SI)").VMOVDQUStore("(DI)", Y7).
		VPBROADCASTD(Y8, "16(DX)").VZEROUPPER()
	checkInsns(t, b, []string{
		"VPSLLD $3, Y0, Y1",
		"VPSRLD $31, Y0, Y2",
		"VPSLLD $13, Y0, Y5",
		"VPSRLD $19, Y0, Y0",
		"VPOR Y5, Y0, Y0",
		"VMOVDQA Y4, Y3",
		"VMOVDQU 32(SI), Y6",
		"VMOVDQU Y7, (DI)",
		"VPBROADCASTD 16(DX), Y8",
		"VZEROUPPER",
	})
}

func TestYmmRefusals(t *testing.T) {
	b := ymmBuilder()
	mustPanic(t, "out of 0..31", func() { b.VPSLLD(Y0, Y0, 32) })
	mustPanic(t, "out of 0..31", func() { b.VPSRLD(Y0, Y0, -1) })
	mustPanic(t, "out of 1..31", func() { b.RotL32(Y0, Y0, 0, Y1) })
	mustPanic(t, "out of 1..31", func() { b.RotL32(Y0, Y0, 32, Y1) })
	mustPanic(t, "scratch", func() { b.RotL32(Y0, Y1, 5, Y0) })
	mustPanic(t, "scratch", func() { b.RotL32(Y0, Y1, 5, Y1) })
	mustPanic(t, "distinct", func() { b.Transpose4x32(Y0, Y1, Y2, Y3, Y4, Y0) })
	mustPanic(t, "distinct", func() { b.Transpose4x32(Y0, Y1, Y2, Y3, Y5, Y5) })
	mustPanic(t, "invalid YMM", func() { b.VPXOR(YReg(16), Y0, Y0) })
}

func TestYmmUnpacksAndTranspose(t *testing.T) {
	b := ymmBuilder()
	b.VPUNPCKLDQ(Y0, Y1, Y2).VPUNPCKHDQ(Y0, Y1, Y2).VPUNPCKLQDQ(Y0, Y1, Y2).VPUNPCKHQDQ(Y0, Y1, Y2)
	checkInsns(t, b, []string{
		"VPUNPCKLDQ Y2, Y1, Y0",
		"VPUNPCKHDQ Y2, Y1, Y0",
		"VPUNPCKLQDQ Y2, Y1, Y0",
		"VPUNPCKHQDQ Y2, Y1, Y0",
	})
	b = ymmBuilder()
	b.Transpose4x32(Y0, Y1, Y2, Y3, Y4, Y5)
	checkInsns(t, b, []string{
		"VPUNPCKLDQ Y1, Y0, Y4",
		"VPUNPCKHDQ Y1, Y0, Y5",
		"VPUNPCKLDQ Y3, Y2, Y0",
		"VPUNPCKHDQ Y3, Y2, Y1",
		"VPUNPCKLQDQ Y1, Y5, Y2",
		"VPUNPCKHQDQ Y1, Y5, Y3",
		"VPUNPCKHQDQ Y0, Y4, Y5",
		"VPUNPCKLQDQ Y0, Y4, Y4",
		"VMOVDQA Y4, Y0",
		"VMOVDQA Y5, Y1",
	})
}
