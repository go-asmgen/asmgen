package arm64

import (
	"fmt"
	"strings"
	"testing"
)

// vfpGolden (vfp_golden_test.go) was produced by the system assembler (Apple
// clang's `as -arch arm64`, read back with `otool -t`) from the assembly in
// each comment, covering every register field at 0, 31 and in between. It is
// the reference the hand encoding has to match.

func TestVectorFloatEncodings(t *testing.T) {
	for _, g := range vfpGolden {
		b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
		r := g.regs
		switch g.op {
		case "fadd":
			b.VFADD2D(r[0], r[1], r[2])
		case "fsub":
			b.VFSUB2D(r[0], r[1], r[2])
		case "fmul":
			b.VFMUL2D(r[0], r[1], r[2])
		case "fmla":
			b.VFMLA2D(r[0], r[1], r[2])
		case "fmls":
			b.VFMLS2D(r[0], r[1], r[2])
		case "fneg":
			b.VFNEG2D(r[0], r[1])
		}
		want := fmt.Sprintf("WORD $0x%08x // %s", g.want, g.op)
		if s := b.Func().String(); !strings.Contains(s, want) {
			t.Errorf("%s %v: emitted\n%s\nwant a line starting %q", g.op, r, s, want)
		}
	}
}

func TestVectorFloatRefusesARegisterThatDoesNotExist(t *testing.T) {
	for _, f := range []func(*Builder){
		func(b *Builder) { b.VFADD2D(32, 0, 0) },
		func(b *Builder) { b.VFMUL2D(0, -1, 0) },
		func(b *Builder) { b.VFMLS2D(0, 0, 40) },
		func(b *Builder) { b.VFNEG2D(0, 32) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("an out-of-range vector register was encoded instead of refused")
				}
			}()
			f(NewFunc("k", Layout(nil, nil, nil, nil), 0))
		}()
	}
}
