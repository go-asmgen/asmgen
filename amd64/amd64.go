// Package amd64 provides an ergonomic, ABI0-targeted builder that emits Plan 9
// assembly for the amd64 architecture. Encoding is delegated to the Go
// toolchain's assembler (cmd/asm); this package only drives the move-selection
// surface and emits well-formed Plan 9 instruction text.
//
// amd64 is already well served by avo for instruction-level work; go-asmgen
// supports it so a single uniform builder spans every 64-bit Go target. It
// reuses the shared ABI0 layout model unchanged. amd64's move table is a little
// richer than the RISC targets': sub-word loads use the explicit sign/zero-
// extending forms (MOVBQSX/MOVBQZX, ...), the 8-byte move is MOVQ, and floats
// use the SSE scalar moves MOVSS (single) / MOVSD (double) on X registers.
package amd64

import (
	"fmt"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
)

// Re-exported ABI0 model (see abi). Correct for sequences of amd64 scalars —
// signed/unsigned integers of 1/2/4/8 bytes, pointers, and 32/64-bit floats — in
// any combination; not yet arrays or vector values (though vector code can be
// emitted via Raw; see examples/simd).
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

// loadMnemonic returns the Plan 9 amd64 move that loads a value of type t into a
// register. Sub-word integer loads use the explicit sign- or zero-extending
// forms so the full 64-bit register holds the correct value (unlike the RISC
// targets, amd64 has distinct mnemonics for this).
func loadMnemonic(t Type) string {
	if t.Float {
		if t.Size == 4 {
			return "MOVSS"
		}
		return "MOVSD"
	}
	switch t.Size {
	case 1:
		if t.Signed {
			return "MOVBQSX"
		}
		return "MOVBQZX"
	case 2:
		if t.Signed {
			return "MOVWQSX"
		}
		return "MOVWQZX"
	case 4:
		if t.Signed {
			return "MOVLQSX"
		}
		return "MOVLQZX"
	default:
		return "MOVQ"
	}
}

// storeMnemonic returns the Plan 9 amd64 move that stores a register into a slot
// of type t. Stores write exactly the type's width, so signedness is irrelevant.
func storeMnemonic(t Type) string {
	if t.Float {
		if t.Size == 4 {
			return "MOVSS"
		}
		return "MOVSD"
	}
	switch t.Size {
	case 1:
		return "MOVB"
	case 2:
		return "MOVW"
	case 4:
		return "MOVL"
	default:
		return "MOVQ"
	}
}

// ----------------------------------------------------------------------------
// Builder
// ----------------------------------------------------------------------------

// Builder constructs one amd64 function.
type Builder struct {
	fn  *emit.Function
	sig Signature
}

// NewFunc starts an ABI0 amd64 function. frameSize is the local frame (0 if the
// function uses no stack locals). The function is marked NOSPLIT by default for
// simplicity in v0 (no stack-growth preamble); revisit for large frames.
func NewFunc(name string, sig Signature, frameSize int) *Builder {
	return NewFuncFlags(name, sig, frameSize, "NOSPLIT")
}

// NewFuncFlags is like NewFunc but with explicit Plan 9 TEXT flags — e.g.
// "NOSPLIT", "NOSPLIT|NOFRAME", or "" for none (which lets the assembler insert
// the stack-growth preamble, needed for large frames or non-leaf functions).
func NewFuncFlags(name string, sig Signature, frameSize int, flags string) *Builder {
	return &Builder{
		fn:  emit.NewFunction(name, flags, frameSize, sig.ArgsSize),
		sig: sig,
	}
}

// LoadArg emits a move of the named argument into register reg. The move width
// and register file follow the argument's type (e.g. "AX" for ints/pointers,
// "X0" for floats).
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

// Raw appends an arbitrary Plan 9 amd64 instruction (escape hatch for the
// arithmetic/logic — including SSE/AVX vector ops — the v0 emit surface does not
// model directly).
func (b *Builder) Raw(format string, args ...any) *Builder {
	b.fn.Insn(format, args...)
	return b
}

// Label emits a branch target (e.g. a loop header) at column 0, for use with
// branch instructions written via Raw.
func (b *Builder) Label(name string) *Builder {
	b.fn.Label(name)
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
	panic(fmt.Sprintf("amd64: unknown param %q", name))
}
