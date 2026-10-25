package arm64

import (
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/internal/gap"
)

var elemMethods = map[string]func(*Builder, []int){
	"fmla.elem.2d": func(b *Builder, r []int) { b.VFMLAElem2D(r[0], r[1], r[2], r[3]) },
	"fmls.elem.2d": func(b *Builder, r []int) { b.VFMLSElem2D(r[0], r[1], r[2], r[3]) },
	"fmul.elem.2d": func(b *Builder, r []int) { b.VFMULElem2D(r[0], r[1], r[2], r[3]) },
	"fmla.elem.4s": func(b *Builder, r []int) { b.VFMLAElem4S(r[0], r[1], r[2], r[3]) },
	"fmls.elem.4s": func(b *Builder, r []int) { b.VFMLSElem4S(r[0], r[1], r[2], r[3]) },
	"fmul.elem.4s": func(b *Builder, r []int) { b.VFMULElem4S(r[0], r[1], r[2], r[3]) },
}

// Every arm64 registry entry has a method, and each emits its own entry's
// WORD, operands in A64 order. cmd/asm then lays down exactly the reference
// words: a WORD it cannot misencode, but this proves the line is a WORD and
// nothing else.
func TestElemEmitsTheRegistryWords(t *testing.T) {
	ops := map[string]func(*Builder, []int){}
	for k, v := range elemMethods {
		ops[k] = v
	}
	b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
	var want []uint32
	for _, in := range gap.ArchOf("arm64").Insns() {
		emit, ok := ops[in.ISA]
		if !ok {
			t.Errorf("registry entry %s has no method", in.ISA)
			continue
		}
		delete(ops, in.ISA)
		for _, c := range in.Golden {
			one := NewFunc("k", Layout(nil, nil, nil, nil), 0)
			emit(one, c.Ops)
			if s := one.Func().String(); !strings.Contains(s, "\t"+in.Word(c.Ops...)+"\n") {
				t.Errorf("%s %v: emitted\n%s", in.ISA, c.Ops, s)
			}
			emit(b, c.Ops)
			want = append(want, uint32(c.Want))
		}
	}
	for isa := range ops {
		t.Errorf("method for %s has no registry entry", isa)
	}
	got := assembleArm64(t, b.Func().String())
	if len(got) < len(want) {
		t.Fatalf("cmd/asm produced %d words for %d instructions", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("word %d: cmd/asm laid down %#08x, want %#08x", i, got[i], want[i])
		}
	}
}

// The line go-ndarray's GEMM kernel writes first, verbatim but for the hex
// case: internal/kernels/sum_arm64.s, "WORD $0x4FD41200 // FMLA V0.2D,
// V16.2D, V20.D[0]".
func TestVFMLAElem2DLine(t *testing.T) {
	b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
	b.VFMLAElem2D(0, 16, 20, 0)
	if s := b.Func().String(); !strings.Contains(s, "\tWORD $0x4fd41200 // VFMLA V20.D[0], V16.D2, V0.D2\n") {
		t.Errorf("emitted\n%s", s)
	}
}

func TestElemRefusesWhatDoesNotExist(t *testing.T) {
	for name, f := range map[string]func(*Builder){
		"V32 d":       func(b *Builder) { b.VFMLAElem2D(32, 0, 0, 0) },
		"V-1 n":       func(b *Builder) { b.VFMLSElem4S(0, -1, 0, 0) },
		"V32 m":       func(b *Builder) { b.VFMULElem2D(0, 0, 32, 0) },
		".D index 2":  func(b *Builder) { b.VFMLAElem2D(0, 0, 0, 2) },
		".S index 4":  func(b *Builder) { b.VFMULElem4S(0, 0, 0, 4) },
		".S index -1": func(b *Builder) { b.VFMLAElem4S(0, 0, 0, -1) },
		".D index -1": func(b *Builder) { b.VFMLSElem2D(0, 0, 0, -1) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: encoded", name)
				}
			}()
			f(NewFunc("k", Layout(nil, nil, nil, nil), 0))
		}()
	}
}
