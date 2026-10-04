package loong64

import (
	"fmt"
	"strings"
	"testing"
)

// lasxGolden (lasx_golden_test.go) covers every register field at 0 and 31
// and mixes in between, and the broadcast-load offset at 0, ±8, 2040 and
// -2048 (the ends of its scaled signed 9-bit field).

func TestLASXEncodings(t *testing.T) {
	for _, g := range lasxGolden {
		b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
		a := g.args
		switch g.op {
		case "xvfmadd.d":
			b.XVFMADDD(a[0], a[1], a[2], a[3])
		case "vfmadd.d":
			b.VFMADDD(a[0], a[1], a[2], a[3])
		case "xvldrepl.d":
			b.XVLDREPLD(a[0], a[1], a[2])
		case "vldrepl.d":
			b.VLDREPLD(a[0], a[1], a[2])
		default:
			t.Fatalf("golden has an op with no encoder: %s", g.op)
		}
		want := fmt.Sprintf("WORD $0x%08x // %s", g.want, g.op)
		if s := b.Func().String(); !strings.Contains(s, want) {
			t.Errorf("%s %v: emitted\n%s\nwant a line starting %q", g.op, a, s, want)
		}
	}
}

func TestLASXRefusesWhatDoesNotEncode(t *testing.T) {
	for name, f := range map[string]func(*Builder){
		"vector 32":        func(b *Builder) { b.XVFMADDD(32, 0, 0, 0) },
		"vector -1":        func(b *Builder) { b.VFMADDD(0, 0, 0, -1) },
		"vector 32 (load)": func(b *Builder) { b.XVLDREPLD(32, 4, 0) },
		"general 32":       func(b *Builder) { b.VLDREPLD(0, 32, 0) },
		"general -1":       func(b *Builder) { b.VLDREPLD(0, -1, 0) },
		"offset not ×8":    func(b *Builder) { b.XVLDREPLD(0, 4, 4) },
		"offset 2048":      func(b *Builder) { b.XVLDREPLD(0, 4, 2048) },
		"offset -2056":     func(b *Builder) { b.VLDREPLD(0, 4, -2056) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s was encoded", name)
				}
			}()
			f(NewFunc("k", Layout(nil, nil, nil, nil), 0))
		}()
	}
}
