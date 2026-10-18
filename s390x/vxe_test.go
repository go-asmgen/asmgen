package s390x

import (
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/internal/gap"
)

// The encodings are tested against their references in internal/gap; this
// checks each method emits its own entry, operands in ISA order.
func TestVXEEmitsTheRegistryBytes(t *testing.T) {
	ops := map[string]func(*Builder, []int){
		"vfasb":  func(b *Builder, r []int) { b.VFASB(r[0], r[1], r[2]) },
		"wfasb":  func(b *Builder, r []int) { b.WFASB(r[0], r[1], r[2]) },
		"vfssb":  func(b *Builder, r []int) { b.VFSSB(r[0], r[1], r[2]) },
		"wfssb":  func(b *Builder, r []int) { b.WFSSB(r[0], r[1], r[2]) },
		"vfmsb":  func(b *Builder, r []int) { b.VFMSB(r[0], r[1], r[2]) },
		"wfmsb":  func(b *Builder, r []int) { b.WFMSB(r[0], r[1], r[2]) },
		"vfdsb":  func(b *Builder, r []int) { b.VFDSB(r[0], r[1], r[2]) },
		"wfdsb":  func(b *Builder, r []int) { b.WFDSB(r[0], r[1], r[2]) },
		"vfmasb": func(b *Builder, r []int) { b.VFMASB(r[0], r[1], r[2], r[3]) },
		"wfmasb": func(b *Builder, r []int) { b.WFMASB(r[0], r[1], r[2], r[3]) },
		"vfmssb": func(b *Builder, r []int) { b.VFMSSB(r[0], r[1], r[2], r[3]) },
		"wfmssb": func(b *Builder, r []int) { b.WFMSSB(r[0], r[1], r[2], r[3]) },
		"vfsqsb": func(b *Builder, r []int) { b.VFSQSB(r[0], r[1]) },
		"wfsqsb": func(b *Builder, r []int) { b.WFSQSB(r[0], r[1]) },
	}
	for _, in := range gap.ArchOf("s390x").Insns() {
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

// The line itself, for the one go-ndarray uses: the six bytes GNU as gives
// for vfasb %v3,%v1,%v2, then the spelling go.dev/cl/846286 gives it.
func TestVFASBLine(t *testing.T) {
	b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
	b.VFASB(3, 1, 2)
	if s := b.Func().String(); !strings.Contains(s, "\tWORD $0xe7312000; BYTE $0x20; BYTE $0xe3 // VFASB V1, V2, V3\n") {
		t.Errorf("emitted\n%s", s)
	}
}

func TestVXERefusesARegisterThatDoesNotExist(t *testing.T) {
	for _, f := range []func(*Builder){
		func(b *Builder) { b.VFASB(32, 0, 0) },
		func(b *Builder) { b.VFSSB(0, -1, 0) },
		func(b *Builder) { b.VFMSB(0, 0, 32) },
		func(b *Builder) { b.VFMASB(0, 0, 0, 32) },
		func(b *Builder) { b.WFMSSB(-1, 0, 0, 0) },
		func(b *Builder) { b.VFSQSB(32, 0) },
		func(b *Builder) { b.WFSQSB(0, 32) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("a register outside V0–V31 was encoded")
				}
			}()
			f(NewFunc("k", Layout(nil, nil, nil, nil), 0))
		}()
	}
}
