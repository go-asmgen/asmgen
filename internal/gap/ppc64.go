package gap

// ppc64: VSX vector float64 arithmetic. cmd/asm has the VSX loads, stores and
// permutes but none of these (Go master, 2026-10-04). Registers are VSX
// numbers 0–63; the high bit of each sits apart from the other five (AX, BX,
// TX), which is why the reference cases straddle 31/32.
//
// Plan 9 order is the one cmd/asm's existing VSX instructions use (XXLAND,
// XXLANDC, XSMAXJDP): the ISA's operands with the target moved last, so
// XVSUBDP VS1, VS2, VS3 is VS3 = VS1 - VS2. Go's disassembler already prints
// these instructions that way.

const ppc64Source = "GNU as 2.44 (Debian, POWER9), read back with objdump; llvm-mc 22.1.5 agrees on every case"

func init() {
	a := &Arch{Name: "ppc64", GOARCH: []string{"ppc64", "ppc64le"}, Testdata: "ppc64.s", BigEndianTestdata: true}
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvadddp", Plan9: "XVADDDP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 96<<3,
		Encode: func(ops ...int) uint64 { return xx3(96, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000300},    // xvadddp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620b00},    // xvadddp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00301},   // xvadddp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0304},   // xvadddp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fb02},   // xvadddp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011307}, // xvadddp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d306}, // xvadddp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffb07}, // xvadddp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00b04},  // xvadddp vs31,vs32,vs1
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvsubdp", Plan9: "XVSUBDP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 104<<3,
		Encode: func(ops ...int) uint64 { return xx3(104, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000340},    // xvsubdp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620b40},    // xvsubdp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00341},   // xvsubdp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0344},   // xvsubdp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fb42},   // xvsubdp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011347}, // xvsubdp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d346}, // xvsubdp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffb47}, // xvsubdp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00b44},  // xvsubdp vs31,vs32,vs1
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvmuldp", Plan9: "XVMULDP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 112<<3,
		Encode: func(ops ...int) uint64 { return xx3(112, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000380},    // xvmuldp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620b80},    // xvmuldp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00381},   // xvmuldp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0384},   // xvmuldp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fb82},   // xvmuldp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011387}, // xvmuldp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d386}, // xvmuldp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffb87}, // xvmuldp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00b84},  // xvmuldp vs31,vs32,vs1
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvdivdp", Plan9: "XVDIVDP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 120<<3,
		Encode: func(ops ...int) uint64 { return xx3(120, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf00003c0},    // xvdivdp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620bc0},    // xvdivdp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e003c1},   // xvdivdp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f03c4},   // xvdivdp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fbc2},   // xvdivdp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf00113c7}, // xvdivdp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d3c6}, // xvdivdp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffbc7}, // xvdivdp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00bc4},  // xvdivdp vs31,vs32,vs1
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvmaddadp", Plan9: "XVMADDADP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 97<<3,
		Encode: func(ops ...int) uint64 { return xx3(97, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000308},    // xvmaddadp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620b08},    // xvmaddadp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00309},   // xvmaddadp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f030c},   // xvmaddadp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fb0a},   // xvmaddadp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf001130f}, // xvmaddadp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d30e}, // xvmaddadp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffb0f}, // xvmaddadp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00b0c},  // xvmaddadp vs31,vs32,vs1
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvmaxdp", Plan9: "XVMAXDP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 224<<3,
		Encode: func(ops ...int) uint64 { return xx3(224, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000700},    // xvmaxdp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620f00},    // xvmaxdp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00701},   // xvmaxdp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0704},   // xvmaxdp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000ff02},   // xvmaxdp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011707}, // xvmaxdp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d706}, // xvmaxdp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3ffff07}, // xvmaxdp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00f04},  // xvmaxdp vs31,vs32,vs1
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvmindp", Plan9: "XVMINDP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 232<<3,
		Encode: func(ops ...int) uint64 { return xx3(232, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000740},    // xvmindp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620f40},    // xvmindp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00741},   // xvmindp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0744},   // xvmindp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000ff42},   // xvmindp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011747}, // xvmindp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d746}, // xvmindp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3ffff47}, // xvmindp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00f44},  // xvmindp vs31,vs32,vs1
		},
	})
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvsqrtdp", Plan9: "XVSQRTDP VS{1}, VS{0}",
		Mask: 0xfc0007fc, Bits: 60<<26 | 203<<2,
		Encode: func(ops ...int) uint64 { return xx2(203, ops[0], ops[1]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0}, 0xf000032c},   // xvsqrtdp vs0,vs0
			{[]int{3, 2}, 0xf060132c},   // xvsqrtdp vs3,vs2
			{[]int{63, 0}, 0xf3e0032d},  // xvsqrtdp vs63,vs0
			{[]int{0, 63}, 0xf000fb2e},  // xvsqrtdp vs0,vs63
			{[]int{17, 41}, 0xf2204b2e}, // xvsqrtdp vs17,vs41
			{[]int{63, 63}, 0xf3e0fb2f}, // xvsqrtdp vs63,vs63
			{[]int{31, 32}, 0xf3e0032e}, // xvsqrtdp vs31,vs32
		},
	})
	// The single-precision family (four float32 lanes), the float32 kernels'
	// counterparts of the float64 instructions above. Same forms, same
	// register cases; the XOs were read from the two assemblers' words, not
	// assumed (each is its double-precision sibling's minus 32, as the ISA
	// lays them out).
	// xvaddsp: VSR[t] = VSR[a] + VSR[b], four float32 lanes.
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvaddsp", Plan9: "XVADDSP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 64<<3,
		Encode: func(ops ...int) uint64 { return xx3(64, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000200},    // xvaddsp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620a00},    // xvaddsp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00201},   // xvaddsp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0204},   // xvaddsp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fa02},   // xvaddsp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011207}, // xvaddsp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d206}, // xvaddsp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffa07}, // xvaddsp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00a04},  // xvaddsp vs31,vs32,vs1
		},
	})
	// xvsubsp: VSR[t] = VSR[a] - VSR[b].
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvsubsp", Plan9: "XVSUBSP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 72<<3,
		Encode: func(ops ...int) uint64 { return xx3(72, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000240},    // xvsubsp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620a40},    // xvsubsp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00241},   // xvsubsp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0244},   // xvsubsp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fa42},   // xvsubsp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011247}, // xvsubsp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d246}, // xvsubsp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffa47}, // xvsubsp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00a44},  // xvsubsp vs31,vs32,vs1
		},
	})
	// xvmulsp: VSR[t] = VSR[a] × VSR[b] (each product rounded once).
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvmulsp", Plan9: "XVMULSP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 80<<3,
		Encode: func(ops ...int) uint64 { return xx3(80, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000280},    // xvmulsp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620a80},    // xvmulsp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00281},   // xvmulsp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0284},   // xvmulsp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fa82},   // xvmulsp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011287}, // xvmulsp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d286}, // xvmulsp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffa87}, // xvmulsp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00a84},  // xvmulsp vs31,vs32,vs1
		},
	})
	// xvdivsp: VSR[t] = VSR[a] / VSR[b].
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvdivsp", Plan9: "XVDIVSP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 88<<3,
		Encode: func(ops ...int) uint64 { return xx3(88, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf00002c0},    // xvdivsp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620ac0},    // xvdivsp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e002c1},   // xvdivsp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f02c4},   // xvdivsp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fac2},   // xvdivsp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf00112c7}, // xvdivsp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d2c6}, // xvdivsp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffac7}, // xvdivsp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00ac4},  // xvdivsp vs31,vs32,vs1
		},
	})
	// xvmaddasp: VSR[t] = VSR[a] × VSR[b] + VSR[t], fused (one rounding).
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvmaddasp", Plan9: "XVMADDASP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 65<<3,
		Encode: func(ops ...int) uint64 { return xx3(65, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000208},    // xvmaddasp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620a08},    // xvmaddasp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00209},   // xvmaddasp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f020c},   // xvmaddasp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fa0a},   // xvmaddasp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf001120f}, // xvmaddasp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d20e}, // xvmaddasp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffa0f}, // xvmaddasp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00a0c},  // xvmaddasp vs31,vs32,vs1
		},
	})
	// xvmaxsp: VSR[t] = max(VSR[a], VSR[b]) per lane, with the ISA's NaN rule.
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvmaxsp", Plan9: "XVMAXSP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 192<<3,
		Encode: func(ops ...int) uint64 { return xx3(192, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000600},    // xvmaxsp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620e00},    // xvmaxsp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00601},   // xvmaxsp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0604},   // xvmaxsp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fe02},   // xvmaxsp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011607}, // xvmaxsp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d606}, // xvmaxsp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffe07}, // xvmaxsp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00e04},  // xvmaxsp vs31,vs32,vs1
		},
	})
	// xvminsp: VSR[t] = min(VSR[a], VSR[b]) per lane, with the ISA's NaN rule.
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvminsp", Plan9: "XVMINSP VS{1}, VS{2}, VS{0}",
		Mask: 0xfc0007f8, Bits: 60<<26 | 200<<3,
		Encode: func(ops ...int) uint64 { return xx3(200, ops[0], ops[1], ops[2]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0, 0}, 0xf0000640},    // xvminsp vs0,vs0,vs0
			{[]int{3, 2, 1}, 0xf0620e40},    // xvminsp vs3,vs2,vs1
			{[]int{63, 0, 0}, 0xf3e00641},   // xvminsp vs63,vs0,vs0
			{[]int{0, 63, 0}, 0xf01f0644},   // xvminsp vs0,vs63,vs0
			{[]int{0, 0, 63}, 0xf000fe42},   // xvminsp vs0,vs0,vs63
			{[]int{32, 33, 34}, 0xf0011647}, // xvminsp vs32,vs33,vs34
			{[]int{17, 41, 58}, 0xf229d646}, // xvminsp vs17,vs41,vs58
			{[]int{63, 63, 63}, 0xf3fffe47}, // xvminsp vs63,vs63,vs63
			{[]int{31, 32, 1}, 0xf3e00e44},  // xvminsp vs31,vs32,vs1
		},
	})
	// xvsqrtsp: VSR[t] = sqrt(VSR[b]), correctly rounded.
	a.insns = append(a.insns, &Insn{
		Arch: "ppc64", ISA: "xvsqrtsp", Plan9: "XVSQRTSP VS{1}, VS{0}",
		Mask: 0xfc0007fc, Bits: 60<<26 | 139<<2,
		Encode: func(ops ...int) uint64 { return xx2(139, ops[0], ops[1]) },
		Source: ppc64Source,
		Golden: []Case{
			{[]int{0, 0}, 0xf000022c},   // xvsqrtsp vs0,vs0
			{[]int{3, 2}, 0xf060122c},   // xvsqrtsp vs3,vs2
			{[]int{63, 0}, 0xf3e0022d},  // xvsqrtsp vs63,vs0
			{[]int{0, 63}, 0xf000fa2e},  // xvsqrtsp vs0,vs63
			{[]int{17, 41}, 0xf2204a2e}, // xvsqrtsp vs17,vs41
			{[]int{63, 63}, 0xf3e0fa2f}, // xvsqrtsp vs63,vs63
			{[]int{31, 32}, 0xf3e0022e}, // xvsqrtsp vs31,vs32
		},
	})
	arches = append(arches, a)
}

// xx3 encodes a Power ISA XX3-form instruction: opcode 60, T/A/B low five
// bits in 6–20, an 8-bit XO, then the high bits AX, BX, TX.
func xx3(xo uint64, t, a, b int) uint64 {
	T, A, B := vsr(t), vsr(a), vsr(b)
	return 60<<26 | T&31<<21 | A&31<<16 | B&31<<11 | xo<<3 | A>>5<<2 | B>>5<<1 | T>>5
}

// xx2 encodes an XX2-form instruction: no A, a 9-bit XO.
func xx2(xo uint64, t, b int) uint64 {
	T, B := vsr(t), vsr(b)
	return 60<<26 | T&31<<21 | B&31<<11 | xo<<2 | B>>5<<1 | T>>5
}

func vsr(r int) uint64 { return reg("ppc64", "VSX", r, 63) }
