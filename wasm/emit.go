// Package wasm is a WAT (WebAssembly text) emitter, peer of the amd64 /
// arm64 / riscv64 / loong64 / ppc64 / s390x packages in this module.
// Where those build Plan 9 assembly for the six 64-bit Go arches, this
// package builds the seventh target: wasm-SIMD (v128) module text that
// wat2wasm compiles into .wasm bytes for //go:wasmimport consumption
// from Go host code.
//
// It is deliberately dumb: it knows how to build a well-formed WAT
// module and function, nothing about any specific kernel. A per-kernel
// builder drives the surface — see examples/wasm/{matchlen, hex,
// popcount, toupper, memchr, isascii, utf8len, hex_decode, json_clean}
// for the shipped nine.
package wasm

import (
	"fmt"
	"strings"
)

// ValueType is a wasm value type: i32, i64, f32, f64, v128.
type ValueType string

const (
	I32  ValueType = "i32"
	I64  ValueType = "i64"
	F32  ValueType = "f32"
	F64  ValueType = "f64"
	V128 ValueType = "v128"
)

// Param is a wasm function parameter (name, type). Local variables have the
// same shape but different semantics.
type Param struct {
	Name string
	Type ValueType
}

// Function accumulates the body of one WAT `(func $name (export "name") ...)`
// block. It is stack-based: emit ops in evaluation order and each op pops its
// operands from the top of the stack and pushes its result.
type Function struct {
	Name    string
	Export  string // export name; empty = not exported
	Params  []Param
	Results []ValueType
	Locals  []Param
	body    []string
}

// NewFunction starts a function. Pass export = "" for a private helper.
func NewFunction(name, export string, params []Param, results []ValueType) *Function {
	return &Function{Name: name, Export: export, Params: params, Results: results}
}

// Local appends a local variable of the given type. Returns the local's name so
// callers can reference it (`local.get`, `local.set`, `local.tee`).
func (f *Function) Local(name string, t ValueType) string {
	f.Locals = append(f.Locals, Param{Name: name, Type: t})
	return "$" + name
}

// Raw appends a raw WAT s-expression line, indented one level inside the func.
// This is the low-level escape hatch — higher-level helpers below (Op, Block,
// Loop, If, BrIf) call it too.
func (f *Function) Raw(format string, args ...any) {
	f.body = append(f.body, "    "+fmt.Sprintf(format, args...))
}

// Op is a stack-op. Its operands must already be on the stack (via previous
// Op calls, LocalGet, or nested s-expressions).
func (f *Function) Op(op string, args ...any) {
	if len(args) > 0 {
		f.Raw("(%s %v)", op, fmt.Sprint(args...))
		return
	}
	f.Raw("(%s)", op)
}

// LocalGet pushes the local's value on the stack.
func (f *Function) LocalGet(name string) { f.Raw("(local.get $%s)", name) }

// LocalSet pops from the stack and stores into the local.
func (f *Function) LocalSet(name string) { f.Raw("(local.set $%s)", name) }

// LocalTee is LocalSet + LocalGet (pop, store, push).
func (f *Function) LocalTee(name string) { f.Raw("(local.tee $%s)", name) }

// I32Const pushes a constant i32.
func (f *Function) I32Const(v int32) { f.Raw("(i32.const %d)", v) }

// v128 ops — the wasm-SIMD subset the kernels here need.

// V128Load pushes a v128 read from memory at (top-of-stack i32 address). Offset
// and align are the same fields WAT accepts.
func (f *Function) V128Load(offset int) { f.Raw("(v128.load offset=%d)", offset) }

// V128Store pops (addr:i32) and (v:v128); writes v to memory at addr+offset.
func (f *Function) V128Store(offset int) { f.Raw("(v128.store offset=%d)", offset) }

// V128Store64Lane pops (v:v128) and (addr:i32); writes 8 bytes from the
// selected i64 lane (0 or 1) of v to memory at addr+offset. Used when a
// kernel packs its result into the low half of a v128 (e.g. hex_decode's
// 16-char → 8-byte compression) and only wants those 8 bytes stored.
func (f *Function) V128Store64Lane(offset, lane int) {
	if lane < 0 || lane > 1 {
		panic("V128Store64Lane needs lane 0..1")
	}
	f.Raw("(v128.store64_lane offset=%d %d)", offset, lane)
}

// I32Store pops (v:i32) and (addr:i32); writes v to memory at addr+offset.
// Used when a kernel returns multiple i32 values via caller-provided output
// pointers (adler32 writes back the running (a, b) pair this way).
func (f *Function) I32Store(offset int) { f.Raw("(i32.store offset=%d)", offset) }

