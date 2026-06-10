// Package riscv64 provides an ergonomic, ABI0-targeted builder that emits
// Plan 9 assembly for the riscv64 architecture. Encoding is delegated to the Go
// toolchain's assembler (cmd/asm); this package only drives the move-selection
// surface and emits well-formed Plan 9 instruction text.
//
// The ABI0 frame layout is identical to every other 64-bit target and is shared
// from abi — demonstrating the core go-asmgen thesis: a new
// architecture is just register names plus a move table over the same layout
// model. riscv64 differs from arm64 only in those mnemonics: the 8-byte integer
// move is MOV (not MOVD), and floats use MOVF (single) / MOVD (double).
package riscv64

import (
	"fmt"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
)

// Re-exported ABI0 model (see abi). Correct for sequences of riscv64
// scalars — signed/unsigned integers of 1/2/4/8 bytes, pointers, and 32/64-bit
// floats — in any combination; not yet structs, arrays, or vectors.
type (
	// Type describes one ABI0 scalar parameter type. See abi.Type.
	Type = abi.Type
	// Param is one laid-out ABI0 stack slot. See abi.Param.
	Param = abi.Param
	// Signature holds the computed layout for a function. See abi.Signature.
	Signature = abi.Signature
)

// Predefined ABI0 scalar types mirroring the Go builtins.
var (
	Int8    = abi.Int8
	Int16   = abi.Int16
	Int32   = abi.Int32
	Int64   = abi.Int64
	Uint8   = abi.Uint8
	Uint16  = abi.Uint16
	Uint32  = abi.Uint32
	Uint64  = abi.Uint64
	Ptr     = abi.Ptr
	Float32 = abi.Float32
	Float64 = abi.Float64
)

// Layout computes ABI0 FP offsets for the given argument and result types. The
// result area is word-aligned and the frame size is the end of the last result;
// these are the offsets `go vet` asmdecl expects.
func Layout(argNames []string, argTypes []Type, retNames []string, retTypes []Type) Signature {
	return abi.Layout(argNames, argTypes, retNames, retTypes)
}

// loadMnemonic returns the Plan 9 riscv64 move that loads a value of type t into
// a register. Sub-word integer loads pick the sign- or zero-extending form so
// the full register holds the correct value. Note MOV (not MOVD) is the 8-byte
// integer move; MOVF/MOVD are the float moves.
func loadMnemonic(t Type) string {
	if t.Float {
		if t.Size == 4 {
			return "MOVF"
		}
		return "MOVD"
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
		return "MOV"
	}
}

// storeMnemonic returns the Plan 9 riscv64 move that stores a register into a
// slot of type t. Stores write exactly the type's width, so signedness is
// irrelevant.
func storeMnemonic(t Type) string {
	if t.Float {
		if t.Size == 4 {
			return "MOVF"
		}
		return "MOVD"
	}
	switch t.Size {
	case 1:
		return "MOVB"
	case 2:
		return "MOVH"
	case 4:
		return "MOVW"
	default:
		return "MOV"
	}
}

// ----------------------------------------------------------------------------
// Builder
// ----------------------------------------------------------------------------

// Builder constructs one riscv64 function.
type Builder struct {
	fn  *emit.Function
	sig Signature
}

// NewFunc starts an ABI0 riscv64 function. frameSize is the local frame (0 if
// the function uses no stack locals). The function is marked NOSPLIT by default
// for simplicity in v0 (no stack-growth preamble); revisit for large frames.
func NewFunc(name string, sig Signature, frameSize int) *Builder {
	return &Builder{
		fn:  emit.NewFunction(name, "NOSPLIT", frameSize, sig.ArgsSize),
		sig: sig,
	}
}

// LoadArg emits a move of the named argument into register reg. The move width
// and register file follow the argument's type (e.g. "X5" for ints/pointers,
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

// Raw appends an arbitrary Plan 9 riscv64 instruction (escape hatch for the
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
	panic(fmt.Sprintf("riscv64: unknown param %q", name))
}
