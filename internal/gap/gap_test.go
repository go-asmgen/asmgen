package gap

import "testing"

func TestEveryEncodingMatchesItsReference(t *testing.T) {
	n := 0
	for _, in := range All() {
		if len(in.Golden) == 0 || in.Source == "" {
			t.Errorf("%s %s: no reference encodings, or no source for them", in.Arch, in.ISA)
		}
		for _, c := range in.Golden {
			if got := in.Encode(c.Ops...); got != c.Want {
				t.Errorf("%s %v: encoded %#08x, %s gives %#08x", in.ISA, c.Ops, got, in.Source, c.Want)
			}
			n++
		}
	}
	if n < 70+32 {
		t.Errorf("only %d reference cases; ppc64 alone had 70", n)
	}
}

// Mask/Bits is what finds an instruction in Go's test data under another
// name, so it must accept every reference word and fix only opcode bits.
func TestOpcodeMaskRecognisesEveryReference(t *testing.T) {
	for _, in := range All() {
		if in.Bits&^in.Mask != 0 {
			t.Errorf("%s: Bits %#08x has bits outside Mask %#08x", in.ISA, in.Bits, in.Mask)
		}
		for _, c := range in.Golden {
			if c.Want&in.Mask != in.Bits {
				t.Errorf("%s %v: %#08x not recognised by its mask", in.ISA, c.Ops, c.Want)
			}
		}
		for _, other := range All() {
			if other != in && other.Arch == in.Arch && other.Golden[0].Want&in.Mask == in.Bits {
				t.Errorf("%s's mask also matches %s", in.ISA, other.ISA)
			}
		}
	}
}

func TestSyntaxAndWord(t *testing.T) {
	in := Lookup("ppc64", "xvsubdp")
	if got := in.Syntax(3, 1, 2); got != "XVSUBDP VS1, VS2, VS3" {
		t.Errorf("Syntax = %q", got)
	}
	if got := in.Word(3, 2, 1); got != "WORD $0xf0620b40 // XVSUBDP VS2, VS1, VS3" {
		t.Errorf("Word = %q", got)
	}
	if got := Lookup("loong64", "vfmadd.d").Syntax(3, 2, 1, 4); got != "VFMADDD V4, V1, V2, V3" {
		t.Errorf("loong64 Syntax = %q", got)
	}
	if got := Lookup("loong64", "xvldrepl.d").Syntax(9, 5, -2048); got != "XVMOVQ -2048(R5), X9.V4" {
		t.Errorf("load Syntax = %q", got)
	}
}

func TestTestdataLine(t *testing.T) {
	be, le := ArchOf("ppc64"), ArchOf("loong64")
	if got := be.TestdataLine(Lookup("ppc64", "xvadddp"), Case{[]int{3, 2, 1}, 0xf0620b00}); got != "\tXVADDDP VS2, VS1, VS3           // f0620b00" {
		t.Errorf("ppc64: %q", got)
	}
	if got := le.TestdataLine(Lookup("loong64", "vldrepl.d"), Case{[]int{0, 4, 0}, 0x30100080}); got != "\tVMOVQ\t\t(R4), V0.V2\t// 80001030" {
		t.Errorf("loong64: %q", got)
	}
	if got := le.TestdataLine(Lookup("loong64", "xvfmadd.d"), Case{[]int{3, 2, 1, 4}, 0x0a220443}); got != "\tXVFMADDD\tX4, X1, X2, X3\t// 4304220a" {
		t.Errorf("loong64, 8-letter mnemonic: %q", got)
	}
	if got := le.TestdataLine(Lookup("loong64", "vldrepl.d"), Case{[]int{0, 4, 2040}, 0}); got != "\tVMOVQ\t\t2040(R4), V0.V2\t// 00000000" {
		t.Errorf("a non-zero offset was rewritten: %q", got)
	}
}

func TestArches(t *testing.T) {
	if len(Arches()) != 3 || ArchOf("ppc64") == nil || ArchOf("loong64") == nil || ArchOf("s390x") == nil || ArchOf("mips") != nil {
		t.Fatalf("Arches = %v", Arches())
	}
	for _, a := range Arches() {
		if len(a.Insns()) == 0 || len(a.GOARCH) == 0 || a.Testdata == "" {
			t.Errorf("%s is incomplete", a.Name)
		}
	}
	be, le := ArchOf("ppc64"), ArchOf("loong64")
	if be.Hex(0xf0620b40) != "f0620b40" || le.Hex(0x0825d2ec) != "ecd22508" {
		t.Errorf("Hex: %s %s", be.Hex(0xf0620b40), le.Hex(0x0825d2ec))
	}
	for _, a := range Arches() {
		w, err := a.ParseHex(a.Hex(0x12345678))
		if err != nil || w != 0x12345678 {
			t.Errorf("%s ParseHex(Hex) = %#x, %v", a.Name, w, err)
		}
		for _, bad := range []string{"1234567", "123456789", "zz345678", "12345678901", "1234567890123", "zz3456789012"} {
			if _, err := a.ParseHex(bad); err == nil {
				t.Errorf("%s ParseHex(%q) accepted", a.Name, bad)
			}
		}
	}
}

