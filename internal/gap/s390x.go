package gap

// s390x: vector float32 arithmetic, the vector-enhancements facility 1 (z14).
// cmd/asm has the float64 forms (VFADB, WFADB, ...) but none of the float32
// ones (Go master, 2026-10-18); go.dev/cl/846286 adds them, and its spellings
// and operand order are the ones proposed here. The V forms work on the four
// lanes of a vector, the W forms on element 0 only.
//
// The instructions are 6 bytes long, so an encoding here is 48 bits: the
// bytes in memory order, read as a big-endian number. Registers are V0–V31;
// the low four bits of each sit in its field and the high bit in RXB, which is
// why the reference cases put every operand at 0, 15, 16 and 31.
//
// Plan 9 order follows cmd/asm's float64 forms: the target last, and, as for
// SUB, the subtrahend and the divisor first. VFSSB V1, V2, V3 is
// V3 = V2 - V1 (vfssb %v3,%v2,%v1); VFASB V1, V2, V3 is vfasb %v3,%v1,%v2.

const s390xSource = "GNU as 2.42 (Ubuntu, IBM z with vxe/vxe2, -march=z14), read back with objdump; llvm-mc 23.1.2 (-mcpu=z14) agrees on every case"

func init() {
	a := &Arch{Name: "s390x", GOARCH: []string{"s390x"}, Testdata: "s390x.s", Size: 6, BigEndianTestdata: true, OperandPad: 24}
	// VRR-c: V1 = V2 op V3. m4 = 2 is the short (float32) format, m5 = 8
	// the single-element (W) control.
	vrrcEntry := func(isa, plan9 string, op, m5 uint64, golden []Case) {
		a.insns = append(a.insns, &Insn{
			Arch: "s390x", ISA: isa, Plan9: plan9,
			Mask: 0xff000ffff0ff, Bits: 0xe7<<40 | m5<<16 | 2<<12 | op,
			Encode: func(ops ...int) uint64 { return vrrc(op, 2, m5, ops[0], ops[1], ops[2]) },
			Source: s390xSource, Golden: golden,
		})
	}
	vrrcEntry("vfasb", "VFASB V{1}, V{2}, V{0}", 0xe3, 0, []Case{
		{[]int{0, 0, 0}, 0xe700000020e3},    // vfasb %v0,%v0,%v0
		{[]int{3, 1, 2}, 0xe731200020e3},    // vfasb %v3,%v1,%v2
		{[]int{16, 0, 0}, 0xe700000028e3},   // vfasb %v16,%v0,%v0
		{[]int{0, 16, 0}, 0xe700000024e3},   // vfasb %v0,%v16,%v0
		{[]int{0, 0, 16}, 0xe700000022e3},   // vfasb %v0,%v0,%v16
		{[]int{15, 16, 31}, 0xe7f0f00026e3}, // vfasb %v15,%v16,%v31
		{[]int{31, 15, 16}, 0xe7ff00002ae3}, // vfasb %v31,%v15,%v16
		{[]int{16, 31, 15}, 0xe70ff0002ce3}, // vfasb %v16,%v31,%v15
		{[]int{31, 31, 31}, 0xe7fff0002ee3}, // vfasb %v31,%v31,%v31
	})
	vrrcEntry("wfasb", "WFASB V{1}, V{2}, V{0}", 0xe3, 8, []Case{
		{[]int{0, 0, 0}, 0xe700000820e3},    // wfasb %v0,%v0,%v0
		{[]int{3, 1, 2}, 0xe731200820e3},    // wfasb %v3,%v1,%v2
		{[]int{16, 0, 0}, 0xe700000828e3},   // wfasb %v16,%v0,%v0
		{[]int{0, 16, 0}, 0xe700000824e3},   // wfasb %v0,%v16,%v0
		{[]int{0, 0, 16}, 0xe700000822e3},   // wfasb %v0,%v0,%v16
		{[]int{15, 16, 31}, 0xe7f0f00826e3}, // wfasb %v15,%v16,%v31
		{[]int{31, 15, 16}, 0xe7ff00082ae3}, // wfasb %v31,%v15,%v16
		{[]int{16, 31, 15}, 0xe70ff0082ce3}, // wfasb %v16,%v31,%v15
		{[]int{31, 31, 31}, 0xe7fff0082ee3}, // wfasb %v31,%v31,%v31
	})
	vrrcEntry("vfssb", "VFSSB V{2}, V{1}, V{0}", 0xe2, 0, []Case{
		{[]int{0, 0, 0}, 0xe700000020e2},    // vfssb %v0,%v0,%v0
		{[]int{3, 1, 2}, 0xe731200020e2},    // vfssb %v3,%v1,%v2
		{[]int{16, 0, 0}, 0xe700000028e2},   // vfssb %v16,%v0,%v0
		{[]int{0, 16, 0}, 0xe700000024e2},   // vfssb %v0,%v16,%v0
		{[]int{0, 0, 16}, 0xe700000022e2},   // vfssb %v0,%v0,%v16
		{[]int{15, 16, 31}, 0xe7f0f00026e2}, // vfssb %v15,%v16,%v31
		{[]int{31, 15, 16}, 0xe7ff00002ae2}, // vfssb %v31,%v15,%v16
		{[]int{16, 31, 15}, 0xe70ff0002ce2}, // vfssb %v16,%v31,%v15
		{[]int{31, 31, 31}, 0xe7fff0002ee2}, // vfssb %v31,%v31,%v31
	})
	vrrcEntry("wfssb", "WFSSB V{2}, V{1}, V{0}", 0xe2, 8, []Case{
		{[]int{0, 0, 0}, 0xe700000820e2},    // wfssb %v0,%v0,%v0
		{[]int{3, 1, 2}, 0xe731200820e2},    // wfssb %v3,%v1,%v2
		{[]int{16, 0, 0}, 0xe700000828e2},   // wfssb %v16,%v0,%v0
		{[]int{0, 16, 0}, 0xe700000824e2},   // wfssb %v0,%v16,%v0
		{[]int{0, 0, 16}, 0xe700000822e2},   // wfssb %v0,%v0,%v16
		{[]int{15, 16, 31}, 0xe7f0f00826e2}, // wfssb %v15,%v16,%v31
		{[]int{31, 15, 16}, 0xe7ff00082ae2}, // wfssb %v31,%v15,%v16
		{[]int{16, 31, 15}, 0xe70ff0082ce2}, // wfssb %v16,%v31,%v15
		{[]int{31, 31, 31}, 0xe7fff0082ee2}, // wfssb %v31,%v31,%v31
	})
	vrrcEntry("vfmsb", "VFMSB V{1}, V{2}, V{0}", 0xe7, 0, []Case{
		{[]int{0, 0, 0}, 0xe700000020e7},    // vfmsb %v0,%v0,%v0
		{[]int{3, 1, 2}, 0xe731200020e7},    // vfmsb %v3,%v1,%v2
		{[]int{16, 0, 0}, 0xe700000028e7},   // vfmsb %v16,%v0,%v0
		{[]int{0, 16, 0}, 0xe700000024e7},   // vfmsb %v0,%v16,%v0
		{[]int{0, 0, 16}, 0xe700000022e7},   // vfmsb %v0,%v0,%v16
		{[]int{15, 16, 31}, 0xe7f0f00026e7}, // vfmsb %v15,%v16,%v31
		{[]int{31, 15, 16}, 0xe7ff00002ae7}, // vfmsb %v31,%v15,%v16
		{[]int{16, 31, 15}, 0xe70ff0002ce7}, // vfmsb %v16,%v31,%v15
		{[]int{31, 31, 31}, 0xe7fff0002ee7}, // vfmsb %v31,%v31,%v31
	})
	vrrcEntry("wfmsb", "WFMSB V{1}, V{2}, V{0}", 0xe7, 8, []Case{
		{[]int{0, 0, 0}, 0xe700000820e7},    // wfmsb %v0,%v0,%v0
		{[]int{3, 1, 2}, 0xe731200820e7},    // wfmsb %v3,%v1,%v2
		{[]int{16, 0, 0}, 0xe700000828e7},   // wfmsb %v16,%v0,%v0
		{[]int{0, 16, 0}, 0xe700000824e7},   // wfmsb %v0,%v16,%v0
		{[]int{0, 0, 16}, 0xe700000822e7},   // wfmsb %v0,%v0,%v16
		{[]int{15, 16, 31}, 0xe7f0f00826e7}, // wfmsb %v15,%v16,%v31
		{[]int{31, 15, 16}, 0xe7ff00082ae7}, // wfmsb %v31,%v15,%v16
		{[]int{16, 31, 15}, 0xe70ff0082ce7}, // wfmsb %v16,%v31,%v15
		{[]int{31, 31, 31}, 0xe7fff0082ee7}, // wfmsb %v31,%v31,%v31
	})
	vrrcEntry("vfdsb", "VFDSB V{2}, V{1}, V{0}", 0xe5, 0, []Case{
		{[]int{0, 0, 0}, 0xe700000020e5},    // vfdsb %v0,%v0,%v0
		{[]int{3, 1, 2}, 0xe731200020e5},    // vfdsb %v3,%v1,%v2
		{[]int{16, 0, 0}, 0xe700000028e5},   // vfdsb %v16,%v0,%v0
		{[]int{0, 16, 0}, 0xe700000024e5},   // vfdsb %v0,%v16,%v0
		{[]int{0, 0, 16}, 0xe700000022e5},   // vfdsb %v0,%v0,%v16
		{[]int{15, 16, 31}, 0xe7f0f00026e5}, // vfdsb %v15,%v16,%v31
		{[]int{31, 15, 16}, 0xe7ff00002ae5}, // vfdsb %v31,%v15,%v16
		{[]int{16, 31, 15}, 0xe70ff0002ce5}, // vfdsb %v16,%v31,%v15
		{[]int{31, 31, 31}, 0xe7fff0002ee5}, // vfdsb %v31,%v31,%v31
	})
	vrrcEntry("wfdsb", "WFDSB V{2}, V{1}, V{0}", 0xe5, 8, []Case{
		{[]int{0, 0, 0}, 0xe700000820e5},    // wfdsb %v0,%v0,%v0
		{[]int{3, 1, 2}, 0xe731200820e5},    // wfdsb %v3,%v1,%v2
		{[]int{16, 0, 0}, 0xe700000828e5},   // wfdsb %v16,%v0,%v0
		{[]int{0, 16, 0}, 0xe700000824e5},   // wfdsb %v0,%v16,%v0
		{[]int{0, 0, 16}, 0xe700000822e5},   // wfdsb %v0,%v0,%v16
		{[]int{15, 16, 31}, 0xe7f0f00826e5}, // wfdsb %v15,%v16,%v31
		{[]int{31, 15, 16}, 0xe7ff00082ae5}, // wfdsb %v31,%v15,%v16
		{[]int{16, 31, 15}, 0xe70ff0082ce5}, // wfdsb %v16,%v31,%v15
		{[]int{31, 31, 31}, 0xe7fff0082ee5}, // wfdsb %v31,%v31,%v31
	})
	// VRR-e: V1 = V2 × V3 ± V4, fused (one rounding). The format (2) sits
	// in bits 20–23, the single-element control in 28–31.
	vrreEntry := func(isa, plan9 string, op, m5 uint64, golden []Case) {
		a.insns = append(a.insns, &Insn{
			Arch: "s390x", ISA: isa, Plan9: plan9,
			Mask: 0xff000fff00ff, Bits: 0xe7<<40 | 2<<24 | m5<<16 | op,
			Encode: func(ops ...int) uint64 { return vrre(op, 2, m5, ops[0], ops[1], ops[2], ops[3]) },
			Source: s390xSource, Golden: golden,
		})
	}
	vrreEntry("vfmasb", "VFMASB V{1}, V{2}, V{3}, V{0}", 0x8f, 0, []Case{
		{[]int{0, 0, 0, 0}, 0xe7000200008f},     // vfmasb %v0,%v0,%v0,%v0
		{[]int{4, 1, 2, 3}, 0xe7412200308f},     // vfmasb %v4,%v1,%v2,%v3
		{[]int{16, 0, 0, 0}, 0xe7000200088f},    // vfmasb %v16,%v0,%v0,%v0
		{[]int{0, 16, 0, 0}, 0xe7000200048f},    // vfmasb %v0,%v16,%v0,%v0
		{[]int{0, 0, 16, 0}, 0xe7000200028f},    // vfmasb %v0,%v0,%v16,%v0
		{[]int{0, 0, 0, 16}, 0xe7000200018f},    // vfmasb %v0,%v0,%v0,%v16
		{[]int{15, 16, 31, 0}, 0xe7f0f200068f},  // vfmasb %v15,%v16,%v31,%v0
		{[]int{31, 15, 16, 15}, 0xe7ff0200fa8f}, // vfmasb %v31,%v15,%v16,%v15
		{[]int{16, 31, 15, 31}, 0xe70ff200fd8f}, // vfmasb %v16,%v31,%v15,%v31
		{[]int{31, 31, 31, 31}, 0xe7fff200ff8f}, // vfmasb %v31,%v31,%v31,%v31
	})
	vrreEntry("wfmasb", "WFMASB V{1}, V{2}, V{3}, V{0}", 0x8f, 8, []Case{
		{[]int{0, 0, 0, 0}, 0xe7000208008f},     // wfmasb %v0,%v0,%v0,%v0
		{[]int{4, 1, 2, 3}, 0xe7412208308f},     // wfmasb %v4,%v1,%v2,%v3
		{[]int{16, 0, 0, 0}, 0xe7000208088f},    // wfmasb %v16,%v0,%v0,%v0
		{[]int{0, 16, 0, 0}, 0xe7000208048f},    // wfmasb %v0,%v16,%v0,%v0
		{[]int{0, 0, 16, 0}, 0xe7000208028f},    // wfmasb %v0,%v0,%v16,%v0
		{[]int{0, 0, 0, 16}, 0xe7000208018f},    // wfmasb %v0,%v0,%v0,%v16
		{[]int{15, 16, 31, 0}, 0xe7f0f208068f},  // wfmasb %v15,%v16,%v31,%v0
		{[]int{31, 15, 16, 15}, 0xe7ff0208fa8f}, // wfmasb %v31,%v15,%v16,%v15
		{[]int{16, 31, 15, 31}, 0xe70ff208fd8f}, // wfmasb %v16,%v31,%v15,%v31
		{[]int{31, 31, 31, 31}, 0xe7fff208ff8f}, // wfmasb %v31,%v31,%v31,%v31
	})
	vrreEntry("vfmssb", "VFMSSB V{1}, V{2}, V{3}, V{0}", 0x8e, 0, []Case{
		{[]int{0, 0, 0, 0}, 0xe7000200008e},     // vfmssb %v0,%v0,%v0,%v0
		{[]int{4, 1, 2, 3}, 0xe7412200308e},     // vfmssb %v4,%v1,%v2,%v3
		{[]int{16, 0, 0, 0}, 0xe7000200088e},    // vfmssb %v16,%v0,%v0,%v0
		{[]int{0, 16, 0, 0}, 0xe7000200048e},    // vfmssb %v0,%v16,%v0,%v0
		{[]int{0, 0, 16, 0}, 0xe7000200028e},    // vfmssb %v0,%v0,%v16,%v0
		{[]int{0, 0, 0, 16}, 0xe7000200018e},    // vfmssb %v0,%v0,%v0,%v16
		{[]int{15, 16, 31, 0}, 0xe7f0f200068e},  // vfmssb %v15,%v16,%v31,%v0
		{[]int{31, 15, 16, 15}, 0xe7ff0200fa8e}, // vfmssb %v31,%v15,%v16,%v15
		{[]int{16, 31, 15, 31}, 0xe70ff200fd8e}, // vfmssb %v16,%v31,%v15,%v31
		{[]int{31, 31, 31, 31}, 0xe7fff200ff8e}, // vfmssb %v31,%v31,%v31,%v31
	})
	vrreEntry("wfmssb", "WFMSSB V{1}, V{2}, V{3}, V{0}", 0x8e, 8, []Case{
		{[]int{0, 0, 0, 0}, 0xe7000208008e},     // wfmssb %v0,%v0,%v0,%v0
		{[]int{4, 1, 2, 3}, 0xe7412208308e},     // wfmssb %v4,%v1,%v2,%v3
		{[]int{16, 0, 0, 0}, 0xe7000208088e},    // wfmssb %v16,%v0,%v0,%v0
		{[]int{0, 16, 0, 0}, 0xe7000208048e},    // wfmssb %v0,%v16,%v0,%v0
		{[]int{0, 0, 16, 0}, 0xe7000208028e},    // wfmssb %v0,%v0,%v16,%v0
		{[]int{0, 0, 0, 16}, 0xe7000208018e},    // wfmssb %v0,%v0,%v0,%v16
		{[]int{15, 16, 31, 0}, 0xe7f0f208068e},  // wfmssb %v15,%v16,%v31,%v0
		{[]int{31, 15, 16, 15}, 0xe7ff0208fa8e}, // wfmssb %v31,%v15,%v16,%v15
		{[]int{16, 31, 15, 31}, 0xe70ff208fd8e}, // wfmssb %v16,%v31,%v15,%v31
		{[]int{31, 31, 31, 31}, 0xe7fff208ff8e}, // wfmssb %v31,%v31,%v31,%v31
	})
	// VRR-a: V1 = sqrt(V2), correctly rounded. Format in m3, single-element
	// control in m4.
	vrraEntry := func(isa, plan9 string, op, m4 uint64, golden []Case) {
		a.insns = append(a.insns, &Insn{
			Arch: "s390x", ISA: isa, Plan9: plan9,
			Mask: 0xff00fffff0ff, Bits: 0xe7<<40 | m4<<16 | 2<<12 | op,
			Encode: func(ops ...int) uint64 { return vrra(op, 2, m4, ops[0], ops[1]) },
			Source: s390xSource, Golden: golden,
		})
	}
	vrraEntry("vfsqsb", "VFSQSB V{1}, V{0}", 0xce, 0, []Case{
		{[]int{0, 0}, 0xe700000020ce},   // vfsqsb %v0,%v0
		{[]int{2, 1}, 0xe721000020ce},   // vfsqsb %v2,%v1
		{[]int{16, 0}, 0xe700000028ce},  // vfsqsb %v16,%v0
		{[]int{0, 16}, 0xe700000024ce},  // vfsqsb %v0,%v16
		{[]int{15, 16}, 0xe7f0000024ce}, // vfsqsb %v15,%v16
		{[]int{31, 15}, 0xe7ff000028ce}, // vfsqsb %v31,%v15
		{[]int{31, 31}, 0xe7ff00002cce}, // vfsqsb %v31,%v31
	})
	vrraEntry("wfsqsb", "WFSQSB V{1}, V{0}", 0xce, 8, []Case{
		{[]int{0, 0}, 0xe700000820ce},   // wfsqsb %v0,%v0
		{[]int{2, 1}, 0xe721000820ce},   // wfsqsb %v2,%v1
		{[]int{16, 0}, 0xe700000828ce},  // wfsqsb %v16,%v0
		{[]int{0, 16}, 0xe700000824ce},  // wfsqsb %v0,%v16
		{[]int{15, 16}, 0xe7f0000824ce}, // wfsqsb %v15,%v16
		{[]int{31, 15}, 0xe7ff000828ce}, // wfsqsb %v31,%v15
		{[]int{31, 31}, 0xe7ff00082cce}, // wfsqsb %v31,%v31
	})
	arches = append(arches, a)
}

