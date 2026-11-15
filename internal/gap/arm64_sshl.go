package gap

import "fmt"

// arm64: SSHL, signed shift left by register (Arm ARM, "Advanced SIMD three
// same"):
//
//	0 Q 0 01110 size 1 Rm 01000 1 Rn Rd
//
// Each lane of Vn is shifted by the signed low byte of the same lane of Vm:
// a positive count shifts left, a negative one shifts right arithmetically
// (rounding toward minus infinity, like ">>" on a signed integer). It is the
// variable right shift a codec needs when the shift is a bitstream parameter
// (H.264 weighted prediction, logWD, clause 8.4.2.3.2), where the immediate
// shifts (VSSHR, VSRSHR) do not apply. cmd/asm (Go 1.27.4) has the immediate
// forms and the unsigned VUSHL/VSSHLL, not this one.
//
// The proposed Plan 9 spelling follows the other three-operand vector forms,
// Vm first: VSSHL Vm.T, Vn.T, Vd.T.

const sshlSource = "Apple clang 21.0.0 (clang-2100.4.1.1, LLVM integrated assembler, -target arm64-apple-macos), words read back with otool -t"

type sshlForm struct {
	name string // ISA name with the arrangement
	t    string // Plan 9 arrangement
	q    uint64
	size uint64
}

var sshlForms = []sshlForm{
	{"sshl.8b", "B8", 0, 0},
	{"sshl.16b", "B16", 1, 0},
	{"sshl.4h", "H4", 0, 1},
	{"sshl.8h", "H8", 1, 1},
	{"sshl.2s", "S2", 0, 2},
	{"sshl.4s", "S4", 1, 2},
	{"sshl.2d", "D2", 1, 3},
}

func init() {
	a := ArchOf("arm64")
	for _, f := range sshlForms {
		a.insns = append(a.insns, sshlInsn(f))
	}
}

// sshlInsn is one arrangement; its operands, in A64 order, are Vd, Vn, Vm.
func sshlInsn(f sshlForm) *Insn {
	base := 0x0e204400 | f.q<<30 | f.size<<22
	return &Insn{
		Arch: "arm64", ISA: f.name,
		Plan9: "VSSHL V{2}." + f.t + ", V{1}." + f.t + ", V{0}." + f.t,
		Mask:  0xffe0fc00, Bits: base,
		Encode: func(ops ...int) uint64 {
			if len(ops) != 3 {
				panic(fmt.Sprintf("arm64: %s takes 3 operands, got %d", f.name, len(ops)))
			}
			return base | reg("arm64", "vector", ops[2], 31)<<16 | reg("arm64", "vector", ops[1], 31)<<5 | reg("arm64", "vector", ops[0], 31)
		},
		Source: sshlSource,
		Golden: sshlGolden[f.name],
	}
}

var sshlGolden = map[string][]Case{
	"sshl.8b": {
		{[]int{0, 0, 0}, 0x0e204400},  // sshl v0.8b, v0.8b, v0.8b
		{[]int{31, 1, 2}, 0x0e22443f}, // sshl v31.8b, v1.8b, v2.8b
		{[]int{0, 31, 1}, 0x0e2147e0}, // sshl v0.8b, v31.8b, v1.8b
		{[]int{0, 1, 31}, 0x0e3f4420}, // sshl v0.8b, v1.8b, v31.8b
	},
	"sshl.16b": {
		{[]int{0, 0, 0}, 0x4e204400},    // sshl v0.16b, v0.16b, v0.16b
		{[]int{31, 30, 29}, 0x4e3d47df}, // sshl v31.16b, v30.16b, v29.16b
	},
	"sshl.4h": {
		{[]int{0, 0, 0}, 0x0e604400},  // sshl v0.4h, v0.4h, v0.4h
		{[]int{31, 1, 2}, 0x0e62443f}, // sshl v31.4h, v1.4h, v2.4h
		{[]int{0, 31, 1}, 0x0e6147e0}, // sshl v0.4h, v31.4h, v1.4h
		{[]int{0, 1, 31}, 0x0e7f4420}, // sshl v0.4h, v1.4h, v31.4h
	},
	"sshl.8h": {
		{[]int{0, 0, 0}, 0x4e604400},    // sshl v0.8h, v0.8h, v0.8h
		{[]int{0, 0, 22}, 0x4e764400},   // sshl v0.8h, v0.8h, v22.8h
		{[]int{31, 30, 29}, 0x4e7d47df}, // sshl v31.8h, v30.8h, v29.8h
	},
	"sshl.2s": {
		{[]int{0, 0, 0}, 0x0ea04400},  // sshl v0.2s, v0.2s, v0.2s
		{[]int{31, 1, 2}, 0x0ea2443f}, // sshl v31.2s, v1.2s, v2.2s
		{[]int{0, 1, 31}, 0x0ebf4420}, // sshl v0.2s, v1.2s, v31.2s
	},
	"sshl.4s": {
		{[]int{0, 0, 0}, 0x4ea04400},    // sshl v0.4s, v0.4s, v0.4s
		{[]int{2, 2, 23}, 0x4eb74442},   // sshl v2.4s, v2.4s, v23.4s
		{[]int{3, 3, 23}, 0x4eb74463},   // sshl v3.4s, v3.4s, v23.4s
		{[]int{31, 30, 29}, 0x4ebd47df}, // sshl v31.4s, v30.4s, v29.4s
	},
	"sshl.2d": {
		{[]int{0, 0, 0}, 0x4ee04400},    // sshl v0.2d, v0.2d, v0.2d
		{[]int{31, 30, 29}, 0x4efd47df}, // sshl v31.2d, v30.2d, v29.2d
		{[]int{5, 6, 7}, 0x4ee744c5},    // sshl v5.2d, v6.2d, v7.2d
	},
}
