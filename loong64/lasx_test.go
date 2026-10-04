package loong64

import (
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/internal/gap"
)

// The encodings are tested against their references in internal/gap; this
// checks what each method emits: the mnemonic where cmd/asm has one, the
// registry's WORD where it does not.
func TestLASXEmits(t *testing.T) {
	ops := map[string]func(*Builder, []int){
		"xvfmadd.d":  func(b *Builder, a []int) { b.XVFMADDD(a[0], a[1], a[2], a[3]) },
		"vfmadd.d":   func(b *Builder, a []int) { b.VFMADDD(a[0], a[1], a[2], a[3]) },
		"xvldrepl.d": func(b *Builder, a []int) { b.XVLDREPLD(a[0], a[1], a[2]) },
		"vldrepl.d":  func(b *Builder, a []int) { b.VLDREPLD(a[0], a[1], a[2]) },
	}
	for _, in := range gap.ArchOf("loong64").Insns() {
		emit, ok := ops[in.ISA]
		if !ok {
			t.Errorf("registry entry %s has no method", in.ISA)
			continue
		}
		delete(ops, in.ISA)
		for _, c := range in.Golden {
			want := in.Word(c.Ops...)
			if strings.Contains(in.ISA, "ldrepl") && c.Ops[2] != -2048 {
				want = in.Syntax(c.Ops...)
			}
			b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
			emit(b, c.Ops)
			if s := b.Func().String(); !strings.Contains(s, "\t"+want+"\n") {
				t.Errorf("%s %v: emitted\n%s\nwant %q", in.ISA, c.Ops, s, want)
			}
		}
	}
	for isa := range ops {
		t.Errorf("method for %s has no registry entry", isa)
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
