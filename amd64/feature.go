package amd64

// Feature probes: the CPUID/XGETBV dance that every dispatched SIMD kernel
// needs before it may use one.
//
// This is here because the fleet had two answers to one question. go-fft/fft
// hand-wrote 24 lines of Plan 9 assembly for AVX2; go-simd/popcount and
// go-simd/base64 instead took a runtime dependency on golang.org/x/sys/cpu,
// which computes the same predicate. Neither is wrong, and having both is:
// a generator that already writes the kernel should be able to write the gate
// in front of it, so a package that wants no runtime dependency does not have
// to hand-write CPUID to get one.
//
// ⛔ The OS half is not optional and is the part hand-written probes get wrong.
// A CPU that reports AVX2 on a kernel that does not save YMM state will fault
// or silently corrupt the upper lanes on a context switch, so the probe checks
// OSXSAVE, then XGETBV's XMM and YMM bits, and only then the feature bit.
// x/sys/cpu does exactly this (HasAVX2 = isSet(5, ebx7) && osSupportsAVX), and
// so does the sequence below.

import (
	"fmt"

	"github.com/go-asmgen/asmgen/emit"
)

// Feature is a CPU feature a probe can report on.
type Feature int

const (
	// AVX2 reports 256-bit integer and floating-point vector support, with the
	// OS saving YMM state. Callers: go-fft/fft's butterfly kernels.
	AVX2 Feature = iota
	// POPCNT reports the population-count instruction. Callers:
	// go-simd/popcount.
	POPCNT
	// AVX512F reports the AVX-512 foundation (512-bit ZMM registers and the
	// opmask registers K0–K7), with the OS saving all of their state. Callers:
	// go-fft/fft's Stockham pass kernels.
	//
	// On macOS the probe answers false even on hardware that has AVX-512:
	// Darwin enables ZMM state lazily, on the first fault, so XCR0 does not
	// show it to a process that has not used it yet (golang.org/x/sys/cpu reads
	// a sysctl there instead). A false negative only means the caller falls
	// back to its AVX2 kernel; a false positive would fault.
	AVX512F
	// FMA reports the three-operand fused multiply-add (FMA3: VFMADD231PD and
	// family), with the OS saving YMM state. It is a separate CPUID bit from
	// AVX2 (leaf 1 ECX bit 12, not leaf 7), so a kernel that issues VFMADD*
	// must probe it on its own: every Intel and AMD part since Haswell and
	// Piledriver has both, but a hypervisor can mask one without the other.
	// Callers: go-ndarray/ndarray's GEMM micro-kernel.
	FMA
)

// The list stops there on purpose. Every entry costs a probe sequence that has
// to be right about its own OS-state requirement, and one nobody calls is one
// nobody has run on hardware that lacks the feature. Add the next when a
// caller needs it, not before.

func (f Feature) String() string {
	switch f {
	case AVX2:
		return "AVX2"
	case POPCNT:
		return "POPCNT"
	case AVX512F:
		return "AVX512F"
	case FMA:
		return "FMA"
	}
	return "Feature(?)"
}

