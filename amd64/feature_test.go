package amd64

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/emit"
)

// handWrittenAVX2Probe is go-fft/fft's internal/kernels/cpu_amd64.s as it was
// committed there, before this function existed. It is the reason this file is
// here, and it is the thing the generated probe has to agree with: a package
// that swaps 24 hand-written lines for a generator call must not thereby change
// what its CPU gate decides.
const handWrittenAVX2Probe = `
TEXT ·supportsAVX2(SB), NOSPLIT, $0-1
	MOVB $0, ret+0(FP)
	XORL AX, AX
	CPUID
	CMPL AX, $7
	JL done
	MOVL $1, AX
	CPUID
	ANDL $0x18000000, CX // AVX and OSXSAVE
	CMPL CX, $0x18000000
	JNE done
	XORL CX, CX
	XGETBV
	ANDL $6, AX // XMM and YMM state enabled in XCR0
	CMPL AX, $6
	JNE done
	MOVL $7, AX
	XORL CX, CX
	CPUID
	TESTL $0x20, BX // AVX2
	JZ done
	MOVB $1, ret+0(FP)
done:
	RET
`

var labelWord = regexp.MustCompile(`\b(done|supportsAVX2_unsupported)\b`)

// body strips the file header and normalises the one thing the two are allowed
// to differ on -- the label's spelling -- so the comparison is about the
// instruction sequence and nothing else.
func body(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(labelWord.ReplaceAllString(l, "L"))
		switch {
		case l == "", strings.HasPrefix(l, "//"), strings.HasPrefix(l, "#include"):
			continue
		}
		out = append(out, l)
	}
	return out
}

func TestAVX2ProbeMatchesTheHandWrittenOne(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("supportsAVX2", AVX2))

	got, want := body(f.String()), body(handWrittenAVX2Probe)
	if len(got) != len(want) {
		t.Fatalf("%d instructions, want %d:\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got  %q\n want %q", i, got[i], want[i])
		}
	}
}

// TestTheProbeChecksTheOSAndNotJustTheCPU pins the half that is easy to drop
// and expensive to drop: a CPU can report AVX2 on a kernel that does not save
// YMM state, and then the upper lanes are lost at the next context switch. The
// feature bit alone is a promise the OS has not made.
func TestTheProbeChecksTheOSAndNotJustTheCPU(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("supportsAVX2", AVX2))
	s := f.String()
	for _, want := range []string{
		"ANDL $0x18000000, CX", // OSXSAVE, without which XGETBV is illegal
		"XGETBV",
		"ANDL $6, AX",     // XCR0 XMM+YMM
		"CMPL AX, $7",     // leaf 7 exists before leaf 7 is read
		"TESTL $0x20, BX", // and only then the AVX2 bit
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the probe does not contain %q:\n%s", want, s)
		}
	}
}

// TestTwoProbesInOneFileDoNotCollide is why the label is derived from the name.
// The hand-written original branched to a fixed `done`, which is correct for
// exactly one probe per file and a duplicate-symbol error at the second --
// which is the shape go-simd/popcount would need, since it gates on both
// POPCNT and AVX2.
func TestTwoProbesInOneFileDoNotCollide(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("hasAVX2", AVX2))
	f.Add(FeatureProbe("hasPOPCNT", POPCNT))
	s := f.String()

	labels := regexp.MustCompile(`(?m)^(\w+):`).FindAllStringSubmatch(s, -1)
	if len(labels) != 2 {
		t.Fatalf("found %d labels, want 2:\n%s", len(labels), s)
	}
	if labels[0][1] == labels[1][1] {
		t.Errorf("both probes branch to %q; the assembler would refuse the file", labels[0][1])
	}
	for _, want := range []string{"hasAVX2_unsupported", "hasPOPCNT_unsupported"} {
		if !strings.Contains(s, want+":") {
			t.Errorf("no label %q in:\n%s", want, s)
		}
	}
}

// TestPOPCNTNeedsNoOSState: unlike AVX2, POPCNT writes a general-purpose
// register, so there is no saved state for the OS to lose and no XGETBV in the
// probe. A probe that checked XCR0 here would refuse the instruction on
// perfectly good hardware.
func TestPOPCNTNeedsNoOSState(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("hasPOPCNT", POPCNT))
	s := f.String()
	if strings.Contains(s, "XGETBV") {
		t.Errorf("the POPCNT probe checks OS vector state it does not need:\n%s", s)
	}
	if !strings.Contains(s, "TESTL $0x800000, CX") {
		t.Errorf("the POPCNT probe does not test CPUID.1:ECX bit 23:\n%s", s)
	}
}

func TestFeatureString(t *testing.T) {
	for _, c := range []struct {
		f    Feature
		want string
	}{{AVX2, "AVX2"}, {POPCNT, "POPCNT"}, {AVX512F, "AVX512F"}, {FMA, "FMA"}, {Feature(99), "Feature(?)"}} {
		if got := c.f.String(); got != c.want {
			t.Errorf("Feature(%d).String() = %q, want %q", c.f, got, c.want)
		}
	}
}

