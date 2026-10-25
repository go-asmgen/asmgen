package gap

import "fmt"

// arm64: the Advanced SIMD floating-point multiply (and multiply-add) by
// element, vector forms (Arm ARM, FMLA/FMLS/FMUL (by element), "Advanced SIMD
// vector x indexed element"):
//
//	0 Q U 0 1 1 1 1 1 sz L M Rm[3:0] opcode H 0 Rn Rd
//
// with U = 0, opcode 0001 fmla, 0101 fmls, 1001 fmul. For .S (sz = 0) the
// element index is H:L (0-3); for .D (sz = 1) it is H (0-1) and L must be 0,
// sz:L = 11 being unallocated. M is bit 4 of Vm, so Vm is any of V0-V31: only
// the half-precision forms, which go-asmgen does not use, are limited to
// V0-V15 (LLVM's AArch64InstrFormats.td, SIMDFPIndexed: V128 for .s and .d,
// V128_lo for .h).
//
// cmd/asm on master (2026-10-25) has the vector-by-vector VFMLA/VFMLS/VFMUL
// but no indexed form: arm64enc.s carries "//TODO VFMLA V28.S[2], V2.S2,
// V30.S2". go-ndarray's GEMM micro-kernel therefore writes its sixteen
// "FMLA Vd.2D, Vn.2D, Vm.D[i]" as WORDs. go.dev/cl/764224 (Jonathan Swinney)
// adds the indexed forms, spelt as below: the element operand first, then Vn,
// then Vd, as every cmd/asm arm64 vector instruction reverses the A64 order.
// That CL is not ours, so it is not in goasm-patches/cls.txt; goasmgap verify
// against a tree carrying it is how it is checked.

const arm64Source = "llvm-mc 23.1.2 (-triple=aarch64 -show-encoding); GNU as 2.44 (Debian trixie, aarch64), read back with objdump, agrees on every case"

func init() {
	a := &Arch{Name: "arm64", GOARCH: []string{"arm64"}, Testdata: "arm64enc.s", InsnPad: 43}
	for _, op := range []struct {
		isa    string
		plan9  string
		opcode uint64
	}{
		{"fmla", "VFMLA", 0x1},
		{"fmls", "VFMLS", 0x5},
		{"fmul", "VFMUL", 0x9},
	} {
		a.insns = append(a.insns, elemInsn(op.isa, op.plan9, op.opcode, false), elemInsn(op.isa, op.plan9, op.opcode, true))
	}
	arches = append(arches, a)
}

// elemInsn is the .4S (d false) or .2D (d true) by-element form of one
// instruction. Its operands, in A64 order, are Vd, Vn, Vm and the index.
func elemInsn(isa, plan9 string, opcode uint64, d bool) *Insn {
	base := 0x4f800000 | opcode<<12 // Q = 1, U = 0, sz = 0
	mask := uint64(0xffc0f400)      // Q U 01111 sz, opcode, bit 10
	name, syntax := isa+".elem.4s", plan9+" V{2}.S[{3}], V{1}.S4, V{0}.S4"
	if d {
		base |= 1 << 22
		mask |= 1 << 21 // L is 0 for .D
		name, syntax = isa+".elem.2d", plan9+" V{2}.D[{3}], V{1}.D2, V{0}.D2"
	}
	return &Insn{
		Arch: "arm64", ISA: name, Plan9: syntax,
		Mask: mask, Bits: base,
		Encode: func(ops ...int) uint64 { return byElement(base, d, ops[0], ops[1], ops[2], ops[3]) },
		Source: arm64Source,
		Golden: arm64Golden[name],
	}
}

// byElement encodes the operands into base: Vm's bit 4 is M (bit 20), its low
// four bits Rm (19-16); the index is H:L for .S, H for .D.
func byElement(base uint64, d bool, vd, vn, vm, index int) uint64 {
	max := 3
	if d {
		max = 1
	}
	if index < 0 || index > max {
		panic(fmt.Sprintf("arm64: element index %d is not in [0, %d]", index, max))
	}
	m := reg("arm64", "vector", vm, 31)
	w := base | m<<16 | reg("arm64", "vector", vn, 31)<<5 | reg("arm64", "vector", vd, 31)
	if d {
		return w | uint64(index)<<11
	}
	return w | uint64(index>>1)<<11 | uint64(index&1)<<21
}

