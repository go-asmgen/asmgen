package arm64

import "github.com/go-asmgen/asmgen/internal/gap"

// Vector float multiply (and fused multiply-add/subtract) by element: every
// lane of Vn times ONE lane of Vm, the broadcast a GEMM micro-kernel or a
// polynomial with a shared coefficient needs without a separate VDUP.
//
// TRANSITIONAL. Go's assembler (master, checked 2026-10-25) has the
// vector-by-vector VFMLA/VFMLS/VFMUL but not their indexed forms, so these
// methods emit the instruction as a WORD, commented with the spelling
// go.dev/cl/764224 gives it (VFMLA Vm.D[i], Vn.D2, Vd.D2). The encodings and
// the llvm-mc / GNU as references they are held to live in internal/gap;
// when a Go release assembles them, these methods emit the mnemonic instead.
//
// Operands are in A64 order, like the other methods here: (d, n, m, index)
// for "fmla vd.2d, vn.2d, vm.d[index]". Vm is any of V0-V31 (only the
// half-precision forms are limited to V0-V15); the index is 0-3 for .4S and
// 0-1 for .2D. A register or an index outside those panics at generation time.

// VFMLAElem2D emits Vd.2D += Vn.2D × Vm.D[index], fused (one rounding).
func (b *Builder) VFMLAElem2D(d, n, m, index int) *Builder {
	return b.elem("fmla.elem.2d", d, n, m, index)
}

// VFMLSElem2D emits Vd.2D -= Vn.2D × Vm.D[index], fused (one rounding).
func (b *Builder) VFMLSElem2D(d, n, m, index int) *Builder {
	return b.elem("fmls.elem.2d", d, n, m, index)
}

// VFMULElem2D emits Vd.2D = Vn.2D × Vm.D[index].
func (b *Builder) VFMULElem2D(d, n, m, index int) *Builder {
	return b.elem("fmul.elem.2d", d, n, m, index)
}

// VFMLAElem4S emits Vd.4S += Vn.4S × Vm.S[index], fused (one rounding).
func (b *Builder) VFMLAElem4S(d, n, m, index int) *Builder {
	return b.elem("fmla.elem.4s", d, n, m, index)
}

// VFMLSElem4S emits Vd.4S -= Vn.4S × Vm.S[index], fused (one rounding).
func (b *Builder) VFMLSElem4S(d, n, m, index int) *Builder {
	return b.elem("fmls.elem.4s", d, n, m, index)
}

// VFMULElem4S emits Vd.4S = Vn.4S × Vm.S[index].
func (b *Builder) VFMULElem4S(d, n, m, index int) *Builder {
	return b.elem("fmul.elem.4s", d, n, m, index)
}

func (b *Builder) elem(isa string, ops ...int) *Builder {
	return b.Raw("%s", gap.Lookup("arm64", isa).Word(ops...))
}
