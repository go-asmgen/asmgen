package ppc64

import (
	"fmt"
	"strings"
	"testing"
)

// vsxGolden (vsx_golden_test.go) covers every register field at 0, 31, 32 and
// 63 — the boundary between the low five bits and the high bit stored apart in
// AX/BX/TX — and mixes in between.

func TestVSXEncodings(t *testing.T) {
	for _, g := range vsxGolden {
		b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
		r := g.regs
		switch g.op {
		case "xvadddp":
			b.XVADDDP(r[0], r[1], r[2])
		case "xvsubdp":
			b.XVSUBDP(r[0], r[1], r[2])
		case "xvmuldp":
			b.XVMULDP(r[0], r[1], r[2])
		case "xvdivdp":
			b.XVDIVDP(r[0], r[1], r[2])
		case "xvmaddadp":
			b.XVMADDADP(r[0], r[1], r[2])
		case "xvmaxdp":
			b.XVMAXDP(r[0], r[1], r[2])
		case "xvmindp":
			b.XVMINDP(r[0], r[1], r[2])
		case "xvsqrtdp":
			b.XVSQRTDP(r[0], r[1])
		default:
			t.Fatalf("golden has an op with no encoder: %s", g.op)
		}
		want := fmt.Sprintf("WORD $0x%08x // %s", g.want, g.op)
		if s := b.Func().String(); !strings.Contains(s, want) {
			t.Errorf("%s %v: emitted\n%s\nwant a line starting %q", g.op, r, s, want)
		}
	}
}

func TestVSXRefusesARegisterThatDoesNotExist(t *testing.T) {
	for _, f := range []func(*Builder){
		func(b *Builder) { b.XVADDDP(64, 0, 0) },
		func(b *Builder) { b.XVMULDP(0, -1, 0) },
		func(b *Builder) { b.XVMADDADP(0, 0, 64) },
		func(b *Builder) { b.XVSQRTDP(64, 0) },
		func(b *Builder) { b.XVSQRTDP(0, 64) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("a register outside VS0–VS63 was encoded")
				}
			}()
			f(NewFunc("k", Layout(nil, nil, nil, nil), 0))
		}()
	}
}
