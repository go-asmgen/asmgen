package gap

import "fmt"

// loong64: LSX/LASX float64 instructions.
//
// The fused multiply-add is missing from cmd/asm altogether (Go master,
// 2026-10-04; VMADDV/XVMADDV are the integer ones). The spelling proposed
// follows cmd/asm's own: an F-prefixed ISA mnemonic keeps its F (VFSQRTD for
// vfsqrt.d), and the operands go in the order the scalar FMADDD uses,
// FMADDD Fa, Fk, Fj, Fd for fd = fj*fk + fa.
//
// The broadcast load is there, spelt as an arrangement load (VMOVQ off(R),
// V.V2 is vldrepl.d; XVMOVQ ... X.V4 is xvldrepl.d), and has been since at
// least Go 1.26. Only its most negative offset, -2048, is refused, though the
// ISA's scaled signed 9-bit field holds it.

const loong64Source = "GNU as 2.43 (Debian, Loongson 3C5000L), read back with objdump"

func init() {
	a := &Arch{Name: "loong64", GOARCH: []string{"loong64"}, Testdata: "loong64enc1.s"}
	a.insns = append(a.insns, &Insn{
		Arch: "loong64", ISA: "xvfmadd.d", Plan9: "XVFMADDD X{3}, X{2}, X{1}, X{0}",
		Mask: 0xfff00000, Bits: 0x0a200000,
		Encode: func(ops ...int) uint32 { return r4(0x0a200000, ops[0], ops[1], ops[2], ops[3]) },
		Source: loong64Source,
		Golden: []Case{
			{[]int{0, 0, 0, 0}, 0x0a200000},     // xvfmadd.d $xr0,$xr0,$xr0,$xr0
			{[]int{3, 2, 1, 4}, 0x0a220443},     // xvfmadd.d $xr3,$xr2,$xr1,$xr4
			{[]int{31, 0, 0, 0}, 0x0a20001f},    // xvfmadd.d $xr31,$xr0,$xr0,$xr0
			{[]int{0, 31, 0, 0}, 0x0a2003e0},    // xvfmadd.d $xr0,$xr31,$xr0,$xr0
			{[]int{0, 0, 31, 0}, 0x0a207c00},    // xvfmadd.d $xr0,$xr0,$xr31,$xr0
			{[]int{0, 0, 0, 31}, 0x0a2f8000},    // xvfmadd.d $xr0,$xr0,$xr0,$xr31
			{[]int{17, 9, 26, 5}, 0x0a22e931},   // xvfmadd.d $xr17,$xr9,$xr26,$xr5
			{[]int{31, 31, 31, 31}, 0x0a2fffff}, // xvfmadd.d $xr31,$xr31,$xr31,$xr31
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "loong64", ISA: "vfmadd.d", Plan9: "VFMADDD V{3}, V{2}, V{1}, V{0}",
		Mask: 0xfff00000, Bits: 0x09200000,
		Encode: func(ops ...int) uint32 { return r4(0x09200000, ops[0], ops[1], ops[2], ops[3]) },
		Source: loong64Source,
		Golden: []Case{
			{[]int{0, 0, 0, 0}, 0x09200000},     // vfmadd.d $vr0,$vr0,$vr0,$vr0
			{[]int{3, 2, 1, 4}, 0x09220443},     // vfmadd.d $vr3,$vr2,$vr1,$vr4
			{[]int{31, 0, 0, 0}, 0x0920001f},    // vfmadd.d $vr31,$vr0,$vr0,$vr0
			{[]int{0, 31, 0, 0}, 0x092003e0},    // vfmadd.d $vr0,$vr31,$vr0,$vr0
			{[]int{0, 0, 31, 0}, 0x09207c00},    // vfmadd.d $vr0,$vr0,$vr31,$vr0
			{[]int{0, 0, 0, 31}, 0x092f8000},    // vfmadd.d $vr0,$vr0,$vr0,$vr31
			{[]int{17, 9, 26, 5}, 0x0922e931},   // vfmadd.d $vr17,$vr9,$vr26,$vr5
			{[]int{31, 31, 31, 31}, 0x092fffff}, // vfmadd.d $vr31,$vr31,$vr31,$vr31
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "loong64", ISA: "xvldrepl.d", Plan9: "XVMOVQ {2}(R{1}), X{0}.V4",
		Mask: 0xfff80000, Bits: 0x32100000,
		Encode: func(ops ...int) uint32 { return ldrepl(0x32100000, ops[0], ops[1], ops[2]) },
		Source: loong64Source,
		Note:   "cmd/asm assembles it, but refuses offset -2048",
		Golden: []Case{
			{[]int{0, 4, 0}, 0x32100080},      // xvldrepl.d $xr0,$r4,0
			{[]int{4, 4, 8}, 0x32100484},      // xvldrepl.d $xr4,$r4,8
			{[]int{31, 4, 0}, 0x3210009f},     // xvldrepl.d $xr31,$r4,0
			{[]int{0, 31, 0}, 0x321003e0},     // xvldrepl.d $xr0,$r31,0
			{[]int{7, 12, 2040}, 0x3213fd87},  // xvldrepl.d $xr7,$r12,2040
			{[]int{9, 5, -2048}, 0x321400a9},  // xvldrepl.d $xr9,$r5,-2048
			{[]int{3, 6, -8}, 0x3217fcc3},     // xvldrepl.d $xr3,$r6,-8
			{[]int{31, 31, 2040}, 0x3213ffff}, // xvldrepl.d $xr31,$r31,2040
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "loong64", ISA: "vldrepl.d", Plan9: "VMOVQ {2}(R{1}), V{0}.V2",
		Mask: 0xfff80000, Bits: 0x30100000,
		Encode: func(ops ...int) uint32 { return ldrepl(0x30100000, ops[0], ops[1], ops[2]) },
		Source: loong64Source,
		Note:   "cmd/asm assembles it, but refuses offset -2048",
		Golden: []Case{
			{[]int{0, 4, 0}, 0x30100080},      // vldrepl.d $vr0,$r4,0
			{[]int{4, 4, 8}, 0x30100484},      // vldrepl.d $vr4,$r4,8
			{[]int{31, 4, 0}, 0x3010009f},     // vldrepl.d $vr31,$r4,0
			{[]int{0, 31, 0}, 0x301003e0},     // vldrepl.d $vr0,$r31,0
			{[]int{7, 12, 2040}, 0x3013fd87},  // vldrepl.d $vr7,$r12,2040
			{[]int{9, 5, -2048}, 0x301400a9},  // vldrepl.d $vr9,$r5,-2048
			{[]int{3, 6, -8}, 0x3017fcc3},     // vldrepl.d $vr3,$r6,-8
			{[]int{31, 31, 2040}, 0x3013ffff}, // vldrepl.d $vr31,$r31,2040
		},
	})
	arches = append(arches, a)
}

// r4 encodes the four-register form: d, j, k, a from bit 0 up, five bits each.
func r4(base uint32, d, j, k, a int) uint32 {
	return base | vec(a)<<15 | vec(k)<<10 | vec(j)<<5 | vec(d)
}

// ldrepl encodes a doubleword broadcast load: a signed 9-bit offset in units
// of 8 bytes.
func ldrepl(base uint32, d, rj, off int) uint32 {
	if off%8 != 0 || off < -2048 || off > 2040 {
		panic(fmt.Sprintf("loong64: broadcast-load offset %d is not a multiple of 8 in [-2048, 2040]", off))
	}
	return base | uint32(off/8)&0x1ff<<10 | reg("loong64", "general", rj, 31)<<5 | vec(d)
}

func vec(r int) uint32 { return reg("loong64", "vector", r, 31) }