// s390x instructions are 6 bytes: the encoding is 48 bits, written as twelve
// hex digits in memory order, and emitted as a WORD and two BYTEs.
func TestSixByteInstructions(t *testing.T) {
	a := ArchOf("s390x")
	if got := a.Hex(0xe731200020e3); got != "e731200020e3" {
		t.Errorf("Hex = %q", got)
	}
	if got := a.Hex(0x20e3); got != "0000000020e3" {
		t.Errorf("Hex keeps leading zero bytes: %q", got)
	}
	if w, err := a.ParseHex("e7412200308f"); err != nil || w != 0xe7412200308f {
		t.Errorf("ParseHex = %#x, %v", w, err)
	}
	if _, err := a.ParseHex("f0620b40"); err == nil {
		t.Error("a 4-byte encoding was read as a 6-byte one")
	}
	if got := Lookup("s390x", "vfasb").Word(3, 1, 2); got != "WORD $0xe7312000; BYTE $0x20; BYTE $0xe3 // VFASB V1, V2, V3" {
		t.Errorf("Word = %q", got)
	}
	if got := Lookup("s390x", "wfmssb").Word(31, 15, 16, 15); got != "WORD $0xe7ff0208; BYTE $0xfa; BYTE $0x8e // WFMSSB V15, V16, V15, V31" {
		t.Errorf("Word = %q", got)
	}
}

// The lines go.dev/cl/846286 adds to cmd/asm's s390x.s, verbatim: the
// registry must reproduce each one, spelling, operand order, layout and
// encoding, from the ISA operands.
func TestS390xTestdataMatchesTheCL(t *testing.T) {
	a := ArchOf("s390x")
	for _, tc := range []struct {
		isa  string
		ops  []int
		line string
	}{
		{"vfasb", []int{3, 1, 2}, "\tVFASB\tV1, V2, V3              // e731200020e3"},
		{"wfasb", []int{6, 4, 5}, "\tWFASB\tV4, V5, V6              // e764500820e3"},
		{"vfssb", []int{9, 8, 7}, "\tVFSSB\tV7, V8, V9              // e798700020e2"},
		{"wfssb", []int{12, 11, 10}, "\tWFSSB\tV10, V11, V12           // e7cba00820e2"},
		{"vfmsb", []int{15, 13, 14}, "\tVFMSB\tV13, V14, V15           // e7fde00020e7"},
		{"wfmsb", []int{3, 1, 2}, "\tWFMSB\tV1, V2, V3              // e731200820e7"},
		{"vfdsb", []int{6, 5, 4}, "\tVFDSB\tV4, V5, V6              // e765400020e5"},
		{"wfdsb", []int{9, 8, 7}, "\tWFDSB\tV7, V8, V9              // e798700820e5"},
		{"vfsqsb", []int{2, 1}, "\tVFSQSB\tV1, V2                  // e721000020ce"},
		{"wfsqsb", []int{4, 3}, "\tWFSQSB\tV3, V4                  // e743000820ce"},
		{"vfmasb", []int{4, 1, 2, 3}, "\tVFMASB\tV1, V2, V3, V4          // e7412200308f"},
		{"wfmasb", []int{8, 5, 6, 7}, "\tWFMASB\tV5, V6, V7, V8          // e7856208708f"},
		{"vfmssb", []int{12, 9, 10, 11}, "\tVFMSSB\tV9, V10, V11, V12       // e7c9a200b08e"},
		{"wfmssb", []int{0, 13, 14, 15}, "\tWFMSSB\tV13, V14, V15, V0       // e70de208f08e"},
	} {
		in := Lookup("s390x", tc.isa)
		if got := a.TestdataLine(in, Case{tc.ops, in.Encode(tc.ops...)}); got != tc.line {
			t.Errorf("%s:\n got %q\nwant %q", tc.isa, got, tc.line)
		}
	}
}

func TestRefusals(t *testing.T) {
	for name, f := range map[string]func(){
		"unknown entry": func() { Lookup("ppc64", "xvfoo") },
		"VS64":          func() { Lookup("ppc64", "xvadddp").Encode(64, 0, 0) },
		"VS-1 sqrt":     func() { Lookup("ppc64", "xvsqrtdp").Encode(0, -1) },
		"vector 32":     func() { Lookup("loong64", "xvfmadd.d").Encode(32, 0, 0, 0) },
		"general 32":    func() { Lookup("loong64", "vldrepl.d").Encode(0, 32, 0) },
		"offset not ×8": func() { Lookup("loong64", "xvldrepl.d").Encode(0, 4, 4) },
		"offset 2048":   func() { Lookup("loong64", "xvldrepl.d").Encode(0, 4, 2048) },
		"offset -2056":  func() { Lookup("loong64", "vldrepl.d").Encode(0, 4, -2056) },
		"V32 target":    func() { Lookup("s390x", "vfasb").Encode(32, 0, 0) },
		"V32 source":    func() { Lookup("s390x", "vfssb").Encode(0, 0, 32) },
		"V-1 fma":       func() { Lookup("s390x", "vfmasb").Encode(0, 0, 0, -1) },
		"V32 sqrt":      func() { Lookup("s390x", "wfsqsb").Encode(0, 32) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: accepted", name)
				}
			}()
			f()
		}()
	}
}
