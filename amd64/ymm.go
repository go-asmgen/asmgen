package amd64

import "fmt"

// ----------------------------------------------------------------------------
// 256-bit integer vector ops (AVX2)
// ----------------------------------------------------------------------------

// YReg names one of the sixteen 256-bit registers Y0..Y15. The typed methods
// below take their operands in the order of the instruction's destination
// first (Intel order: dst, then the sources), and emit the reversed Plan 9
// operand order, so that a bitsliced kernel can be written as the C
// expression it comes from. A register outside Y0..Y15 panics at generation
// time, where a hand-typed "Y16" would only fail in the assembler.
type YReg uint8

// The sixteen YMM registers.
const (
	Y0 YReg = iota
	Y1
	Y2
	Y3
	Y4
	Y5
	Y6
	Y7
	Y8
	Y9
	Y10
	Y11
	Y12
	Y13
	Y14
	Y15
)

// String returns the Plan 9 name of the register.
func (y YReg) String() string {
	if y > Y15 {
		panic(fmt.Sprintf("amd64: invalid YMM register %d", uint8(y)))
	}
	return fmt.Sprintf("Y%d", uint8(y))
}

// vex3 emits "mnemonic b, a, dst": dst = a op b in Intel order.
func (b *Builder) vex3(mnemonic string, dst, x, y YReg) *Builder {
	b.yinsn("%s %s, %s, %s", mnemonic, y, x, dst)
	return b
}

// VPXOR emits dst = x ^ y over 256 bits.
func (b *Builder) VPXOR(dst, x, y YReg) *Builder { return b.vex3("VPXOR", dst, x, y) }

// VPAND emits dst = x & y over 256 bits.
func (b *Builder) VPAND(dst, x, y YReg) *Builder { return b.vex3("VPAND", dst, x, y) }

// VPOR emits dst = x | y over 256 bits.
func (b *Builder) VPOR(dst, x, y YReg) *Builder { return b.vex3("VPOR", dst, x, y) }

// VPANDN emits dst = ^x & y over 256 bits: the complement is of the FIRST
// source, as in the Intel manual (and a classic slip when writing x &^ y).
func (b *Builder) VPANDN(dst, x, y YReg) *Builder { return b.vex3("VPANDN", dst, x, y) }

// VPADDD emits eight 32-bit lane additions, dst = x + y.
func (b *Builder) VPADDD(dst, x, y YReg) *Builder { return b.vex3("VPADDD", dst, x, y) }

// VPCMPEQD emits dst = (x == y) per 32-bit lane; with x == y it is the
// all-ones register, the cheapest way to make one (see Ones).
func (b *Builder) VPCMPEQD(dst, x, y YReg) *Builder { return b.vex3("VPCMPEQD", dst, x, y) }

// Ones sets dst to all ones (VPCMPEQD dst, dst, dst). It reads dst, which the
// CPU treats as independent of its old value, so it never waits on it.
func (b *Builder) Ones(dst YReg) *Builder { return b.VPCMPEQD(dst, dst, dst) }

func checkShift32(n int) {
	if n < 0 || n > 31 {
		panic(fmt.Sprintf("amd64: 32-bit lane shift count %d out of 0..31", n))
	}
}

// VPSLLD emits dst = x << n on each 32-bit lane, 0 <= n <= 31.
func (b *Builder) VPSLLD(dst, x YReg, n int) *Builder {
	checkShift32(n)
	b.yinsn("VPSLLD $%d, %s, %s", n, x, dst)
	return b
}

// VPSRLD emits dst = x >> n (logical) on each 32-bit lane, 0 <= n <= 31.
func (b *Builder) VPSRLD(dst, x YReg, n int) *Builder {
	checkShift32(n)
	b.yinsn("VPSRLD $%d, %s, %s", n, x, dst)
	return b
}

// RotL32 emits dst = x rotated left by n bits on each 32-bit lane, 1 <= n <=
// 31, in three instructions (AVX2 has no vector rotate; AVX-512VL's VPROLD is
// not assumed): tmp = x << n; dst = x >> (32-n); dst |= tmp. tmp must differ
// from dst and from x, and dst may equal x.
func (b *Builder) RotL32(dst, x YReg, n int, tmp YReg) *Builder {
	if n < 1 || n > 31 {
		panic(fmt.Sprintf("amd64: 32-bit lane rotate count %d out of 1..31", n))
	}
	if tmp == dst || tmp == x {
		panic("amd64: RotL32 scratch register must differ from dst and x")
	}
	b.VPSLLD(tmp, x, n)
	b.VPSRLD(dst, x, 32-n)
	return b.VPOR(dst, dst, tmp)
}