// arm64Golden holds the reference encodings: every register field at 0 and
// 31, Vm at 15 and 16 (where M flips), every index, and for fmla .2D the
// sixteen words go-ndarray's GEMM kernel emits (internal/kernels/sum_arm64.s).
var arm64Golden = map[string][]Case{
	"fmla.elem.4s": {
		{[]int{0, 0, 0, 0}, 0x4f801000},    // fmla v0.4s, v0.4s, v0.s[0]
		{[]int{31, 0, 0, 0}, 0x4f80101f},   // fmla v31.4s, v0.4s, v0.s[0]
		{[]int{0, 31, 0, 0}, 0x4f8013e0},   // fmla v0.4s, v31.4s, v0.s[0]
		{[]int{0, 0, 31, 0}, 0x4f9f1000},   // fmla v0.4s, v0.4s, v31.s[0]
		{[]int{0, 0, 15, 0}, 0x4f8f1000},   // fmla v0.4s, v0.4s, v15.s[0]
		{[]int{0, 0, 16, 0}, 0x4f901000},   // fmla v0.4s, v0.4s, v16.s[0]
		{[]int{0, 0, 0, 1}, 0x4fa01000},    // fmla v0.4s, v0.4s, v0.s[1]
		{[]int{0, 0, 0, 2}, 0x4f801800},    // fmla v0.4s, v0.4s, v0.s[2]
		{[]int{0, 0, 0, 3}, 0x4fa01800},    // fmla v0.4s, v0.4s, v0.s[3]
		{[]int{17, 9, 26, 2}, 0x4f9a1931},  // fmla v17.4s, v9.4s, v26.s[2]
		{[]int{31, 31, 31, 3}, 0x4fbf1bff}, // fmla v31.4s, v31.4s, v31.s[3]
	},
	"fmla.elem.2d": {
		{[]int{0, 0, 0, 0}, 0x4fc01000},    // fmla v0.2d, v0.2d, v0.d[0]
		{[]int{31, 0, 0, 0}, 0x4fc0101f},   // fmla v31.2d, v0.2d, v0.d[0]
		{[]int{0, 31, 0, 0}, 0x4fc013e0},   // fmla v0.2d, v31.2d, v0.d[0]
		{[]int{0, 0, 31, 0}, 0x4fdf1000},   // fmla v0.2d, v0.2d, v31.d[0]
		{[]int{0, 0, 15, 0}, 0x4fcf1000},   // fmla v0.2d, v0.2d, v15.d[0]
		{[]int{0, 0, 16, 0}, 0x4fd01000},   // fmla v0.2d, v0.2d, v16.d[0]
		{[]int{0, 0, 0, 1}, 0x4fc01800},    // fmla v0.2d, v0.2d, v0.d[1]
		{[]int{17, 9, 26, 0}, 0x4fda1131},  // fmla v17.2d, v9.2d, v26.d[0]
		{[]int{31, 31, 31, 1}, 0x4fdf1bff}, // fmla v31.2d, v31.2d, v31.d[1]
		{[]int{0, 16, 20, 0}, 0x4fd41200},  // fmla v0.2d, v16.2d, v20.d[0]
		{[]int{1, 17, 20, 0}, 0x4fd41221},  // fmla v1.2d, v17.2d, v20.d[0]
		{[]int{2, 18, 20, 0}, 0x4fd41242},  // fmla v2.2d, v18.2d, v20.d[0]
		{[]int{3, 19, 20, 0}, 0x4fd41263},  // fmla v3.2d, v19.2d, v20.d[0]
		{[]int{4, 16, 20, 1}, 0x4fd41a04},  // fmla v4.2d, v16.2d, v20.d[1]
		{[]int{5, 17, 20, 1}, 0x4fd41a25},  // fmla v5.2d, v17.2d, v20.d[1]
		{[]int{6, 18, 20, 1}, 0x4fd41a46},  // fmla v6.2d, v18.2d, v20.d[1]
		{[]int{7, 19, 20, 1}, 0x4fd41a67},  // fmla v7.2d, v19.2d, v20.d[1]
		{[]int{8, 16, 21, 0}, 0x4fd51208},  // fmla v8.2d, v16.2d, v21.d[0]
		{[]int{9, 17, 21, 0}, 0x4fd51229},  // fmla v9.2d, v17.2d, v21.d[0]
		{[]int{10, 18, 21, 0}, 0x4fd5124a}, // fmla v10.2d, v18.2d, v21.d[0]
		{[]int{11, 19, 21, 0}, 0x4fd5126b}, // fmla v11.2d, v19.2d, v21.d[0]
		{[]int{12, 16, 21, 1}, 0x4fd51a0c}, // fmla v12.2d, v16.2d, v21.d[1]
		{[]int{13, 17, 21, 1}, 0x4fd51a2d}, // fmla v13.2d, v17.2d, v21.d[1]
		{[]int{14, 18, 21, 1}, 0x4fd51a4e}, // fmla v14.2d, v18.2d, v21.d[1]
		{[]int{15, 19, 21, 1}, 0x4fd51a6f}, // fmla v15.2d, v19.2d, v21.d[1]
	},
	"fmls.elem.4s": {
		{[]int{0, 0, 0, 0}, 0x4f805000},    // fmls v0.4s, v0.4s, v0.s[0]
		{[]int{31, 0, 0, 0}, 0x4f80501f},   // fmls v31.4s, v0.4s, v0.s[0]
		{[]int{0, 31, 0, 0}, 0x4f8053e0},   // fmls v0.4s, v31.4s, v0.s[0]
		{[]int{0, 0, 31, 0}, 0x4f9f5000},   // fmls v0.4s, v0.4s, v31.s[0]
		{[]int{0, 0, 15, 0}, 0x4f8f5000},   // fmls v0.4s, v0.4s, v15.s[0]
		{[]int{0, 0, 16, 0}, 0x4f905000},   // fmls v0.4s, v0.4s, v16.s[0]
		{[]int{0, 0, 0, 1}, 0x4fa05000},    // fmls v0.4s, v0.4s, v0.s[1]
		{[]int{0, 0, 0, 2}, 0x4f805800},    // fmls v0.4s, v0.4s, v0.s[2]
		{[]int{0, 0, 0, 3}, 0x4fa05800},    // fmls v0.4s, v0.4s, v0.s[3]
		{[]int{17, 9, 26, 2}, 0x4f9a5931},  // fmls v17.4s, v9.4s, v26.s[2]
		{[]int{31, 31, 31, 3}, 0x4fbf5bff}, // fmls v31.4s, v31.4s, v31.s[3]
	},
	"fmls.elem.2d": {
		{[]int{0, 0, 0, 0}, 0x4fc05000},    // fmls v0.2d, v0.2d, v0.d[0]
		{[]int{31, 0, 0, 0}, 0x4fc0501f},   // fmls v31.2d, v0.2d, v0.d[0]
		{[]int{0, 31, 0, 0}, 0x4fc053e0},   // fmls v0.2d, v31.2d, v0.d[0]
		{[]int{0, 0, 31, 0}, 0x4fdf5000},   // fmls v0.2d, v0.2d, v31.d[0]
		{[]int{0, 0, 15, 0}, 0x4fcf5000},   // fmls v0.2d, v0.2d, v15.d[0]
		{[]int{0, 0, 16, 0}, 0x4fd05000},   // fmls v0.2d, v0.2d, v16.d[0]
		{[]int{0, 0, 0, 1}, 0x4fc05800},    // fmls v0.2d, v0.2d, v0.d[1]
		{[]int{17, 9, 26, 0}, 0x4fda5131},  // fmls v17.2d, v9.2d, v26.d[0]
		{[]int{31, 31, 31, 1}, 0x4fdf5bff}, // fmls v31.2d, v31.2d, v31.d[1]
	},
	"fmul.elem.4s": {
		{[]int{0, 0, 0, 0}, 0x4f809000},    // fmul v0.4s, v0.4s, v0.s[0]
		{[]int{31, 0, 0, 0}, 0x4f80901f},   // fmul v31.4s, v0.4s, v0.s[0]
		{[]int{0, 31, 0, 0}, 0x4f8093e0},   // fmul v0.4s, v31.4s, v0.s[0]
		{[]int{0, 0, 31, 0}, 0x4f9f9000},   // fmul v0.4s, v0.4s, v31.s[0]
		{[]int{0, 0, 15, 0}, 0x4f8f9000},   // fmul v0.4s, v0.4s, v15.s[0]
		{[]int{0, 0, 16, 0}, 0x4f909000},   // fmul v0.4s, v0.4s, v16.s[0]
		{[]int{0, 0, 0, 1}, 0x4fa09000},    // fmul v0.4s, v0.4s, v0.s[1]
		{[]int{0, 0, 0, 2}, 0x4f809800},    // fmul v0.4s, v0.4s, v0.s[2]
		{[]int{0, 0, 0, 3}, 0x4fa09800},    // fmul v0.4s, v0.4s, v0.s[3]
		{[]int{17, 9, 26, 2}, 0x4f9a9931},  // fmul v17.4s, v9.4s, v26.s[2]
		{[]int{31, 31, 31, 3}, 0x4fbf9bff}, // fmul v31.4s, v31.4s, v31.s[3]
	},
	"fmul.elem.2d": {
		{[]int{0, 0, 0, 0}, 0x4fc09000},    // fmul v0.2d, v0.2d, v0.d[0]
		{[]int{31, 0, 0, 0}, 0x4fc0901f},   // fmul v31.2d, v0.2d, v0.d[0]
		{[]int{0, 31, 0, 0}, 0x4fc093e0},   // fmul v0.2d, v31.2d, v0.d[0]
		{[]int{0, 0, 31, 0}, 0x4fdf9000},   // fmul v0.2d, v0.2d, v31.d[0]
		{[]int{0, 0, 15, 0}, 0x4fcf9000},   // fmul v0.2d, v0.2d, v15.d[0]
		{[]int{0, 0, 16, 0}, 0x4fd09000},   // fmul v0.2d, v0.2d, v16.d[0]
		{[]int{0, 0, 0, 1}, 0x4fc09800},    // fmul v0.2d, v0.2d, v0.d[1]
		{[]int{17, 9, 26, 0}, 0x4fda9131},  // fmul v17.2d, v9.2d, v26.d[0]
		{[]int{31, 31, 31, 1}, 0x4fdf9bff}, // fmul v31.2d, v31.2d, v31.d[1]
	},
}
