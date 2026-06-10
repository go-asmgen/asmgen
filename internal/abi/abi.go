// Package abi models Go's ABI0 stack-based calling convention for the 64-bit
// architectures go-asmgen targets (register word size 8). It is deliberately
// architecture-independent: arm64 and riscv64 share these offset and alignment
// rules exactly, and differ only in register names and move mnemonics. Each
// architecture package re-exports these types and drives its own emit surface.
package abi

// WordSize is the ABI0 register word size on the 64-bit targets modelled here.
// The result area is aligned to it, and 8-byte values to it.
const WordSize = 8

// Type describes one ABI0 scalar parameter type. Size is in bytes (1, 2, 4, or
// 8). Float selects the floating-point register file. Signed controls whether a
// sub-word integer load sign- or zero-extends; it is ignored for floats and for
// 8-byte values.
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
	ArgsSize int // total args+rets region; end of the last result, not rounded
}

// Align rounds n up to the next multiple of to (a power of two).
func Align(n, to int) int { return (n + to - 1) &^ (to - 1) }

// Layout computes ABI0 FP offsets for the given argument and result types, in
// declaration order. Arguments are laid out first from offset 0, each at its
// natural alignment. The result area then begins at a word-aligned offset, and
// the frame size is the end of the last result with no further rounding — the
// exact offsets `go vet` asmdecl expects (e.g. func(a,b int16) int16 lays out
// a@0, b@2, ret@8, size 10).
func Layout(argNames []string, argTypes []Type, retNames []string, retTypes []Type) Signature {
	var sig Signature
	off := 0
	place := func(names []string, types []Type) []Param {
		ps := make([]Param, 0, len(types))
		for i, t := range types {
			off = Align(off, t.Size)
			ps = append(ps, Param{Name: names[i], Type: t, Offset: off})
			off += t.Size
		}
		return ps
	}
	sig.Args = place(argNames, argTypes)
	off = Align(off, WordSize)
	sig.Rets = place(retNames, retTypes)
	sig.ArgsSize = off
	return sig
}
