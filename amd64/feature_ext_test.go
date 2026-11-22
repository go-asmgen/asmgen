package amd64

import (
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/emit"
)

// TestVNNIPopcntClmulProbesTestTheirBits pins the CPUID bits of the 512-bit
// extension probes (Intel SDM vol. 2A, CPUID leaf 07H, Table 3-8): VPCLMULQDQ
// is ECX bit 10 (0x400), AVX512VNNI bit 11 (0x800), AVX512VPOPCNTDQ bit 14
// (0x4000), each on top of AVX512F (EBX bit 16) and the OS checks AVX512F makes.
func TestVNNIPopcntClmulProbesTestTheirBits(t *testing.T) {
	for _, c := range []struct {
		f     Feature
		name  string
		wants []string
	}{
		{VPCLMULQDQ, "hasCLMUL", []string{"TESTL $0x10000, BX", "TESTL $0x400, CX"}},
		{AVX512VNNI, "hasVNNI", []string{"TESTL $0x10000, BX", "TESTL $0x800, CX"}},
		{AVX512VPOPCNTDQ, "hasVPOPCNT", []string{"TESTL $0x10000, BX", "TESTL $0x4000, CX"}},
	} {
		f := emit.NewFile("amd64")
		f.Add(FeatureProbe(c.name, c.f))
		s := f.String()
		at := -1
		for _, want := range append([]string{"CMPL AX, $7", "ANDL $0x18000000, CX", "XGETBV", "ANDL $0xE6, AX", "CMPL AX, $0xE6"}, c.wants...) {
			i := strings.Index(s, want)
			if i < 0 {
				t.Fatalf("the %v probe does not contain %q:\n%s", c.f, want, s)
			}
			if i < at {
				t.Errorf("%v: %q comes before the step that must precede it:\n%s", c.f, want, s)
			}
			at = i
		}
		if !strings.Contains(s, c.name+"_unsupported:") {
			t.Errorf("%v: no label of its own", c.f)
		}
	}
}

// TestBMI2AndF16CProbesTestTheirBits: BMI2 is CPUID.7.0:EBX bit 8 behind the
// max-leaf check and needs no XGETBV (general-purpose registers only); F16C is
// leaf 1 ECX bit 29 together with AVX (28) and OSXSAVE (27), then XCR0's XMM
// and YMM bits (Intel SDM vol. 2A, Tables 3-6 and 3-8).
func TestBMI2AndF16CProbesTestTheirBits(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("hasBMI2", BMI2))
	s := f.String()
	for _, want := range []string{"CMPL AX, $7", "TESTL $0x100, BX"} {
		if !strings.Contains(s, want) {
			t.Errorf("BMI2 probe does not contain %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "XGETBV") {
		t.Errorf("BMI2 probe reads XCR0, but needs no OS state:\n%s", s)
	}

	f = emit.NewFile("amd64")
	f.Add(FeatureProbe("hasF16C", F16C))
	s = f.String()
	at := -1
	for _, want := range []string{"ANDL $0x38000000, CX", "CMPL CX, $0x38000000", "XGETBV", "CMPL AX, $6"} {
		i := strings.Index(s, want)
		if i < 0 {
			t.Fatalf("F16C probe does not contain %q:\n%s", want, s)
		}
		if i < at {
			t.Errorf("F16C: %q comes before the step that must precede it:\n%s", want, s)
		}
		at = i
	}
	if strings.Contains(s, "CMPL AX, $7") {
		t.Errorf("F16C is a leaf-1 bit; it must not read leaf 7:\n%s", s)
	}
}
