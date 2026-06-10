// Package arm64 provides an ergonomic, ABI0-targeted builder that emits
// Plan 9 assembly for the arm64 architecture. Encoding is delegated to the Go
// toolchain's assembler (cmd/asm); this package only computes the ABI0 frame
// layout and emits well-formed Plan 9 instruction text.
package arm64

import (
	"fmt"

	"github.com/go-asmgen/asmgen/internal/emit"
)

// ----------------------------------------------------------------------------
// ABI0 layout
//
// Under Go's ABI0, arguments and results are passed on the stack, accessed via
// the FP pseudo-register, laid out in declaration order. Each value sits at an
// offset that respects its own alignment (which, for the scalar types modelled
// here, equals its size). The result area starts at an offset rounded up to the
// register word size (8 on arm64); the total frame size is the end of the last
// result and is NOT rounded further — these are the exact offsets `go vet`
// asmdecl expects (e.g. func(a,b int16) int16 lays out a@0, b@2, ret@8, size 10).
//
// This is correct for sequences of arm64 scalars — signed/unsigned integers of
// 1/2/4/8 bytes, pointers, and 32/64-bit floats — in any combination. It is NOT
// yet correct for structs, arrays, or vector (V-register) values, which need
// field decomposition / different register files. Widen carefully and test each
// case against `go vet` asmdecl, which cross-checks .s offsets and access widths
// against the Go declaration.
// ----------------------------------------------------------------------------

const wordSize = 8

// Type describes one ABI0 scalar parameter type. Size is in bytes (1, 2, 4, or
// 8). Float selects the floating-point register file (F0..) and the FMOV*
// moves. Signed controls whether a sub-word integer load sign-extends (MOVB) or
// zero-extends (MOVBU); it is ignored for floats and for 8-byte values.
type Type struct {
	Size   int
	Float  bool
	Signed bool
}

// Predefined ABI0 scalar types mirroring the Go builtins.
var (
	Int8    = Type{Size: 1, Signed: true}
	Int16   = Type{Size: 2, Signed: true}
	Int32   = Type{Size: 4, Signed: true}
	Int64   = Type{Size: 8, Signed: true}
	Uint8   = Type{Size: 1}
	Uint16  = Type{Size: 2}
	Uint32  = Type{Size: 4}
	Uint64  = Type{Size: 8}
	Ptr     = Type{Size: 8}
	Float32 = Type{Size: 4, Float: true}
	Float64 = Type{Size: 8, Float: true}
)

// Param describes one laid-out ABI0 stack slot (argument or result).
type Param struct {
	Name   string
	Type   Type
	Offset int // computed FP-relative offset
}

// Signature holds the computed layout for a function.
type Signature struct {
	Args     []Param
	Rets     []Param
	ArgsSize int // total args+rets region, word-aligned
}

func align(n, to int) int { return (n + to - 1) &^ (to - 1) }

// Layout computes ABI0 FP offsets for the given argument and result types, in
// declaration order. Arguments are laid out first, then results, contiguously.
func Layout(argNames []string, argTypes []Type, retNames []string, retTypes []Type) Signature {
	var sig Signature
	off := 0
	place := func(names []string, types []Type) []Param {
		ps := make([]Param, 0, len(types))
		for i, t := range types {
			off = align(off, t.Size)
			ps = append(ps, Param{Name: names[i], Type: t, Offset: off})
			off += t.Size
		}
		return ps
	}
	sig.Args = place(argNames, argTypes)
	// Results begin at a word-aligned offset (ABI0 aligns the result area to the
	// register size). The frame is then sized to the end of the last result,
	// with no further rounding — matching what asmdecl computes.
	off = align(off, wordSize)
	sig.Rets = place(retNames, retTypes)
	sig.ArgsSize = off
	return sig
}

// loadMnemonic returns the Plan 9 arm64 move that loads a value of type t from
// memory into a register. Sub-word integer loads pick the sign- or
// zero-extending form so the full register holds the correct value.
func loadMnemonic(t Type) string {
	if t.Float {
		if t.Size == 4 {
			return "FMOVS"
		}
		return "FMOVD"
	}
	switch t.Size {
	case 1:
		if t.Signed {
			return "MOVB"
		}
		return "MOVBU"
	case 2:
		if t.Signed {
			return "MOVH"
		}
		return "MOVHU"
	case 4:
		if t.Signed {
			return "MOVW"
		}
		return "MOVWU"
	default:
		return "MOVD"
	}
}

// storeMnemonic returns the Plan 9 arm64 move that stores a register into a slot
// of type t. Stores write exactly the type's width, so signedness is irrelevant.
func storeMnemonic(t Type) string {
	if t.Float {
		if t.Size == 4 {
			return "FMOVS"
		}
		return "FMOVD"
	}
	switch t.Size {
	case 1:
		return "MOVB"
	case 2:
		return "MOVH"
	case 4:
		return "MOVW"
	default:
		return "MOVD"
	}
}

// ----------------------------------------------------------------------------
// Builder
// ----------------------------------------------------------------------------

// Builder constructs one arm64 function.
type Builder struct {
	fn  *emit.Function
	sig Signature
}

// NewFunc starts an ABI0 arm64 function. frameSize is the local frame (0 if the
// function uses no stack locals). The function is marked NOSPLIT by default for
// simplicity in v0 (no stack-growth preamble); revisit for large frames.
func NewFunc(name string, sig Signature, frameSize int) *Builder {
	return &Builder{
		fn:  emit.NewFunction(name, "NOSPLIT", frameSize, sig.ArgsSize),
		sig: sig,
	}
}

// LoadArg emits a move of the named argument into register reg. The move width
// and register file follow the argument's type (e.g. "R0" for ints/pointers,
// "F0" for floats).
func (b *Builder) LoadArg(name, reg string) *Builder {
	p := b.lookup(b.sig.Args, name)
	b.fn.Insn("%s %s+%d(FP), %s", loadMnemonic(p.Type), name, p.Offset, reg)
	return b
}

// StoreRet emits a move of register reg into the named result slot, with the
// width determined by the result's type.
func (b *Builder) StoreRet(reg, name string) *Builder {
	p := b.lookup(b.sig.Rets, name)
	b.fn.Insn("%s %s, %s+%d(FP)", storeMnemonic(p.Type), reg, name, p.Offset)
	return b
}

// Raw appends an arbitrary Plan 9 arm64 instruction (escape hatch for the
// arithmetic/logic the v0 emit surface does not model directly).
func (b *Builder) Raw(format string, args ...any) *Builder {
	b.fn.Insn(format, args...)
	return b
}

// Ret emits the return.
func (b *Builder) Ret() *Builder { b.fn.Insn("RET"); return b }

func (b *Builder) Func() *emit.Function { return b.fn }

func (b *Builder) lookup(ps []Param, name string) Param {
	for _, p := range ps {
		if p.Name == name {
			return p
		}
	}
	panic(fmt.Sprintf("arm64: unknown param %q", name))
}
