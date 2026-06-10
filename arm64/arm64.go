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
// Under Go's ABI0, integer/pointer arguments and results are passed on the
// stack, accessed via the FP pseudo-register, laid out in declaration order.
// Each value sits at an offset that respects its own alignment; the total
// argument+result area is rounded up to the register word size (8 on arm64).
//
// IMPORTANT (verify first when you run this locally): the offsets below assume
// natural alignment and no padding rules beyond word alignment of the whole
// frame. For mixed-size or struct arguments this is NOT yet correct. The v0
// only claims correctness for sequences of 8-byte (int64/uint64/pointer)
// values. Widen carefully and test each case against `go vet` asmdecl, which
// cross-checks .s offsets against the Go declaration.
// ----------------------------------------------------------------------------

const wordSize = 8

// Param describes one ABI0 stack slot (argument or result).
type Param struct {
	Name   string
	Size   int // bytes
	Offset int // computed FP-relative offset
}

// Signature holds the computed layout for a function.
type Signature struct {
	Args     []Param
	Rets     []Param
	ArgsSize int // total args+rets region, word-aligned
}

func align(n, to int) int { return (n + to - 1) &^ (to - 1) }

// Layout computes ABI0 FP offsets for the given argument and result sizes,
// in declaration order. argSizes then retSizes are laid out contiguously.
func Layout(argNames []string, argSizes []int, retNames []string, retSizes []int) Signature {
	var sig Signature
	off := 0
	for i, sz := range argSizes {
		off = align(off, sz)
		sig.Args = append(sig.Args, Param{Name: argNames[i], Size: sz, Offset: off})
		off += sz
	}
	for i, sz := range retSizes {
		off = align(off, sz)
		sig.Rets = append(sig.Rets, Param{Name: retNames[i], Size: sz, Offset: off})
		off += sz
	}
	sig.ArgsSize = align(off, wordSize)
	return sig
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

// LoadArg emits a MOVD of the named argument into register reg (e.g. "R0").
func (b *Builder) LoadArg(name, reg string) *Builder {
	p := b.lookup(b.sig.Args, name)
	b.fn.Insn("MOVD %s+%d(FP), %s", name, p.Offset, reg)
	return b
}

// StoreRet emits a MOVD of register reg into the named result slot.
func (b *Builder) StoreRet(reg, name string) *Builder {
	p := b.lookup(b.sig.Rets, name)
	b.fn.Insn("MOVD %s, %s+%d(FP)", reg, name, p.Offset)
	return b
}

// Raw appends an arbitrary Plan 9 arm64 instruction (escape hatch).
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