// V128Const16 pushes a v128 constant built from 16 immediate bytes. Uses the
// (v128.const i8x16 ...) shape so byte-lane-oriented constants (nibble LUTs,
// masks) read naturally.
func (f *Function) V128Const16(b []byte) {
	if len(b) != 16 {
		panic("V128Const16 needs exactly 16 bytes")
	}
	var sb strings.Builder
	sb.WriteString("(v128.const i8x16")
	for _, v := range b {
		fmt.Fprintf(&sb, " %d", int8(v))
	}
	sb.WriteString(")")
	f.Raw("%s", sb.String())
}

// V128And / V128Or / V128Xor are the bitwise ops. Every SIMD kernel here
// uses V128And to mask a lane range (e.g. the low-nibble mask 0x0f × 16).
func (f *Function) V128And() { f.Raw("(v128.and)") }
func (f *Function) V128Or()  { f.Raw("(v128.or)") }
func (f *Function) V128Xor() { f.Raw("(v128.xor)") }

// I8x16Eq pops two v128, pushes a per-byte equality mask v128.
func (f *Function) I8x16Eq() { f.Raw("(i8x16.eq)") }

// I8x16GeS / I8x16LeS pop two v128 and push a per-byte signed-compare mask
// (all-ones where a >= b or a <= b respectively). Used for byte-range checks
// in text-processing kernels — a ge_s('a') + le_s('z') pair identifies
// lowercase ASCII lanes.
func (f *Function) I8x16GeS() { f.Raw("(i8x16.ge_s)") }
func (f *Function) I8x16LeS() { f.Raw("(i8x16.le_s)") }

// I8x16Sub pops (b:v128) and (a:v128); pushes a - b per lane (with modular
// wraparound). Used for the ASCII case-fold "subtract 32 where mask" pattern.
func (f *Function) I8x16Sub() { f.Raw("(i8x16.sub)") }

// I8x16Splat pops an i32 and pushes a v128 with the low byte broadcast to
// all 16 lanes. Handy for building a search-key vector from a runtime
// argument (e.g. the byte to look for in memchr / bytes.IndexByte).
func (f *Function) I8x16Splat() { f.Raw("(i8x16.splat)") }

// I8x16Bitmask pops a v128 and pushes an i32 whose bit i is the MSB of
// lane i (0..15) — wasm-SIMD's SSE-pmovmskb analogue. Combined with
// i32.ctz this locates the index of the first "true" lane in a compare
// mask, the core of memchr / bytes.IndexByte.
func (f *Function) I8x16Bitmask() { f.Raw("(i8x16.bitmask)") }

// I32Ctz pops an i32 and pushes its count of trailing zero bits (32 for
// zero input). Paired with i8x16.bitmask to find the first set lane.
func (f *Function) I32Ctz() { f.Raw("(i32.ctz)") }

// I32Sub pops (b:i32) and (a:i32); pushes a - b.
func (f *Function) I32Sub() { f.Raw("(i32.sub)") }

// I32Popcnt pops an i32 and pushes its population count (set-bit count).
// Wasm's scalar popcnt op — the natural reducer for i8x16.bitmask's 16-bit
// output, giving "how many lanes were true" without a scalar loop. utf8len
// uses this to count non-continuation bytes per block.
func (f *Function) I32Popcnt() { f.Raw("(i32.popcnt)") }

// I8x16AllTrue pops one v128, pushes i32 1 if every lane is non-zero, else 0.
func (f *Function) I8x16AllTrue() { f.Raw("(i8x16.all_true)") }

// I8x16Swizzle pops (indices:v128) and (table:v128); pushes a v128 where each
// output byte i is table[indices[i]] if indices[i] < 16, else 0. Direct
// analogue of SSSE3 PSHUFB. Used for nibble→ASCII LUT in hex/base64/base32.
func (f *Function) I8x16Swizzle() { f.Raw("(i8x16.swizzle)") }

// I8x16Shuffle pops two v128 (a, b) and pushes a v128 where output lane i is
// a[imm[i]] if imm[i] < 16 else b[imm[i]-16]. The 16 lane-picker imms are
// compile-time constants — SSSE3 PUNPCKL/HBW analogues use fixed patterns.
func (f *Function) I8x16Shuffle(imm []byte) {
	if len(imm) != 16 {
		panic("I8x16Shuffle needs 16 immediates")
	}
	var sb strings.Builder
	sb.WriteString("(i8x16.shuffle")
	for _, v := range imm {
		fmt.Fprintf(&sb, " %d", v)
	}
	sb.WriteString(")")
	f.Raw("%s", sb.String())
}

// I8x16ShrU pops (v:v128) and (shift:i32); pushes per-lane unsigned right
// shift. Note wasm-SIMD's byte-shift takes the count from an i32 on the stack,
// unlike SSE PSRLW that word-shifts and needs an AND fixup — this is the
// "correct byte shift" the kernels want.
func (f *Function) I8x16ShrU() { f.Raw("(i8x16.shr_u)") }

