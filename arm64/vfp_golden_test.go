package arm64

var vfpGolden = []struct {
	op   string
	regs []int
	want uint32
}{
	{"fadd", []int{0, 0, 0}, 0x4e60d400},    // fadd v0.2d, v0.2d, v0.2d
	{"fadd", []int{3, 2, 1}, 0x4e61d443},    // fadd v3.2d, v2.2d, v1.2d
	{"fadd", []int{31, 0, 0}, 0x4e60d41f},   // fadd v31.2d, v0.2d, v0.2d
	{"fadd", []int{0, 31, 0}, 0x4e60d7e0},   // fadd v0.2d, v31.2d, v0.2d
	{"fadd", []int{0, 0, 31}, 0x4e7fd400},   // fadd v0.2d, v0.2d, v31.2d
	{"fadd", []int{17, 9, 26}, 0x4e7ad531},  // fadd v17.2d, v9.2d, v26.2d
	{"fadd", []int{31, 31, 31}, 0x4e7fd7ff}, // fadd v31.2d, v31.2d, v31.2d
	{"fsub", []int{0, 0, 0}, 0x4ee0d400},    // fsub v0.2d, v0.2d, v0.2d
	{"fsub", []int{3, 2, 1}, 0x4ee1d443},    // fsub v3.2d, v2.2d, v1.2d
	{"fsub", []int{31, 0, 0}, 0x4ee0d41f},   // fsub v31.2d, v0.2d, v0.2d
	{"fsub", []int{0, 31, 0}, 0x4ee0d7e0},   // fsub v0.2d, v31.2d, v0.2d
	{"fsub", []int{0, 0, 31}, 0x4effd400},   // fsub v0.2d, v0.2d, v31.2d
	{"fsub", []int{17, 9, 26}, 0x4efad531},  // fsub v17.2d, v9.2d, v26.2d
	{"fsub", []int{31, 31, 31}, 0x4effd7ff}, // fsub v31.2d, v31.2d, v31.2d
	{"fmul", []int{0, 0, 0}, 0x6e60dc00},    // fmul v0.2d, v0.2d, v0.2d
	{"fmul", []int{3, 2, 1}, 0x6e61dc43},    // fmul v3.2d, v2.2d, v1.2d
	{"fmul", []int{31, 0, 0}, 0x6e60dc1f},   // fmul v31.2d, v0.2d, v0.2d
	{"fmul", []int{0, 31, 0}, 0x6e60dfe0},   // fmul v0.2d, v31.2d, v0.2d
	{"fmul", []int{0, 0, 31}, 0x6e7fdc00},   // fmul v0.2d, v0.2d, v31.2d
	{"fmul", []int{17, 9, 26}, 0x6e7add31},  // fmul v17.2d, v9.2d, v26.2d
	{"fmul", []int{31, 31, 31}, 0x6e7fdfff}, // fmul v31.2d, v31.2d, v31.2d
	{"fmla", []int{0, 0, 0}, 0x4e60cc00},    // fmla v0.2d, v0.2d, v0.2d
	{"fmla", []int{3, 2, 1}, 0x4e61cc43},    // fmla v3.2d, v2.2d, v1.2d
	{"fmla", []int{31, 0, 0}, 0x4e60cc1f},   // fmla v31.2d, v0.2d, v0.2d
	{"fmla", []int{0, 31, 0}, 0x4e60cfe0},   // fmla v0.2d, v31.2d, v0.2d
	{"fmla", []int{0, 0, 31}, 0x4e7fcc00},   // fmla v0.2d, v0.2d, v31.2d
	{"fmla", []int{17, 9, 26}, 0x4e7acd31},  // fmla v17.2d, v9.2d, v26.2d
	{"fmla", []int{31, 31, 31}, 0x4e7fcfff}, // fmla v31.2d, v31.2d, v31.2d
	{"fmls", []int{0, 0, 0}, 0x4ee0cc00},    // fmls v0.2d, v0.2d, v0.2d
	{"fmls", []int{3, 2, 1}, 0x4ee1cc43},    // fmls v3.2d, v2.2d, v1.2d
	{"fmls", []int{31, 0, 0}, 0x4ee0cc1f},   // fmls v31.2d, v0.2d, v0.2d
	{"fmls", []int{0, 31, 0}, 0x4ee0cfe0},   // fmls v0.2d, v31.2d, v0.2d
	{"fmls", []int{0, 0, 31}, 0x4effcc00},   // fmls v0.2d, v0.2d, v31.2d
	{"fmls", []int{17, 9, 26}, 0x4efacd31},  // fmls v17.2d, v9.2d, v26.2d
	{"fmls", []int{31, 31, 31}, 0x4effcfff}, // fmls v31.2d, v31.2d, v31.2d
	{"fneg", []int{0, 0}, 0x6ee0f800},       // fneg v0.2d, v0.2d
	{"fneg", []int{3, 2}, 0x6ee0f843},       // fneg v3.2d, v2.2d
	{"fneg", []int{31, 0}, 0x6ee0f81f},      // fneg v31.2d, v0.2d
	{"fneg", []int{0, 31}, 0x6ee0fbe0},      // fneg v0.2d, v31.2d
	{"fneg", []int{0, 0}, 0x6ee0f800},       // fneg v0.2d, v0.2d
	{"fneg", []int{17, 9}, 0x6ee0f931},      // fneg v17.2d, v9.2d
	{"fneg", []int{31, 31}, 0x6ee0fbff},     // fneg v31.2d, v31.2d
}
