// Package arm64 provides an ergonomic, ABI0-targeted builder that emits
// Plan 9 assembly for the arm64 architecture. Encoding is delegated to the Go
// toolchain's assembler (cmd/asm); this package only computes the ABI0 frame
// layout and emits well-formed Plan 9 instruction text.
package arm64

import (
	"fmt"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
)

// The ABI0 frame layout (offsets, alignment, word size) is architecture-
// independent and lives in abi; arm64 differs from the other 64-bit
// targets only in register names and move mnemonics. These aliases and the
// Layout wrapper let callers stay entirely within the arm64 package.
//
// This is correct for sequences of arm64 scalars — signed/unsigned integers of
// 1/2/4/8 bytes, pointers, and 32/64-bit floats — in any combination. It is NOT
// yet correct for structs, arrays, or vector (V-register) values.
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