// I8x16Shl pops (v:v128) and (shift:i32); pushes per-lane left shift.
// Symmetric to I8x16ShrU — used by hex_decode to shift high nibbles into
// the top half of packed bytes before OR-ing with low nibbles.
func (f *Function) I8x16Shl() { f.Raw("(i8x16.shl)") }

// I8x16Popcnt pops a v128 and pushes a v128 where each output lane is the
// popcount of the corresponding input byte. This is the wasm-SIMD op with no
// direct 128-bit analogue in SSE/NEON pre-AVX-512 — it's the reason popcount
// on wasm can be tight even without a wide reduction network. Combined with
// horizontal reduction ops (extadd_pairwise, i32x4.extract_lane), a whole-
// buffer popcount fits in ~5 v128 ops per 16 input bytes.
func (f *Function) I8x16Popcnt() { f.Raw("(i8x16.popcnt)") }

// I16x8ExtaddPairwiseI8x16U pops a v128 (interpreted as 16 u8 lanes) and
// pushes a v128 (interpreted as 8 u16 lanes) where each output lane is the
// sum of two adjacent input lanes. Used to widen a per-byte popcount into a
// wider accumulator before final sum.
func (f *Function) I16x8ExtaddPairwiseI8x16U() { f.Raw("(i16x8.extadd_pairwise_i8x16_u)") }

// I32x4ExtaddPairwiseI16x8U pairs u16 lanes into u32 lanes (analogous op for
// the next stage of a horizontal reduction).
func (f *Function) I32x4ExtaddPairwiseI16x8U() { f.Raw("(i32x4.extadd_pairwise_i16x8_u)") }

// I16x8ExtmulLowI8x16U / I16x8ExtmulHighI8x16U pop two v128 (interpreted as
// 16 u8 lanes each), widen the low / high 8 lanes to u16, multiply pairwise,
// and push a v128 of 8 u16 products. These are the wasm-SIMD alternative to
// PMULLW+PMULHW that adler32 uses for the weighted-byte-sum step (max
// product 255*16=4080 fits in u16 so no overflow to worry about).
func (f *Function) I16x8ExtmulLowI8x16U()  { f.Raw("(i16x8.extmul_low_i8x16_u)") }
func (f *Function) I16x8ExtmulHighI8x16U() { f.Raw("(i16x8.extmul_high_i8x16_u)") }

// I16x8Mul pops two v128 (as 8 i16 lanes each) and pushes their per-lane
// product truncated to i16 — the wasm-SIMD equivalent of PMULLW. Used by
// base64 encode's Lemire-style 6-bit-index extraction, where each i16
// lane multiplies against a constant that shifts the useful bits into
// place in the low 16 bits of the product.
func (f *Function) I16x8Mul() { f.Raw("(i16x8.mul)") }

// I32x4ExtmulLowI16x8U / I32x4ExtmulHighI16x8U pop two v128 (as 8 u16
// lanes each), widen the low / high 4 lanes to u32, multiply pairwise,
// and push a v128 of 4 u32 products. Combined with I32x4ShrU and
// I16x8NarrowI32x4U these emulate PMULHUW (unsigned high-word multiply)
// — take the widening product, shift right 16, narrow back to i16x8 —
// which is the "extract the top 6 bits into position" primitive Lemire's
// base64 encoder needs.
func (f *Function) I32x4ExtmulLowI16x8U()  { f.Raw("(i32x4.extmul_low_i16x8_u)") }
func (f *Function) I32x4ExtmulHighI16x8U() { f.Raw("(i32x4.extmul_high_i16x8_u)") }

// I32x4ShrU pops (v:v128) and (shift:i32); pushes per-i32-lane unsigned
// right shift. Used in the PMULHUW emulation (shift right 16 to keep
// only the high word of the widened product).
func (f *Function) I32x4ShrU() { f.Raw("(i32x4.shr_u)") }

// I16x8NarrowI32x4U pops two v128 (as 4 i32 lanes each) and packs their
// low 16 bits into a single 8-lane i16x8 via unsigned saturation. Used
// to close the PMULHUW emulation: after shifting u32 products right 16,
// we narrow the two halves back into a single i16x8.
func (f *Function) I16x8NarrowI32x4U() { f.Raw("(i16x8.narrow_i32x4_u)") }

// I8x16GtU / I8x16LtU pop two v128 and push a per-byte unsigned-compare
// mask (all-ones where a > b or a < b respectively). Used for range
// checks on bytes with the high bit set (which the signed compares in
// json_clean's docstring warn about).
func (f *Function) I8x16GtU() { f.Raw("(i8x16.gt_u)") }
func (f *Function) I8x16LtU() { f.Raw("(i8x16.lt_u)") }

