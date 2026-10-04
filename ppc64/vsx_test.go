package ppc64

import (
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/internal/gap"
)

// The encodings are tested against their references in internal/gap; this
// checks each method emits its own entry, operands in ISA order.
func TestVSXEmitsTheRegistryWord(t *testing.T) {
	ops := map[string]func(*Builder, []int){
		"xvadddp":   func(b *Builder, r []int) { b.XVADDDP(r[0], r[1], r[2]) },
		"xvsubdp":   func(b *Builder, r []int) { b.XVSUBDP(r[0], r[1], r[2]) },
		"xvmuldp":   func(b *Builder, r []int) { b.XVMULDP(r[0], r[1], r[2]) },
		"xvdivdp":   func(b *Builder, r []int) { b.XVDIVDP(r[0], r[1], r[2]) },
		"xvmaddadp": func(b *Builder, r []int) { b.XVMADDADP(r[0], r[1], r[2]) },
		"xvmaxdp":   func(b *Builder, r []int) { b.XVMAXDP(r[0], r[1], r[2]) },
		"xvmindp":   func(b *Builder, r []int) { b.XVMINDP(r[0], r[1], r[2]) },
		"xvsqrtdp":  func(b *Builder, r []int) { b.XVSQRTDP(r[0], r[1]) },
	}
	for _, in := range gap.ArchOf("ppc64").Insns() {
		emit, ok := ops[in.ISA]
		if !ok {
			t.Errorf("registry entry %s has no method", in.ISA)
			continue
		}
		delete(ops, in.ISA)
		for _, c := range in.Golden {
			b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
			emit(b, c.Ops)
			if s := b.Func().String(); !strings.Contains(s, "\t"+in.Word(c.Ops...)+"\n") {
				t.Errorf("%s %v: emitted\n%s", in.ISA, c.Ops, s)
			}
		}
	}
	for isa := range ops {
		t.Errorf("method for %s has no registry entry", isa)
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
