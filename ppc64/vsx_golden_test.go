package ppc64

// vsxGolden is GNU as 2.44 (Debian, on a POWER9) assembling each instruction,
// read back with objdump: the reference the encoders are held to.
var vsxGolden = []struct {
	op   string
	regs []int
	want uint32
}{
	{"xvadddp", []int{0, 0, 0}, 0xf0000300},      // xvadddp vs0,vs0,vs0
	{"xvadddp", []int{3, 2, 1}, 0xf0620b00},      // xvadddp vs3,vs2,vs1
	{"xvadddp", []int{63, 0, 0}, 0xf3e00301},     // xvadddp vs63,vs0,vs0
	{"xvadddp", []int{0, 63, 0}, 0xf01f0304},     // xvadddp vs0,vs63,vs0
	{"xvadddp", []int{0, 0, 63}, 0xf000fb02},     // xvadddp vs0,vs0,vs63
	{"xvadddp", []int{32, 33, 34}, 0xf0011307},   // xvadddp vs32,vs33,vs34
	{"xvadddp", []int{17, 41, 58}, 0xf229d306},   // xvadddp vs17,vs41,vs58
	{"xvadddp", []int{63, 63, 63}, 0xf3fffb07},   // xvadddp vs63,vs63,vs63
	{"xvadddp", []int{31, 32, 1}, 0xf3e00b04},    // xvadddp vs31,vs32,vs1
	{"xvsubdp", []int{0, 0, 0}, 0xf0000340},      // xvsubdp vs0,vs0,vs0
	{"xvsubdp", []int{3, 2, 1}, 0xf0620b40},      // xvsubdp vs3,vs2,vs1
	{"xvsubdp", []int{63, 0, 0}, 0xf3e00341},     // xvsubdp vs63,vs0,vs0
	{"xvsubdp", []int{0, 63, 0}, 0xf01f0344},     // xvsubdp vs0,vs63,vs0
	{"xvsubdp", []int{0, 0, 63}, 0xf000fb42},     // xvsubdp vs0,vs0,vs63
	{"xvsubdp", []int{32, 33, 34}, 0xf0011347},   // xvsubdp vs32,vs33,vs34
	{"xvsubdp", []int{17, 41, 58}, 0xf229d346},   // xvsubdp vs17,vs41,vs58
	{"xvsubdp", []int{63, 63, 63}, 0xf3fffb47},   // xvsubdp vs63,vs63,vs63
	{"xvsubdp", []int{31, 32, 1}, 0xf3e00b44},    // xvsubdp vs31,vs32,vs1
	{"xvmuldp", []int{0, 0, 0}, 0xf0000380},      // xvmuldp vs0,vs0,vs0
	{"xvmuldp", []int{3, 2, 1}, 0xf0620b80},      // xvmuldp vs3,vs2,vs1
	{"xvmuldp", []int{63, 0, 0}, 0xf3e00381},     // xvmuldp vs63,vs0,vs0
	{"xvmuldp", []int{0, 63, 0}, 0xf01f0384},     // xvmuldp vs0,vs63,vs0
	{"xvmuldp", []int{0, 0, 63}, 0xf000fb82},     // xvmuldp vs0,vs0,vs63
	{"xvmuldp", []int{32, 33, 34}, 0xf0011387},   // xvmuldp vs32,vs33,vs34
	{"xvmuldp", []int{17, 41, 58}, 0xf229d386},   // xvmuldp vs17,vs41,vs58
	{"xvmuldp", []int{63, 63, 63}, 0xf3fffb87},   // xvmuldp vs63,vs63,vs63
	{"xvmuldp", []int{31, 32, 1}, 0xf3e00b84},    // xvmuldp vs31,vs32,vs1
	{"xvdivdp", []int{0, 0, 0}, 0xf00003c0},      // xvdivdp vs0,vs0,vs0
	{"xvdivdp", []int{3, 2, 1}, 0xf0620bc0},      // xvdivdp vs3,vs2,vs1
	{"xvdivdp", []int{63, 0, 0}, 0xf3e003c1},     // xvdivdp vs63,vs0,vs0
	{"xvdivdp", []int{0, 63, 0}, 0xf01f03c4},     // xvdivdp vs0,vs63,vs0
	{"xvdivdp", []int{0, 0, 63}, 0xf000fbc2},     // xvdivdp vs0,vs0,vs63
	{"xvdivdp", []int{32, 33, 34}, 0xf00113c7},   // xvdivdp vs32,vs33,vs34
	{"xvdivdp", []int{17, 41, 58}, 0xf229d3c6},   // xvdivdp vs17,vs41,vs58
	{"xvdivdp", []int{63, 63, 63}, 0xf3fffbc7},   // xvdivdp vs63,vs63,vs63
	{"xvdivdp", []int{31, 32, 1}, 0xf3e00bc4},    // xvdivdp vs31,vs32,vs1
	{"xvmaddadp", []int{0, 0, 0}, 0xf0000308},    // xvmaddadp vs0,vs0,vs0
	{"xvmaddadp", []int{3, 2, 1}, 0xf0620b08},    // xvmaddadp vs3,vs2,vs1
	{"xvmaddadp", []int{63, 0, 0}, 0xf3e00309},   // xvmaddadp vs63,vs0,vs0
	{"xvmaddadp", []int{0, 63, 0}, 0xf01f030c},   // xvmaddadp vs0,vs63,vs0
	{"xvmaddadp", []int{0, 0, 63}, 0xf000fb0a},   // xvmaddadp vs0,vs0,vs63
	{"xvmaddadp", []int{32, 33, 34}, 0xf001130f}, // xvmaddadp vs32,vs33,vs34
	{"xvmaddadp", []int{17, 41, 58}, 0xf229d30e}, // xvmaddadp vs17,vs41,vs58
	{"xvmaddadp", []int{63, 63, 63}, 0xf3fffb0f}, // xvmaddadp vs63,vs63,vs63
	{"xvmaddadp", []int{31, 32, 1}, 0xf3e00b0c},  // xvmaddadp vs31,vs32,vs1
	{"xvmaxdp", []int{0, 0, 0}, 0xf0000700},      // xvmaxdp vs0,vs0,vs0
	{"xvmaxdp", []int{3, 2, 1}, 0xf0620f00},      // xvmaxdp vs3,vs2,vs1
	{"xvmaxdp", []int{63, 0, 0}, 0xf3e00701},     // xvmaxdp vs63,vs0,vs0
	{"xvmaxdp", []int{0, 63, 0}, 0xf01f0704},     // xvmaxdp vs0,vs63,vs0
	{"xvmaxdp", []int{0, 0, 63}, 0xf000ff02},     // xvmaxdp vs0,vs0,vs63
	{"xvmaxdp", []int{32, 33, 34}, 0xf0011707},   // xvmaxdp vs32,vs33,vs34
	{"xvmaxdp", []int{17, 41, 58}, 0xf229d706},   // xvmaxdp vs17,vs41,vs58
	{"xvmaxdp", []int{63, 63, 63}, 0xf3ffff07},   // xvmaxdp vs63,vs63,vs63
	{"xvmaxdp", []int{31, 32, 1}, 0xf3e00f04},    // xvmaxdp vs31,vs32,vs1
	{"xvmindp", []int{0, 0, 0}, 0xf0000740},      // xvmindp vs0,vs0,vs0
	{"xvmindp", []int{3, 2, 1}, 0xf0620f40},      // xvmindp vs3,vs2,vs1
	{"xvmindp", []int{63, 0, 0}, 0xf3e00741},     // xvmindp vs63,vs0,vs0
	{"xvmindp", []int{0, 63, 0}, 0xf01f0744},     // xvmindp vs0,vs63,vs0
	{"xvmindp", []int{0, 0, 63}, 0xf000ff42},     // xvmindp vs0,vs0,vs63
	{"xvmindp", []int{32, 33, 34}, 0xf0011747},   // xvmindp vs32,vs33,vs34
	{"xvmindp", []int{17, 41, 58}, 0xf229d746},   // xvmindp vs17,vs41,vs58
	{"xvmindp", []int{63, 63, 63}, 0xf3ffff47},   // xvmindp vs63,vs63,vs63
	{"xvmindp", []int{31, 32, 1}, 0xf3e00f44},    // xvmindp vs31,vs32,vs1
	{"xvsqrtdp", []int{0, 0}, 0xf000032c},        // xvsqrtdp vs0,vs0
	{"xvsqrtdp", []int{3, 2}, 0xf060132c},        // xvsqrtdp vs3,vs2
	{"xvsqrtdp", []int{63, 0}, 0xf3e0032d},       // xvsqrtdp vs63,vs0
	{"xvsqrtdp", []int{0, 63}, 0xf000fb2e},       // xvsqrtdp vs0,vs63
	{"xvsqrtdp", []int{17, 41}, 0xf2204b2e},      // xvsqrtdp vs17,vs41
	{"xvsqrtdp", []int{63, 63}, 0xf3e0fb2f},      // xvsqrtdp vs63,vs63
	{"xvsqrtdp", []int{31, 32}, 0xf3e0032e},      // xvsqrtdp vs31,vs32
}
