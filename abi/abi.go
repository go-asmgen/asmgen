// Package abi models Go's ABI0 stack-based calling convention for the 64-bit
// architectures go-asmgen targets (register word size 8). It is deliberately
// architecture-independent: arm64 and riscv64 share these offset and alignment
// rules exactly, and differ only in register names and move mnemonics. Each
// architecture package re-exports these types and drives its own emit surface.
package abi

import "strconv"

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
	args := make([]Arg, len(argTypes))
	for i, t := range argTypes {
		args[i] = Scalar(argNames[i], t)
	}
	rets := make([]Arg, len(retTypes))
	for i, t := range retTypes {
		rets[i] = Scalar(retNames[i], t)
	}
	return LayoutArgs(args, rets)
}

// ----------------------------------------------------------------------------
// Aggregates
//
// A struct/slice/string parameter occupies a contiguous region laid out by Go's
// struct rules: fields in declaration order, each at its natural alignment; the
// region is aligned to the aggregate's alignment (its largest field) and sized
// up to a multiple of it. In hand-written assembly each leaf field is addressed
// as `param_field+offset(FP)`, which is exactly what asmdecl checks — so Layout
// flattens an aggregate into one Param per field with that conventional name.
// ----------------------------------------------------------------------------

// Field is one member of an aggregate parameter.
type Field struct {
	Name string
	Type Type
}

// Arg describes one ABI0 parameter for LayoutArgs: a scalar (Type set, Fields
// nil) or an aggregate (Fields set — a struct, slice, or string).
type Arg struct {
	Name   string
	Type   Type
	Fields []Field
}

// Scalar constructs a scalar argument.
func Scalar(name string, t Type) Arg { return Arg{Name: name, Type: t} }

// Struct constructs an aggregate argument from explicit fields. Field names must
// match the Go struct's exported field names so asmdecl accepts `name_Field`.
func Struct(name string, fields ...Field) Arg { return Arg{Name: name, Fields: fields} }

// Slice constructs a []T argument: the Go slice header {base, len, cap},
// addressed as name_base / name_len / name_cap.
func Slice(name string) Arg {
	return Arg{Name: name, Fields: []Field{
		{Name: "base", Type: Ptr},
		{Name: "len", Type: Int64},
		{Name: "cap", Type: Int64},
	}}
}

// String constructs a string argument: the Go string header {base, len},
// addressed as name_base / name_len.
func String(name string) Arg {
	return Arg{Name: name, Fields: []Field{
		{Name: "base", Type: Ptr},
		{Name: "len", Type: Int64},
	}}
}

// Array constructs an [n]elem array argument. Its elements are laid out
// contiguously and addressed as name_0 .. name_(n-1) — matching how asmdecl
// names array elements. The whole array is also a valid component at name+0(FP)
// (asmdecl appends a whole-value component for every type), which is what a
// single vector load/store targets.
func Array(name string, elem Type, n int) Arg {
	fields := make([]Field, n)
	for i := range fields {
		fields[i] = Field{Name: strconv.Itoa(i), Type: elem}
	}
	return Arg{Name: name, Fields: fields}
}

// aggregateAlign is the alignment of an aggregate: that of its widest field.
func aggregateAlign(fields []Field) int {
	a := 1
	for _, f := range fields {
		if f.Type.Size > a {
			a = f.Type.Size
		}
	}
	return a
}

// LayoutArgs computes ABI0 FP offsets for arbitrary scalar and aggregate
// arguments and results, flattening each aggregate into one Param per field
// named `arg_field`. Result area is word-aligned; the frame is sized to the end
// of the last slot with no further rounding.
func LayoutArgs(args, rets []Arg) Signature {
	var sig Signature
	off := 0
	place := func(in []Arg) []Param {
		var ps []Param
		for _, a := range in {
			if a.Fields == nil {
				off = Align(off, a.Type.Size)
				ps = append(ps, Param{Name: a.Name, Type: a.Type, Offset: off})
				off += a.Type.Size
				continue
			}
			al := aggregateAlign(a.Fields)
			off = Align(off, al)
			fieldOff := 0
			for _, f := range a.Fields {
				fieldOff = Align(fieldOff, f.Type.Size)
				ps = append(ps, Param{Name: a.Name + "_" + f.Name, Type: f.Type, Offset: off + fieldOff})
				fieldOff += f.Type.Size
			}
			off += Align(fieldOff, al) // struct size, with trailing padding
		}
		return ps
	}
	sig.Args = place(args)
	off = Align(off, WordSize)
	sig.Rets = place(rets)
	sig.ArgsSize = off
	return sig
}

// Slot returns the laid-out Param with the given name (a scalar's name, or a
// flattened field/element such as "s_base" or "v_0") and whether it was found.
// It is handy for reading an aggregate's base offset — e.g. an array's whole
// value begins at the offset of its element 0 — when emitting a vector
// load/store of the whole value through Raw.
func (s Signature) Slot(name string) (Param, bool) {
	for _, p := range s.Args {
		if p.Name == name {
			return p, true
		}
	}
	for _, p := range s.Rets {
		if p.Name == name {
			return p, true
		}
	}
	return Param{}, false
}