// VMOVDQA emits a register-to-register copy, dst = src.
func (b *Builder) VMOVDQA(dst, src YReg) *Builder {
	b.yinsn("VMOVDQA %s, %s", src, dst)
	return b
}

// VMOVDQU emits an unaligned 256-bit load, dst = *mem. mem is a Plan 9
// operand such as "(AX)" or "32(SI)".
func (b *Builder) VMOVDQU(dst YReg, mem string) *Builder {
	b.yinsn("VMOVDQU %s, %s", mem, dst)
	return b
}

// VMOVDQUStore emits an unaligned 256-bit store, *mem = src.
func (b *Builder) VMOVDQUStore(mem string, src YReg) *Builder {
	b.yinsn("VMOVDQU %s, %s", src, mem)
	return b
}

// VPBROADCASTD emits dst = the 32-bit word at mem in all eight lanes.
func (b *Builder) VPBROADCASTD(dst YReg, mem string) *Builder {
	b.yinsn("VPBROADCASTD %s, %s", mem, dst)
	return b
}

// VPUNPCKLDQ interleaves the low two 32-bit words of each 128-bit half:
// dst = x0 y0 x1 y1 (per half).
func (b *Builder) VPUNPCKLDQ(dst, x, y YReg) *Builder { return b.vex3("VPUNPCKLDQ", dst, x, y) }

// VPUNPCKHDQ interleaves the high two 32-bit words of each half:
// dst = x2 y2 x3 y3.
func (b *Builder) VPUNPCKHDQ(dst, x, y YReg) *Builder { return b.vex3("VPUNPCKHDQ", dst, x, y) }

// VPUNPCKLQDQ interleaves the low 64-bit word of each half: dst = x.lo y.lo.
func (b *Builder) VPUNPCKLQDQ(dst, x, y YReg) *Builder { return b.vex3("VPUNPCKLQDQ", dst, x, y) }

// VPUNPCKHQDQ interleaves the high 64-bit word of each half: dst = x.hi y.hi.
func (b *Builder) VPUNPCKHQDQ(dst, x, y YReg) *Builder { return b.vex3("VPUNPCKHQDQ", dst, x, y) }

// Transpose4x32 transposes, in place and independently inside each 128-bit
// half, the 4x4 matrix of 32-bit words whose rows are a, b, c and d: on
// return a holds word 0 of the four rows, b word 1, c word 2 and d word 3
// (row r of the input is a half of a, b, c, d in that order, so afterwards
// lane j of a half of a is the old word 0 of row j). Loading eight 16-byte
// blocks as four 32-byte vectors and calling this puts word w of all eight
// blocks in one register, which is the layout a bitsliced cipher wants; the
// lane order is a fixed permutation of the blocks and applying the call
// again undoes it. t0 and t1 are scratch and must be distinct from a..d and
// from each other. Eight unpacks and two register copies.
func (b *Builder) Transpose4x32(a, bb, c, d, t0, t1 YReg) *Builder {
	regs := []YReg{a, bb, c, d, t0, t1}
	for i := range regs {
		for j := i + 1; j < len(regs); j++ {
			if regs[i] == regs[j] {
				panic("amd64: Transpose4x32 registers must be distinct")
			}
		}
	}
	b.VPUNPCKLDQ(t0, a, bb)  // a0 b0 a1 b1
	b.VPUNPCKHDQ(t1, a, bb)  // a2 b2 a3 b3
	b.VPUNPCKLDQ(a, c, d)    // c0 d0 c1 d1
	b.VPUNPCKHDQ(bb, c, d)   // c2 d2 c3 d3
	b.VPUNPCKLQDQ(c, t1, bb) // a2 b2 c2 d2
	b.VPUNPCKHQDQ(d, t1, bb) // a3 b3 c3 d3
	b.VPUNPCKHQDQ(t1, t0, a) // a1 b1 c1 d1
	b.VPUNPCKLQDQ(t0, t0, a) // a0 b0 c0 d0
	b.VMOVDQA(a, t0)
	return b.VMOVDQA(bb, t1)
}

// VZEROUPPER clears the upper halves of every YMM register, which a kernel
// must do before returning to code that may use legacy-SSE instructions.
func (b *Builder) VZEROUPPER() *Builder {
	b.yinsn("VZEROUPPER")
	return b
}

// yinsn formats like Insn but renders each YReg itself, so that an invalid
// register panics here: fmt would swallow a panic raised inside String and
// print it into the instruction text.
func (b *Builder) yinsn(format string, args ...any) {
	for i, a := range args {
		switch r := a.(type) {
		case YReg:
			args[i] = r.String()
		case ZReg:
			args[i] = r.String()
		}
	}
	b.fn.Insn(format, args...)
}