// FeatureProbe returns a `func() bool` reporting whether both the processor
// and the OS support f. The emitted function is NOSPLIT with no arguments and
// a single bool result, so the Go side is one line:
//
//	func supportsAVX2() bool
//
// The label it branches to is derived from name, so several probes can share
// one file — a fixed label would collide the moment a package needs two.
func FeatureProbe(name string, f Feature) *emit.Function {
	sig := Layout(nil, nil, []string{"ret"}, []Type{Uint8})
	b := NewFunc(name, sig, 0)
	done := name + "_unsupported"

	// False first, so every branch out is a jump to the end and no path can
	// fall through leaving the slot untouched.
	b.Raw("MOVB $0, ret+0(FP)")

	switch f {
	case POPCNT:
		// CPUID leaf 1, ECX bit 23. No OS state to check: POPCNT writes a
		// general-purpose register.
		b.Raw("MOVL $1, AX")
		b.Raw("CPUID")
		b.Raw("TESTL $0x800000, CX // POPCNT")
		b.Raw("JZ %s", done)

	case AVX2:
		// Leaf 7 has to exist before it can be read: on a CPU whose maximum
		// leaf is below 7, CPUID returns the highest supported leaf instead
		// and the AVX2 bit would be read out of somebody else's answer.
		b.Raw("XORL AX, AX")
		b.Raw("CPUID")
		b.Raw("CMPL AX, $7")
		b.Raw("JL %s", done)

		// Leaf 1: AVX (bit 28) and OSXSAVE (bit 27) together. XGETBV below is
		// itself only legal once OSXSAVE says so.
		b.Raw("MOVL $1, AX")
		b.Raw("CPUID")
		b.Raw("ANDL $0x18000000, CX // AVX and OSXSAVE")
		b.Raw("CMPL CX, $0x18000000")
		b.Raw("JNE %s", done)

		// XCR0 bits 1 and 2: the OS saves XMM and YMM state across a context
		// switch. Without this the feature bit is a promise the OS does not keep.
		b.Raw("XORL CX, CX")
		b.Raw("XGETBV")
		b.Raw("ANDL $6, AX // XMM and YMM state enabled in XCR0")
		b.Raw("CMPL AX, $6")
		b.Raw("JNE %s", done)

		// Leaf 7, EBX bit 5.
		b.Raw("MOVL $7, AX")
		b.Raw("XORL CX, CX")
		b.Raw("CPUID")
		b.Raw("TESTL $0x20, BX // AVX2")
		b.Raw("JZ %s", done)

	case AVX512F:
		// Leaf 7 must exist before it is read, as for AVX2.
		b.Raw("XORL AX, AX")
		b.Raw("CPUID")
		b.Raw("CMPL AX, $7")
		b.Raw("JL %s", done)

		// Leaf 1: OSXSAVE (bit 27) makes XGETBV legal; AVX (bit 28) is part of
		// the requirement, since AVX-512 extends the VEX register file.
		b.Raw("MOVL $1, AX")
		b.Raw("CPUID")
		b.Raw("ANDL $0x18000000, CX // AVX and OSXSAVE")
		b.Raw("CMPL CX, $0x18000000")
		b.Raw("JNE %s", done)

		// XCR0 bits 1, 2 (XMM, YMM) and 5, 6, 7 (opmask, the upper halves of
		// ZMM0–15, and ZMM16–31): all five must be saved by the OS. Intel SDM
		// vol. 1, 15.2 "Detection of AVX-512 Foundation Instructions"; x/sys/cpu
		// tests the same bits (osSupportsAVX512).
		b.Raw("XORL CX, CX")
		b.Raw("XGETBV")
		b.Raw("ANDL $0xE6, AX // XMM, YMM, opmask, ZMM_Hi256 and Hi16_ZMM state in XCR0")
		b.Raw("CMPL AX, $0xE6")
		b.Raw("JNE %s", done)

		// Leaf 7, EBX bit 16.
		b.Raw("MOVL $7, AX")
		b.Raw("XORL CX, CX")
		b.Raw("CPUID")
		b.Raw("TESTL $0x10000, BX // AVX512F")
		b.Raw("JZ %s", done)

	case FMA:
		// Leaf 1 alone: FMA3 is a leaf-1 ECX bit, so unlike AVX2 there is no
		// leaf-7 read and no max-leaf check in front of one. The OS half is
		// AVX2's: the instructions are VEX-encoded and write YMM registers.
		b.Raw("MOVL $1, AX")
		b.Raw("CPUID")
		b.Raw("ANDL $0x18001000, CX // FMA, AVX and OSXSAVE")
		b.Raw("CMPL CX, $0x18001000")
		b.Raw("JNE %s", done)

		b.Raw("XORL CX, CX")
		b.Raw("XGETBV")
		b.Raw("ANDL $6, AX // XMM and YMM state enabled in XCR0")
		b.Raw("CMPL AX, $6")
		b.Raw("JNE %s", done)

	default:
		// A generator must not emit a gate that says yes without asking. There
		// is no sensible fallback here: every path out of this switch is
		// followed by "report supported", so an unhandled Feature would produce
		// a probe that unconditionally enables a kernel the CPU may not have.
		// This is a build-time tool, so it fails at build time.
		panic(fmt.Sprintf("amd64.FeatureProbe: unknown Feature(%d)", int(f)))
	}

	b.Raw("MOVB $1, ret+0(FP)")
	b.Label(done)
	b.Ret()
	return b.Func()
}

// VendorProbe returns a `func() bool` reporting whether the processor's CPUID
// vendor string (leaf 0) is vendor: "GenuineIntel", "AuthenticAMD",
// "HygonGenuine", and so on. It is for tuning choices measured to differ by
// vendor — never for correctness, which a feature probe decides. CPUID leaf 0
// returns the twelve bytes in EBX, EDX and ECX, in that order, little-endian;
// the probe compares the three registers against the constants the string
// spells. A hypervisor may report its own vendor string, so a caller must
// treat false as "not known to be this vendor" and keep a safe default.
//
// vendor must be exactly 12 bytes long: CPUID leaf 0 has room for no more and
// no fewer, and a shorter string would compare against bytes the caller never
// wrote. A wrong length is a programming error in the generator, so it panics.
func VendorProbe(name, vendor string) *emit.Function {
	if len(vendor) != 12 {
		panic(fmt.Sprintf("amd64.VendorProbe: vendor %q is %d bytes, CPUID leaf 0 holds exactly 12", vendor, len(vendor)))
	}
	word := func(i int) uint32 {
		return uint32(vendor[i]) | uint32(vendor[i+1])<<8 | uint32(vendor[i+2])<<16 | uint32(vendor[i+3])<<24
	}
	sig := Layout(nil, nil, []string{"ret"}, []Type{Uint8})
	b := NewFunc(name, sig, 0)
	done := name + "_other"
	b.Raw("MOVB $0, ret+0(FP)")
	b.Raw("XORL AX, AX")
	b.Raw("CPUID")
	b.Raw("CMPL BX, $0x%08x // %q", word(0), vendor[0:4])
	b.Raw("JNE %s", done)
	b.Raw("CMPL DX, $0x%08x // %q", word(4), vendor[4:8])
	b.Raw("JNE %s", done)
	b.Raw("CMPL CX, $0x%08x // %q", word(8), vendor[8:12])
	b.Raw("JNE %s", done)
	b.Raw("MOVB $1, ret+0(FP)")
	b.Label(done)
	b.Ret()
	return b.Func()
}
