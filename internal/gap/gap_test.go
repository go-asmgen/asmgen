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

func TestArches(t *testing.T) {
	if len(Arches()) != 2 || ArchOf("ppc64") == nil || ArchOf("loong64") == nil || ArchOf("mips") != nil {
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
		for _, bad := range []string{"1234567", "123456789", "zz345678"} {
			if _, err := a.ParseHex(bad); err == nil {
				t.Errorf("%s ParseHex(%q) accepted", a.Name, bad)
			}
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