// I8x16Add pops two v128 (as 16 i8 lanes each) and pushes their per-lane
// sum (with modular wraparound). Base64 encode's range-add offset step
// uses this: start with (index + 65), then add per-range corrections
// masked by the range predicates.
func (f *Function) I8x16Add() { f.Raw("(i8x16.add)") }

// I32x4Add pops two v128 (interpreted as 4 i32 lanes) and pushes their
// per-lane sum. Used to accumulate widened popcount results across blocks.
func (f *Function) I32x4Add() { f.Raw("(i32x4.add)") }

// I32x4ExtractLane pops a v128 and pushes lane[imm] (imm ∈ 0..3) as i32. The
// four extract-lane calls + three i32.add build a horizontal sum of the four
// u32 accumulator lanes at the tail of a reduction.
func (f *Function) I32x4ExtractLane(lane int) {
	if lane < 0 || lane > 3 {
		panic("I32x4ExtractLane needs lane 0..3")
	}
	f.Raw("(i32x4.extract_lane %d)", lane)
}

// V128Zero pushes an all-zero v128 constant. Common initial accumulator.
func (f *Function) V128Zero() { f.Raw("(v128.const i32x4 0 0 0 0)") }

// I32Eqz / I32Add / I32Load8U / I32GtS / I32GeS / I32Ne / I32Eq — the tiny
// integer op subset the matchlen tail loop and bounds tests need.
func (f *Function) I32Eqz()    { f.Raw("(i32.eqz)") }
func (f *Function) I32Add()    { f.Raw("(i32.add)") }
func (f *Function) I32Mul()    { f.Raw("(i32.mul)") }
func (f *Function) I32Load8U() { f.Raw("(i32.load8_u)") }
func (f *Function) I32GtS()    { f.Raw("(i32.gt_s)") }
func (f *Function) I32GeS()    { f.Raw("(i32.ge_s)") }
func (f *Function) I32Ne()     { f.Raw("(i32.ne)") }

// Block / Loop / If / BrIf — enough control-flow to build the matchlen SIMD
// chunk loop and the byte tail loop.
type Label struct{ name string }

func (f *Function) Block(name string) Label { f.Raw("(block $%s", name); return Label{name} }
func (f *Function) Loop(name string) Label  { f.Raw("(loop $%s", name); return Label{name} }
func (f *Function) End()                    { f.Raw(")") }

// BrIf branches to the named label if top-of-stack i32 is non-zero.
func (f *Function) BrIf(label Label) { f.Raw("(br_if $%s)", label.name) }

// Br unconditionally branches to the named label.
func (f *Function) Br(label Label) { f.Raw("(br $%s)", label.name) }

// Return terminates the current function, returning whatever value(s) sit
// on the stack. Used by memchr to bail out of the scan loop on the first
// match without unwinding through the enclosing (block $done).
func (f *Function) Return() { f.Raw("(return)") }

// String renders the complete `(func ...)` block.
func (f *Function) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  (func $%s", f.Name)
	if f.Export != "" {
		fmt.Fprintf(&b, ` (export "%s")`, f.Export)
	}
	for _, p := range f.Params {
		fmt.Fprintf(&b, " (param $%s %s)", p.Name, p.Type)
	}
	for _, r := range f.Results {
		fmt.Fprintf(&b, " (result %s)", r)
	}
	b.WriteString("\n")
	for _, l := range f.Locals {
		fmt.Fprintf(&b, "    (local $%s %s)\n", l.Name, l.Type)
	}
	for _, line := range f.body {
		b.WriteString(line + "\n")
	}
	b.WriteString("  )\n")
	return b.String()
}

// Module is a whole WAT module: imports + functions.
type Module struct {
	imports []string
	funcs   []*Function
}

func NewModule() *Module { return &Module{} }

// ImportMemory declares (import "namespace" "name" (memory 0)). Every kernel
// shares its host's linear memory this way.
func (m *Module) ImportMemory(namespace, name string) {
	m.imports = append(m.imports, fmt.Sprintf(`  (import "%s" "%s" (memory 0))`, namespace, name))
}

func (m *Module) Add(fn *Function) { m.funcs = append(m.funcs, fn) }

func (m *Module) String() string {
	var b strings.Builder
	b.WriteString(";; Code generated by go-asmgen/asmgen/wasm. DO NOT EDIT.\n\n")
	b.WriteString("(module\n")
	for _, imp := range m.imports {
		b.WriteString(imp + "\n")
	}
	for _, fn := range m.funcs {
		b.WriteString(fn.String())
	}
	b.WriteString(")\n")
	return b.String()
}
