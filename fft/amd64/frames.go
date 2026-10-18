package amd64

import (
	"strings"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// ---------------------------------------------------------------------------
// Two-pass transforms on the kernel's own stack frame (Round 32).
//
// A Stockham transform of two passes needs one n-point buffer between them.
// go-fft borrowed it from a sync.Pool, a round trip of about 16 ns, as long
// as a whole 64-point pass on Zen 3; a Go array on the stack would be zeroed
// on every call (1 KB at 64 points). TwoPass's name(cc, ch, tw *complex128,
// k0, k1 *float64, ido, l1 int, pass, last uintptr) keeps the buffer in its
// own frame, uninitialised: it calls the first pass's kernel, at entry point
// pass, as (cc, buf, tw, k0, ido, 1), then the final pass's kernel, at entry
// point last, as (buf, ch, k1, l1). The buffer starts on a 64-byte boundary
// and holds up to max points. The kernels are the ones a plan would call one
// after the other, with the same arguments, so the result is the same bits.
// The entry points are the kernels' ABI0 addresses (see AddrTable).

// TwoPass emits the two-pass trampoline (see above), whose frame holds a
// buffer of max complex128 points.
func TwoPass(f *emit.File, name string, max int) {
	names := []string{"cc", "ch", "tw", "k0", "k1", "ido", "l1", "pass", "last"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Int64, asm.Int64, asm.Uint64, asm.Uint64}
	// The frame: the callees' arguments at 0..47(SP), then the buffer from
	// the first 64-byte boundary at or above 48(SP).
	frame := (48 + 63 + 16*max + 7) &^ 7
	// No NOSPLIT: the assembler inserts the stack check for the frame.
	b := asm.NewFuncFlags(name, asm.Layout(names, types, nil, nil), frame, "")
	b.Raw("NO_LOCAL_POINTERS")
	buf := func(reg string) {
		b.Raw("LEAQ 111(SP), %s", reg).Raw("ANDQ $-64, %s", reg)
	}
	// The first pass: (cc, buf, tw, k0, ido, 1).
	b.LoadArg("cc", "AX").Raw("MOVQ AX, 0(SP)")
	buf("AX")
	b.Raw("MOVQ AX, 8(SP)")
	b.LoadArg("tw", "AX").Raw("MOVQ AX, 16(SP)")
	b.LoadArg("k0", "AX").Raw("MOVQ AX, 24(SP)")
	b.LoadArg("ido", "AX").Raw("MOVQ AX, 32(SP)")
	b.Raw("MOVQ $1, 40(SP)")
	b.LoadArg("pass", "AX").Raw("CALL AX")
	// The final pass: (buf, ch, k1, l1). The callee may have clobbered
	// every register.
	buf("AX")
	b.Raw("MOVQ AX, 0(SP)")
	b.LoadArg("ch", "AX").Raw("MOVQ AX, 8(SP)")
	b.LoadArg("k1", "AX").Raw("MOVQ AX, 16(SP)")
	b.LoadArg("l1", "AX").Raw("MOVQ AX, 24(SP)")
	b.LoadArg("last", "AX").Raw("CALL AX")
	b.Ret()
	f.Add(b.Func())
}

// Addr is one entry of an address table: the ABI0 address of Sym is stored
// in slot Slot.
type Addr struct {
	Sym  string
	Slot int
}

// AddrTable emits name(t *uintptr): it stores the ABI0 address of every
// entry's symbol at t[Slot], in the order given. A pass kernel's entry point
// is what TwoPass and ThreePass call, and Go code cannot take it: a func
// value of an assembly function is its ABIInternal wrapper.
func AddrTable(f *emit.File, name string, entries []Addr) {
	b := asm.NewFunc(name, asm.Layout([]string{"t"}, []asm.Type{asm.Ptr}, nil, nil), 0)
	b.LoadArg("t", "DI")
	for _, e := range entries {
		b.Raw("LEAQ ·%s(SB), AX", e.Sym).Raw("MOVQ AX, %d(DI)", 8*e.Slot)
	}
	b.Ret()
	f.Add(b.Func())
}

// Funcdata adds funcdata.h to the includes of a generated file, src. A file
// that holds TwoPass or ThreePass needs it, for NO_LOCAL_POINTERS (their
// frame holds the buffer and the callees' arguments, which are copies of
// their own).
func Funcdata(src string) string {
	return strings.Replace(src, "#include \"textflag.h\"\n", "#include \"textflag.h\"\n#include \"funcdata.h\"\n", 1)
}

// ThreePass emits name(cc, ch, x, tw0, tw1 *complex128, k0, k1, k2 *float64,
// ido0, ido1, l1b, l1c int, p0, p1, p2 uintptr), the three-pass counterpart
// of TwoPass, whose frame holds two buffers of max complex128 points: pass 0 (entry point p0) reads cc
// and writes x, pass 1 (p1) reads x and writes a buffer B in the frame, the
// final pass (p2) reads B and writes ch, with the arguments a plan would give
// each:
//
//	p0(cc, x, tw0, k0, ido0, 1)
//	p1(x, B, tw1, k1, ido1, l1b)
//	p2(B, ch, k2, l1c)
//
// x is ch when it does not alias cc (go-fft's out-of-place
// schedule for an odd pass count) or nil, which selects a second frame
// buffer B2 (in place, where pass 0 must not write over the input it is
// still reading). B and B2 start on 64-byte boundaries.
func ThreePass(f *emit.File, name string, max int) {
	names := []string{"cc", "ch", "x", "tw0", "tw1", "k0", "k1", "k2", "ido0", "ido1", "l1b", "l1c", "p0", "p1", "p2"}
	types := []asm.Type{asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr, asm.Ptr,
		asm.Int64, asm.Int64, asm.Int64, asm.Int64, asm.Uint64, asm.Uint64, asm.Uint64}
	// The callees' arguments at 0..47(SP), x at 48(SP), then B from the
	// first 64-byte boundary at or above 56(SP), then B2.
	b2 := 16 * max
	frame := (56 + 63 + 2*b2 + 7) &^ 7
	b := asm.NewFuncFlags(name, asm.Layout(names, types, nil, nil), frame, "")
	b.Raw("NO_LOCAL_POINTERS")
	buf := func(reg string) {
		b.Raw("LEAQ 119(SP), %s", reg).Raw("ANDQ $-64, %s", reg)
	}
	// x, or B2 when x is nil, kept at 48(SP).
	b.LoadArg("x", "AX").
		Raw("TESTQ AX, AX").
		Raw("JNZ havex")
	buf("AX")
	b.Raw("ADDQ $%d, AX", b2).
		Label("havex")
	b.Raw("MOVQ AX, 48(SP)")
	// Pass 0: (cc, x, tw0, k0, ido0, 1).
	b.LoadArg("cc", "BX").Raw("MOVQ BX, 0(SP)").
		Raw("MOVQ AX, 8(SP)")
	b.LoadArg("tw0", "BX").Raw("MOVQ BX, 16(SP)")
	b.LoadArg("k0", "BX").Raw("MOVQ BX, 24(SP)")
	b.LoadArg("ido0", "BX").Raw("MOVQ BX, 32(SP)")
	b.Raw("MOVQ $1, 40(SP)")
	b.LoadArg("p0", "AX").Raw("CALL AX")
	// Pass 1: (x, B, tw1, k1, ido1, l1b).
	b.Raw("MOVQ 48(SP), AX").Raw("MOVQ AX, 0(SP)")
	buf("AX")
	b.Raw("MOVQ AX, 8(SP)")
	b.LoadArg("tw1", "AX").Raw("MOVQ AX, 16(SP)")
	b.LoadArg("k1", "AX").Raw("MOVQ AX, 24(SP)")
	b.LoadArg("ido1", "AX").Raw("MOVQ AX, 32(SP)")
	b.LoadArg("l1b", "AX").Raw("MOVQ AX, 40(SP)")
	b.LoadArg("p1", "AX").Raw("CALL AX")
	// The final pass: (B, ch, k2, l1c).
	buf("AX")
	b.Raw("MOVQ AX, 0(SP)")
	b.LoadArg("ch", "AX").Raw("MOVQ AX, 8(SP)")
	b.LoadArg("k2", "AX").Raw("MOVQ AX, 16(SP)")
	b.LoadArg("l1c", "AX").Raw("MOVQ AX, 24(SP)")
	b.LoadArg("p2", "AX").Raw("CALL AX")
	b.Ret()
	f.Add(b.Func())
}
