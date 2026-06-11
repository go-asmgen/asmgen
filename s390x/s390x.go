// Package s390x provides an ergonomic, ABI0-targeted builder that emits Plan 9
// assembly for the s390x (IBM Z / z/Architecture) architecture. Encoding is
// delegated to the Go toolchain's assembler (cmd/asm); this package only drives
// the move-selection surface and emits well-formed Plan 9 instruction text.
//
// Like the other 64-bit targets, s390x reuses the shared ABI0 layout model
// (abi) unchanged — adding it was, once again, only a register/mnemonic table
// over the same offset rules. The integer move surface is uniform: a single
// MOVD covers 8-byte integers and pointers, with MOVW/MOVWZ/MOVH/MOVHZ/MOVB/
// MOVBZ for the narrower signed/unsigned widths; floats use FMOVS (single) /
// FMOVD (double).
//
// # BIG-ENDIAN
//
// s390x is the only big-endian target in go-asmgen. For scalar moves this is
// invisible — MOVD/MOVW/etc. load and store native-endian values exactly like
// the little-endian backends, and the ABI0 FP offsets are identical. It matters
// for SIMD bodies written through Raw:
//
//   - The vector facility (V0–V31) is big-endian: byte 0 of a VL-loaded vector
//     is the LOWEST memory address, which on z/Architecture is the MOST
//     significant byte (lane 0 = the leftmost / high-order element). On the
//     little-endian backends (amd64 VMOVDQU, arm64 VLD1, riscv64 VLE, loong64
//     VLD) lane 0 is the least significant byte. So a "VL (Rn), V0" puts the
//     first source byte into the high-order lane on s390x and the low-order lane
//     elsewhere — element-wise ops (VX xor, VCEQB compare-equal, VAB add) give
//     identical results regardless, but any lane-index-dependent shuffle, shift,
//     pack/unpack, or scalar extract (VLGVB/VLVGB) sees the lanes in the
//     opposite order. Consumers writing such bodies must mind this.
//   - A whole-array argument loaded with VL and stored with VST round-trips
//     byte-for-byte (the store mirrors the load), so the byte-parallel kernels
//     this package is exercised with (XOR, byte compare) match a Go reference
//     without any endian fix-up. Reductions and cross-lane moves do not.
//
// This is documented here so consumers emitting Raw vector bodies get lane order
// right; the backend itself only emits scalar moves + prologue/epilogue +
// labels, which are endian-agnostic.
package s390x

import (
	"fmt"

	"github.com/go-asmgen/asmgen/abi"
	"github.com/go-asmgen/asmgen/emit"
)

// Re-exported ABI0 model (see abi). Correct for sequences of s390x scalars —
// signed/unsigned integers of 1/2/4/8 bytes, pointers, and 32/64-bit floats —
// in any combination; not yet structs, arrays, or vectors.
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
// these are the offsets `go vet` asmdecl expects. The layout is byte-offset
// identical to the little-endian targets — ABI0 stack slots are endian-neutral.
func Layout(argNames []string, argTypes []Type, retNames []string, retTypes []Type) Signature {
	return abi.Layout(argNames, argTypes, retNames, retTypes)
}

// loadMnemonic returns the Plan 9 s390x move that loads a value of type t into a
// register. Sub-word integer loads pick the sign-extending (MOVW/MOVH/MOVB) or
// zero-extending (MOVWZ/MOVHZ/MOVBZ) form so the full 64-bit register holds the
// correct value. The 8-byte integer move is MOVD; FMOVS/FMOVD are the float
// moves.
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
		return "MOVBZ"
	case 2:
		if t.Signed {
			return "MOVH"
		}
		return "MOVHZ"
	case 4:
		if t.Signed {
			return "MOVW"
		}
		return "MOVWZ"
	default:
		return "MOVD"
	}
}

// storeMnemonic returns the Plan 9 s390x move that stores a register into a slot
// of type t. Stores write exactly the type's width, so signedness is
// irrelevant: MOVB/MOVH/MOVW/MOVD (and FMOVS/FMOVD for floats).
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

// Builder constructs one s390x function.
type Builder struct {
	fn  *emit.Function
	sig Signature
}

// NewFunc starts an ABI0 s390x function. frameSize is the local frame (0 if the
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
// and register file follow the argument's type (e.g. "R1" for ints/pointers,
// "F0" for floats). On a NOSPLIT leaf, R1–R5 are natural scratch; avoid R0,
// which reads as 0 in addressing contexts, and R14 (return address) / R15 (SP).
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

// Raw appends an arbitrary Plan 9 s390x instruction (escape hatch for the
// arithmetic/logic and the entire vector facility the v0 emit surface does not
// model directly). s390x SIMD operand order is sources-then-destination, e.g.
//
//	b.Raw("VL (R2), V0")        // load 16 bytes from (R2) into V0
//	b.Raw("VX V1, V0, V2")      // V2 = V0 XOR V1
//	b.Raw("VST V2, (R4)")       // store V2 to (R4)
//
// See the package doc for big-endian lane ordering.
func (b *Builder) Raw(format string, args ...any) *Builder {
	b.fn.Insn(format, args...)
	return b
}

// Label emits a branch target (e.g. a loop header) at column 0, for use with
// branch instructions written via Raw (e.g. "BNE loop", "BRC ...").
func (b *Builder) Label(name string) *Builder {
	b.fn.Label(name)
	return b
}

// LoadIndirect emits a load of a value of type t from the address held in
// ptrReg into reg — e.g. MOVD (R2), R3 — using the same move selection as
// LoadArg (sub-word loads sign/zero-extend).
func (b *Builder) LoadIndirect(ptrReg string, t Type, reg string) *Builder {
	b.fn.Insn("%s (%s), %s", loadMnemonic(t), ptrReg, reg)
	return b
}

// StoreIndirect emits a store of reg (a value of type t) to the address held in
// ptrReg — e.g. MOVD R3, (R2).
func (b *Builder) StoreIndirect(reg, ptrReg string, t Type) *Builder {
	b.fn.Insn("%s %s, (%s)", storeMnemonic(t), reg, ptrReg)
	return b
}

// Ret emits the return. On s390x ABI0 this is the same bare RET as the other
// leaf backends; the assembler expands it to the architecture's branch through
// the return-address register (R14).
func (b *Builder) Ret() *Builder { b.fn.Insn("RET"); return b }

func (b *Builder) Func() *emit.Function { return b.fn }

func (b *Builder) lookup(ps []Param, name string) Param {
	for _, p := range ps {
		if p.Name == name {
			return p
		}
	}
	panic(fmt.Sprintf("s390x: unknown param %q", name))
}