// The three formats, as z/Architecture lays them out (bit 0 is the most
// significant of the 48): opcode E7 in bits 0–7, the low four bits of V1, V2,
// V3 in 8–11, 12–15, 16–19, the mask fields, the RXB byte holding the high bit
// of V1..V4 in bits 36–39, and the second opcode byte in 40–47.

// vrrc encodes VRR-c with m4 in bits 32–35 and m5 in 28–31 (m6 = 0).
func vrrc(op, m4, m5 uint64, v1, v2, v3 int) uint64 {
	V1, V2, V3 := vreg(v1), vreg(v2), vreg(v3)
	return 0xe7<<40 | V1&15<<36 | V2&15<<32 | V3&15<<28 | m5<<16 | m4<<12 | rxb(V1, V2, V3, 0)<<8 | op
}

// vrre encodes VRR-e with m6 in bits 20–23, m5 in 28–31 and V4 in 32–35.
func vrre(op, m6, m5 uint64, v1, v2, v3, v4 int) uint64 {
	V1, V2, V3, V4 := vreg(v1), vreg(v2), vreg(v3), vreg(v4)
	return 0xe7<<40 | V1&15<<36 | V2&15<<32 | V3&15<<28 | m6<<24 | m5<<16 | V4&15<<12 | rxb(V1, V2, V3, V4)<<8 | op
}

// vrra encodes VRR-a with m4 in bits 28–31 and m3 in 32–35 (m5 = 0).
func vrra(op, m3, m4 uint64, v1, v2 int) uint64 {
	V1, V2 := vreg(v1), vreg(v2)
	return 0xe7<<40 | V1&15<<36 | V2&15<<32 | m4<<16 | m3<<12 | rxb(V1, V2, 0, 0)<<8 | op
}

func rxb(v1, v2, v3, v4 uint64) uint64 {
	return v1>>4<<3 | v2>>4<<2 | v3>>4<<1 | v4>>4
}

func vreg(r int) uint64 { return reg("s390x", "vector", r, 31) }
