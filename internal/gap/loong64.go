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

const loong64Source = "GNU as 2.43 (Debian, Loongson 3C5000L), read back with objdump; llvm-mc 22.1.5 agrees on every case"

// loong64SourceS is the source of the float32 (.s) fused multiply-adds: the
// Loongson host was unreachable when they were added, so they come from llvm-mc,
// which in the same run reproduced the GNU as words held for vfmadd.d and
// xvfmadd.d, and match the cases goasm-patches/loong64-vfmadd-vldrepl.patch
// mailed for VFMADDF and XVFMADDF.
const loong64SourceS = "llvm-mc 23.1.2 --triple=loongarch64 -mattr=+lasx --show-encoding (control: the vfmadd.d/xvfmadd.d GNU as words reproduce in the same run)"

func init() {
	a := &Arch{Name: "loong64", GOARCH: []string{"loong64"}, Testdata: "loong64enc1.s", TabbedTestdata: true}
	a.insns = append(a.insns, &Insn{
		Arch: "loong64", ISA: "vfmadd.d", Plan9: "VFMADDD V{3}, V{2}, V{1}, V{0}",
		Mask: 0xfff00000, Bits: 0x09200000,
		Encode: func(ops ...int) uint64 { return r4(0x09200000, ops[0], ops[1], ops[2], ops[3]) },
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
		Arch: "loong64", ISA: "xvfmadd.d", Plan9: "XVFMADDD X{3}, X{2}, X{1}, X{0}",
		Mask: 0xfff00000, Bits: 0x0a200000,
		Encode: func(ops ...int) uint64 { return r4(0x0a200000, ops[0], ops[1], ops[2], ops[3]) },
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
		Arch: "loong64", ISA: "vfmadd.s", Plan9: "VFMADDF V{3}, V{2}, V{1}, V{0}",
		Mask: 0xfff00000, Bits: 0x09100000,
		Encode: func(ops ...int) uint64 { return r4(0x09100000, ops[0], ops[1], ops[2], ops[3]) },
		Source: loong64SourceS,
		Golden: []Case{
			{[]int{0, 0, 0, 0}, 0x09100000},     // vfmadd.s $vr0,$vr0,$vr0,$vr0
			{[]int{3, 2, 1, 4}, 0x09120443},     // vfmadd.s $vr3,$vr2,$vr1,$vr4
			{[]int{31, 0, 0, 0}, 0x0910001f},    // vfmadd.s $vr31,$vr0,$vr0,$vr0
			{[]int{0, 31, 0, 0}, 0x091003e0},    // vfmadd.s $vr0,$vr31,$vr0,$vr0
			{[]int{0, 0, 31, 0}, 0x09107c00},    // vfmadd.s $vr0,$vr0,$vr31,$vr0
			{[]int{0, 0, 0, 31}, 0x091f8000},    // vfmadd.s $vr0,$vr0,$vr0,$vr31
			{[]int{17, 9, 26, 5}, 0x0912e931},   // vfmadd.s $vr17,$vr9,$vr26,$vr5
			{[]int{31, 31, 31, 31}, 0x091fffff}, // vfmadd.s $vr31,$vr31,$vr31,$vr31
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "loong64", ISA: "xvfmadd.s", Plan9: "XVFMADDF X{3}, X{2}, X{1}, X{0}",
		Mask: 0xfff00000, Bits: 0x0a100000,
		Encode: func(ops ...int) uint64 { return r4(0x0a100000, ops[0], ops[1], ops[2], ops[3]) },
		Source: loong64SourceS,
		Golden: []Case{
			{[]int{0, 0, 0, 0}, 0x0a100000},     // xvfmadd.s $xr0,$xr0,$xr0,$xr0
			{[]int{3, 2, 1, 4}, 0x0a120443},     // xvfmadd.s $xr3,$xr2,$xr1,$xr4
			{[]int{31, 0, 0, 0}, 0x0a10001f},    // xvfmadd.s $xr31,$xr0,$xr0,$xr0
			{[]int{0, 31, 0, 0}, 0x0a1003e0},    // xvfmadd.s $xr0,$xr31,$xr0,$xr0
			{[]int{0, 0, 31, 0}, 0x0a107c00},    // xvfmadd.s $xr0,$xr0,$xr31,$xr0
			{[]int{0, 0, 0, 31}, 0x0a1f8000},    // xvfmadd.s $xr0,$xr0,$xr0,$xr31
			{[]int{17, 9, 26, 5}, 0x0a12e931},   // xvfmadd.s $xr17,$xr9,$xr26,$xr5
			{[]int{31, 31, 31, 31}, 0x0a1fffff}, // xvfmadd.s $xr31,$xr31,$xr31,$xr31
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "loong64", ISA: "vldrepl.d", Plan9: "VMOVQ {2}(R{1}), V{0}.V2",
		Mask: 0xfff80000, Bits: 0x30100000,
		Encode: func(ops ...int) uint64 { return ldrepl(0x30100000, ops[0], ops[1], ops[2]) },
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
	a.insns = append(a.insns, &Insn{
		Arch: "loong64", ISA: "xvldrepl.d", Plan9: "XVMOVQ {2}(R{1}), X{0}.V4",
		Mask: 0xfff80000, Bits: 0x32100000,
		Encode: func(ops ...int) uint64 { return ldrepl(0x32100000, ops[0], ops[1], ops[2]) },
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
	a.insns = append(a.insns, csrInsns()...)
	arches = append(arches, a)
}

// loong64: the privileged CSR access instructions (LoongArch Reference
// Manual, Volume 1, §4.2.1 "CSR Access Instructions"). cmd/asm has none of
// them (Go master, 2026-10-25), so tamago's loong64 bare-metal port writes
// csrwr as a WORD. The spelling reverses the ISA operands as cmd/asm does,
// destination last and the CSR number as a constant: csrxchg rd, rj, csr is
// CSRXCHG $csr, Rj, Rd.
//
// The three share one major opcode: rj = 0 is csrrd, rj = 1 csrwr, any other
// rj csrxchg, which is why csrxchg needs NotMask and refuses rj 0 and 1.

const csrSource = "GNU as 2.44 (Debian trixie binutils-loongarch64-linux-gnu), read back with objdump; llvm-mc 23.1.2 agrees on every case"

func csrInsns() []*Insn {
	return []*Insn{{
		Arch: "loong64", ISA: "csrrd", Plan9: "CSRRD ${1}, R{0}",
		Mask: 0xff0003e0, Bits: 0x04000000,
		Encode: func(ops ...int) uint64 { return csr(0x04000000, ops[0], 0, ops[1]) },
		Source: csrSource,
		Golden: []Case{
			{[]int{0, 0}, 0x04000000},       // csrrd $zero, 0x0
			{[]int{4, 1}, 0x04000404},       // csrrd $a0, 0x1
			{[]int{31, 0}, 0x0400001f},      // csrrd $s8, 0x0
			{[]int{0, 0x3fff}, 0x04fffc00},  // csrrd $zero, 0x3fff
			{[]int{12, 0x180}, 0x0406000c},  // csrrd $t0, 0x180
			{[]int{31, 0x3fff}, 0x04fffc1f}, // csrrd $s8, 0x3fff
		},
	}, {
		Arch: "loong64", ISA: "csrwr", Plan9: "CSRWR ${1}, R{0}",
		Mask: 0xff0003e0, Bits: 0x04000020,
		Encode: func(ops ...int) uint64 { return csr(0x04000020, ops[0], 0, ops[1]) },
		Source: csrSource,
		Golden: []Case{
			{[]int{0, 0}, 0x04000020},       // csrwr $zero, 0x0
			{[]int{19, 2}, 0x04000833},      // csrwr $t7, 0x2 (tamago-uefi's EUEN write)
			{[]int{31, 0}, 0x0400003f},      // csrwr $s8, 0x0
			{[]int{0, 0x3fff}, 0x04fffc20},  // csrwr $zero, 0x3fff
			{[]int{12, 0xc}, 0x0400302c},    // csrwr $t0, 0xc
			{[]int{31, 0x3fff}, 0x04fffc3f}, // csrwr $s8, 0x3fff
		},
	}, {
		Arch: "loong64", ISA: "csrxchg", Plan9: "CSRXCHG ${2}, R{1}, R{0}",
		Mask: 0xff000000, Bits: 0x04000000,
		NotMask: 0xff0003c0, NotBits: 0x04000000,
		Encode: func(ops ...int) uint64 {
			if ops[1] < 2 {
				panic(fmt.Sprintf("loong64: csrxchg mask register R%d would encode csrrd or csrwr", ops[1]))
			}
			return csr(0x04000000, ops[0], ops[1], ops[2])
		},
		Source: csrSource,
		Golden: []Case{
			{[]int{0, 2, 0}, 0x04000040},        // csrxchg $zero, $tp, 0x0
			{[]int{12, 13, 1}, 0x040005ac},      // csrxchg $t0, $t1, 0x1
			{[]int{31, 2, 0}, 0x0400005f},       // csrxchg $s8, $tp, 0x0
			{[]int{0, 31, 0}, 0x040003e0},       // csrxchg $zero, $s8, 0x0
			{[]int{0, 2, 0x3fff}, 0x04fffc40},   // csrxchg $zero, $tp, 0x3fff
			{[]int{4, 5, 2}, 0x040008a4},        // csrxchg $a0, $a1, 0x2
			{[]int{31, 31, 0x3fff}, 0x04ffffff}, // csrxchg $s8, $s8, 0x3fff
		},
	}}
}

// csr encodes a CSR access: the 14-bit CSR number, rj and rd.
func csr(base uint64, rd, rj, num int) uint64 {
	if num < 0 || num > 0x3fff {
		panic(fmt.Sprintf("loong64: CSR number %#x is not in [0, 0x3fff]", num))
	}
	return base | uint64(num)<<10 | reg("loong64", "general", rj, 31)<<5 | reg("loong64", "general", rd, 31)
}

// r4 encodes the four-register form: d, j, k, a from bit 0 up, five bits each.
func r4(base uint64, d, j, k, a int) uint64 {
	return base | vec(a)<<15 | vec(k)<<10 | vec(j)<<5 | vec(d)
}

// ldrepl encodes a doubleword broadcast load: a signed 9-bit offset in units
// of 8 bytes.
func ldrepl(base uint64, d, rj, off int) uint64 {
	if off%8 != 0 || off < -2048 || off > 2040 {
		panic(fmt.Sprintf("loong64: broadcast-load offset %d is not a multiple of 8 in [-2048, 2040]", off))
	}
	return base | uint64(off/8)&0x1ff<<10 | reg("loong64", "general", rj, 31)<<5 | vec(d)
}

func vec(r int) uint64 { return reg("loong64", "vector", r, 31) }
