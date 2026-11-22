package amd64

import (
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/emit"
)

// TestVEXProbesNeedNoAVX512 pins the VEX.256 forms: VPCLMULQDQ256 is
// CPUID.7.0:ECX bit 10 and AVXVNNI is CPUID.7.1:EAX bit 4 (x/sys/cpu
// cpu_x86.go: cpuid_AVXVNNI = 1 << 4 in EAX, cpuid_AVX512VPCLMULQDQ = 1 << 10 in
// ECX), both behind AVX, OSXSAVE and XCR0 XMM|YMM and with NO AVX512F test and
// no 0xE6 mask: a CPU with the VEX form and no AVX-512 (Zen 3, Alder Lake) must
// answer true.
func TestVEXProbesNeedNoAVX512(t *testing.T) {
	for _, c := range []struct {
		f     Feature
		name  string
		wants []string
	}{
		{VPCLMULQDQ256, "hasCLMUL256", []string{"TESTL $0x400, CX"}},
		{AVXVNNI, "hasAVXVNNI", []string{"CMPL AX, $1", "MOVL $1, CX", "TESTL $0x10, AX"}},
	} {
		f := emit.NewFile("amd64")
		f.Add(FeatureProbe(c.name, c.f))
		s := f.String()
		at := -1
		for _, want := range append([]string{"CMPL AX, $7", "ANDL $0x18000000, CX", "XGETBV", "CMPL AX, $6"}, c.wants...) {
			i := strings.Index(s, want)
			if i < 0 {
				t.Fatalf("the %v probe does not contain %q:\n%s", c.f, want, s)
			}
			if i < at {
				t.Errorf("%v: %q comes before the step that must precede it:\n%s", c.f, want, s)
			}
			at = i
		}
		for _, bad := range []string{"$0x10000, BX", "$0xE6"} {
			if strings.Contains(s, bad) {
				t.Errorf("%v: probe contains %q, an AVX-512 requirement:\n%s", c.f, bad, s)
			}
		}
		if !strings.Contains(s, c.name+"_unsupported:") {
			t.Errorf("%v: no label of its own", c.f)
		}
	}
	// The 512-bit probe keeps requiring AVX512F.
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("hasCLMUL512", VPCLMULQDQ))
	if !strings.Contains(f.String(), "TESTL $0x10000, BX") {
		t.Errorf("VPCLMULQDQ (512-bit) no longer tests AVX512F")
	}
}