// TestAnUnknownFeatureIsRefusedRatherThanWavedThrough: every branch of the
// switch is followed by "report supported", so an unhandled Feature would emit
// a gate that unconditionally enables a kernel the CPU may not have. A
// generator that cannot ask the question must not answer it.
func TestAnUnknownFeatureIsRefusedRatherThanWavedThrough(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("an unknown Feature produced a probe instead of a refusal")
		}
		if !strings.Contains(fmt.Sprint(r), "unknown Feature(99)") {
			t.Errorf("panic = %v, want it to name the feature", r)
		}
	}()
	FeatureProbe("hasNothing", Feature(99))
}

// TestAVX512FChecksAllFiveStateComponents pins what makes AVX-512 different
// from AVX2: the OS has to save three more state components (the opmask
// registers, the upper halves of ZMM0–15, and ZMM16–31), so the XCR0 mask is
// 0xE6, not AVX2's 6. A probe that reused AVX2's mask would enable 512-bit
// kernels on a kernel that does not save ZMM state.
func TestAVX512FChecksAllFiveStateComponents(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("hasAVX512F", AVX512F))
	s := f.String()
	order := []string{
		"CMPL AX, $7",          // leaf 7 exists
		"ANDL $0x18000000, CX", // OSXSAVE (and AVX) before XGETBV
		"XGETBV",
		"ANDL $0xE6, AX", // XMM|YMM|opmask|ZMM_Hi256|Hi16_ZMM
		"CMPL AX, $0xE6",
		"TESTL $0x10000, BX", // CPUID.7.0:EBX bit 16, last
	}
	at := -1
	for _, want := range order {
		i := strings.Index(s, want)
		if i < 0 {
			t.Fatalf("the AVX512F probe does not contain %q:\n%s", want, s)
		}
		if i < at {
			t.Errorf("%q comes before the step that must precede it:\n%s", want, s)
		}
		at = i
	}
	if strings.Contains(s, "ANDL $6, AX") {
		t.Errorf("the AVX512F probe checks only AVX2's XCR0 bits:\n%s", s)
	}
}

// TestAVX512FAndAVX2ProbesShareAFile: a package dispatching AVX-512 with an
// AVX2 fallback needs both gates in one file.
func TestAVX512FAndAVX2ProbesShareAFile(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("hasAVX2", AVX2))
	f.Add(FeatureProbe("hasAVX512F", AVX512F))
	s := f.String()
	for _, want := range []string{"hasAVX2_unsupported:", "hasAVX512F_unsupported:"} {
		if !strings.Contains(s, want) {
			t.Errorf("no label %q in:\n%s", want, s)
		}
	}
}

// TestFMAChecksLeafOneAndTheOS pins the two things an FMA gate must not get
// wrong. The feature bit is CPUID.1:ECX bit 12 -- not a leaf-7 bit like AVX2 --
// and it is tested together with AVX and OSXSAVE in one mask, so a CPU that
// reports FMA with AVX disabled says no. And because VFMADD* writes YMM
// registers, XCR0 must show YMM state saved: the same OS half as AVX2.
func TestFMAChecksLeafOneAndTheOS(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(FeatureProbe("hasFMA", FMA))
	s := f.String()
	for _, want := range []string{
		"ANDL $0x18001000, CX", // FMA | AVX | OSXSAVE
		"CMPL CX, $0x18001000", // all three, not any
		"XGETBV",
		"ANDL $6, AX", // XCR0 XMM+YMM
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the FMA probe does not contain %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "MOVL $7, AX") {
		t.Errorf("the FMA probe reads leaf 7, where FMA is not:\n%s", s)
	}
}

// TestVendorProbeSpellsTheStringInCPUIDOrder checks the three constants for
// "GenuineIntel" against the values the Intel SDM gives for CPUID leaf 0
// (EBX = "Genu" 0x756E6547, EDX = "ineI" 0x49656E69, ECX = "ntel" 0x6C65746E):
// the register order is EBX, EDX, ECX, not alphabetical, and a probe that
// compared ECX second would never match.
func TestVendorProbeSpellsTheStringInCPUIDOrder(t *testing.T) {
	f := emit.NewFile("amd64")
	f.Add(VendorProbe("isIntel", "GenuineIntel"))
	got := body(f.String())
	want := []string{
		"TEXT ·isIntel(SB), NOSPLIT, $0-1",
		"MOVB $0, ret+0(FP)",
		"XORL AX, AX",
		"CPUID",
		`CMPL BX, $0x756e6547 // "Genu"`,
		"JNE isIntel_other",
		`CMPL DX, $0x49656e69 // "ineI"`,
		"JNE isIntel_other",
		`CMPL CX, $0x6c65746e // "ntel"`,
		"JNE isIntel_other",
		"MOVB $1, ret+0(FP)",
		"isIntel_other:",
		"RET",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("VendorProbe(GenuineIntel):\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestVendorProbeRefusesAWrongLength: CPUID leaf 0 holds exactly twelve bytes.
func TestVendorProbeRefusesAWrongLength(t *testing.T) {
	for _, v := range []string{"", "Intel", "GenuineIntel!"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("VendorProbe(%q) did not panic", v)
				}
			}()
			VendorProbe("p", v)
		}()
	}
}
