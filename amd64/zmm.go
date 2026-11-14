package amd64

import "fmt"

// ----------------------------------------------------------------------------
// 512-bit integer vector ops (AVX-512F)
// ----------------------------------------------------------------------------

// ZReg names one of the thirty-two 512-bit registers Z0..Z31. Like the YReg
// methods, the typed methods below take the destination first (Intel order)
// and emit the reversed Plan 9 operand order, and refuse a register past Z31
// at generation time. Every instruction here is AVX-512F; gate the kernel with
// FeatureProbe(name, AVX512F).
type ZReg uint8

// The thirty-two ZMM registers.
const (
	Z0 ZReg = iota
	Z1
	Z2
	Z3
	Z4
	Z5
	Z6
	Z7
	Z8
	Z9
	Z10
	Z11
	Z12
	Z13
	Z14
	Z15
	Z16
	Z17
	Z18
	Z19
	Z20
	Z21
	Z22
	Z23
	Z24
	Z25
	Z26
	Z27
	Z28
	Z29
	Z30
	Z31
)

// String returns the Plan 9 name of the register.
func (z ZReg) String() string {
	if z > Z31 {
		panic(fmt.Sprintf("amd64: invalid ZMM register %d", uint8(z)))
	}
	return fmt.Sprintf("Z%d", uint8(z))
}

func (b *Builder) zvex3(mnemonic string, dst, x, y ZReg) *Builder {
	b.yinsn("%s %s, %s, %s", mnemonic, y, x, dst)
	return b
}

// VPXORD emits dst = x ^ y over 512 bits.
func (b *Builder) VPXORD(dst, x, y ZReg) *Builder { return b.zvex3("VPXORD", dst, x, y) }

// VPXORDBcst emits dst = x ^ broadcast(*mem), the 32-bit word at mem in all
// sixteen lanes (the embedded-broadcast form, no separate broadcast needed).
func (b *Builder) VPXORDBcst(dst, x ZReg, mem string) *Builder {
	b.yinsn("VPXORD.BCST %s, %s, %s", mem, x, dst)
	return b
}

// VPADDD512 emits sixteen 32-bit lane additions, dst = x + y.
func (b *Builder) VPADDD512(dst, x, y ZReg) *Builder { return b.zvex3("VPADDD", dst, x, y) }

// VPTERNLOGD emits dst = f(dst, x, y) for the three-input boolean function
// whose truth table is imm: the result bit is imm's bit number
// (dst<<2 | x<<1 | y), the numbering of Intel's VPTERNLOGD and of
// _mm512_ternarylogic_epi32(dst, x, y, imm). dst is read and written; copy
// first (VMOVDQA32) when its old value is still needed. For example imm 0x96
// is the three-way XOR and 0xca is dst ? x : y.
func (b *Builder) VPTERNLOGD(imm uint8, dst, x, y ZReg) *Builder {
	b.yinsn("VPTERNLOGD $0x%02x, %s, %s, %s", imm, y, x, dst)
	return b
}

func checkShift512(n int) {
	if n < 0 || n > 31 {
		panic(fmt.Sprintf("amd64: 32-bit lane shift count %d out of 0..31", n))
	}
}

// VPSLLD512 emits dst = x << n on each 32-bit lane, 0 <= n <= 31.
func (b *Builder) VPSLLD512(dst, x ZReg, n int) *Builder {
	checkShift512(n)
	b.yinsn("VPSLLD $%d, %s, %s", n, x, dst)
	return b
}

// VPSRLD512 emits dst = x >> n (logical) on each 32-bit lane, 0 <= n <= 31.
func (b *Builder) VPSRLD512(dst, x ZReg, n int) *Builder {
	checkShift512(n)
	b.yinsn("VPSRLD $%d, %s, %s", n, x, dst)
	return b
}

// VPROLD emits dst = x rotated left by n bits on each 32-bit lane, a single
// instruction (AVX-512F has the rotate AVX2 lacks), 1 <= n <= 31.
func (b *Builder) VPROLD(dst, x ZReg, n int) *Builder {
	if n < 1 || n > 31 {
		panic(fmt.Sprintf("amd64: 32-bit lane rotate count %d out of 1..31", n))
	}
	b.yinsn("VPROLD $%d, %s, %s", n, x, dst)
	return b
}

// VMOVDQA32 emits a register copy, dst = src.
func (b *Builder) VMOVDQA32(dst, src ZReg) *Builder {
	b.yinsn("VMOVDQA32 %s, %s", src, dst)
	return b
}

// VMOVDQU32 emits an unaligned 512-bit load, dst = *mem.
func (b *Builder) VMOVDQU32(dst ZReg, mem string) *Builder {
	b.yinsn("VMOVDQU32 %s, %s", mem, dst)
	return b
}

// VMOVDQU32Store emits an unaligned 512-bit store, *mem = src.
func (b *Builder) VMOVDQU32Store(mem string, src ZReg) *Builder {
	b.yinsn("VMOVDQU32 %s, %s", src, mem)
	return b
}

// VPBROADCASTD512 emits dst = the 32-bit word at mem in all sixteen lanes.
func (b *Builder) VPBROADCASTD512(dst ZReg, mem string) *Builder {
	b.yinsn("VPBROADCASTD %s, %s", mem, dst)
	return b
}

// VPUNPCKLDQ512 interleaves the low two 32-bit words of each 128-bit lane.
func (b *Builder) VPUNPCKLDQ512(dst, x, y ZReg) *Builder { return b.zvex3("VPUNPCKLDQ", dst, x, y) }

// VPUNPCKHDQ512 interleaves the high two 32-bit words of each 128-bit lane.
func (b *Builder) VPUNPCKHDQ512(dst, x, y ZReg) *Builder { return b.zvex3("VPUNPCKHDQ", dst, x, y) }

// VPUNPCKLQDQ512 interleaves the low 64-bit word of each 128-bit lane.
func (b *Builder) VPUNPCKLQDQ512(dst, x, y ZReg) *Builder { return b.zvex3("VPUNPCKLQDQ", dst, x, y) }

// VPUNPCKHQDQ512 interleaves the high 64-bit word of each 128-bit lane.
func (b *Builder) VPUNPCKHQDQ512(dst, x, y ZReg) *Builder { return b.zvex3("VPUNPCKHQDQ", dst, x, y) }

// Transpose4x32Z is Transpose4x32 at 512 bits: in each of the four 128-bit
// lanes the 4x4 matrix of 32-bit words whose rows are a, b, c, d is transposed
// in place, so loading sixteen 16-byte blocks as four 64-byte vectors puts word
// w of all sixteen blocks in one register. t0 and t1 are scratch; all six
// registers must be distinct.
func (b *Builder) Transpose4x32Z(a, bb, c, d, t0, t1 ZReg) *Builder {
	regs := []ZReg{a, bb, c, d, t0, t1}
	for i := range regs {
		for j := i + 1; j < len(regs); j++ {
			if regs[i] == regs[j] {
				panic("amd64: Transpose4x32Z registers must be distinct")
			}
		}
	}
	b.VPUNPCKLDQ512(t0, a, bb)
	b.VPUNPCKHDQ512(t1, a, bb)
	b.VPUNPCKLDQ512(a, c, d)
	b.VPUNPCKHDQ512(bb, c, d)
	b.VPUNPCKLQDQ512(c, t1, bb)
	b.VPUNPCKHQDQ512(d, t1, bb)
	b.VPUNPCKHQDQ512(t1, t0, a)
	b.VPUNPCKLQDQ512(t0, t0, a)
	b.VMOVDQA32(a, t0)
	return b.VMOVDQA32(bb, t1)
}
