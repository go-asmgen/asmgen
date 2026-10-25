// Package gap lists the instructions go-asmgen needs and the Go assembler
// cannot (yet) assemble, and holds what is known about each one: the
// encoding, the reference encodings it was checked against, and the Plan 9
// spelling proposed for cmd/asm.
//
// go-asmgen emits Plan 9 text and leaves encoding to cmd/asm. An entry here is
// the exception: until cmd/asm has the instruction, the architecture package
// emits a WORD from Encode, commented with the proposed spelling. The same
// entry is what tools/goasmgap turns into cmd/asm test lines for an upstream
// patch, and what it checks a toolchain against, so that the day Go gains the
// instruction is noticed and the WORD can go.
package gap

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// An Insn is one instruction cmd/asm lacks, for all operands or some.
type Insn struct {
	// Arch is the architecture family, as go-asmgen names its packages
	// ("ppc64", "loong64").
	Arch string
	// ISA is the manufacturer's mnemonic ("xvadddp", "vfmadd.d"). It
	// identifies the entry within its Arch.
	ISA string
	// Plan9 is the proposed cmd/asm form, with {i} standing for operand i in
	// ISA order: "XVADDDP VS{1}, VS{2}, VS{0}".
	Plan9 string
	// Mask and Bits fix the opcode: an encoding w is this instruction,
	// whatever its operands, iff w&Mask == Bits. This finds the instruction
	// in Go's test data under any spelling.
	//
	// An encoding is the instruction's bytes read as one big-endian number,
	// Arch.Size bytes long: the 32-bit word on ppc64 and loong64, the 48 bits
	// of a 6-byte s390x vector instruction.
	Mask, Bits uint64
	// NotMask and NotBits, when NotMask is not zero, carve an exception out
	// of Mask/Bits: w is not this instruction if w&NotMask == NotBits. It is
	// for an operand field whose values select other instructions: loong64
	// csrxchg with rj 0 or 1 is csrrd or csrwr.
	NotMask, NotBits uint64
	// Encode returns the encoding for operands in ISA order. It panics on an
	// operand the instruction cannot encode.
	Encode func(ops ...int) uint64
	// Golden lists encodings produced by an independent assembler (named in
	// Source), operands in ISA order.
	Golden []Case
	Source string
	// Note says what is missing when cmd/asm has the instruction but not
	// all of its operands.
	Note string
}

// A Case is one reference encoding.
type Case struct {
	Ops  []int
	Want uint64
}

// Matches says whether the encoding w is this instruction, whatever its
// operands.
func (in *Insn) Matches(w uint64) bool {
	return w&in.Mask == in.Bits && (in.NotMask == 0 || w&in.NotMask != in.NotBits)
}

// Syntax renders the proposed Plan 9 form for the operands.
func (in *Insn) Syntax(ops ...int) string {
	s := in.Plan9
	for i, v := range ops {
		s = strings.ReplaceAll(s, "{"+strconv.Itoa(i)+"}", strconv.Itoa(v))
	}
	return s
}

// Word is the line an architecture package emits while cmd/asm lacks the
// instruction: the encoding, and the spelling it will have.
//
// A 6-byte instruction (s390x) is a WORD of its first four bytes and a BYTE
// for each of the other two, on one line: cmd/asm writes WORD big-endian on
// s390x and pads neither, so the three directives lay down exactly the
// instruction's bytes, and one line keeps them from being separated.
func (in *Insn) Word(ops ...int) string {
	w := in.Encode(ops...)
	if a := ArchOf(in.Arch); a != nil && a.size() == 6 {
		return fmt.Sprintf("WORD $0x%08x; BYTE $0x%02x; BYTE $0x%02x // %s", w>>16, byte(w>>8), byte(w), in.Syntax(ops...))
	}
	return fmt.Sprintf("WORD $0x%08x // %s", w, in.Syntax(ops...))
}

// All returns every entry, architecture by architecture.
func All() []*Insn {
	var all []*Insn
	for _, a := range arches {
		all = append(all, a.insns...)
	}
	return all
}

// Lookup returns the entry for an ISA mnemonic, or panics: an architecture
// package naming an entry that does not exist is a programming error.
func Lookup(arch, isa string) *Insn {
	for _, in := range All() {
		if in.Arch == arch && in.ISA == isa {
			return in
		}
	}
	panic(fmt.Sprintf("gap: no entry %s %s", arch, isa))
}

// An Arch says how Go's assembler tests one architecture family.
type Arch struct {
	Name string
	// GOARCH lists the targets to assemble for; the first is the one Go's
	// test data for this family is written for.
	GOARCH []string
	// Testdata is the file under src/cmd/asm/internal/asm/testdata that a
	// patch adds its lines to.
	Testdata string
	// Size is the length of the family's entries in bytes: 4 when zero, 6
	// for the s390x vector instructions.
	Size int
	// BigEndianTestdata says how that file writes an encoding: as the bytes
	// in memory order (ppc64, whose tests assemble big-endian, and s390x)
	// or as the word's bytes in little-endian order (loong64).
	BigEndianTestdata bool
	// TabbedTestdata says the file tabs the operands to column 16
	// (loong64enc1.s), rather than padding the instruction to 32 columns
	// (ppc64.s).
	TabbedTestdata bool
	// OperandPad, when not zero, says the file puts a tab after the
	// mnemonic and pads the operands to this many columns (s390x.s:
	// "\tVFADB\tV1, V2, V3              // ...", 24).
	OperandPad int
	// InsnPad, when not zero, pads the whole instruction to this many
	// columns instead of 32 (arm64enc.s: "\tVFMLA V6.S[0], F2, F14" and its
	// neighbours put the comment at column 44).
	InsnPad int

	insns []*Insn
}

var arches []*Arch

// Arches returns the families that have entries.
func Arches() []*Arch { return arches }

// ArchOf returns the family named name, or nil.
func ArchOf(name string) *Arch {
	for _, a := range arches {
		if a.Name == name {
			return a
		}
	}
	return nil
}

func (a *Arch) size() int {
	if a.Size == 0 {
		return 4
	}
	return a.Size
}

// Hex writes w the way the family's test data does.
func (a *Arch) Hex(w uint64) string {
	n := a.size()
	if a.BigEndianTestdata {
		return fmt.Sprintf("%0*x", 2*n, w)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%02x", byte(w>>(8*i)))
	}
	return b.String()
}

// TestdataLine writes one reference case the way the family's test file
// does, including its habit of writing a zero offset as (R4), not 0(R4).
func (a *Arch) TestdataLine(in *Insn, c Case) string {
	insn := zeroOffset.ReplaceAllString(in.Syntax(c.Ops...), "$1(")
	if a.OperandPad != 0 {
		mnemonic, operands, _ := strings.Cut(insn, " ")
		return fmt.Sprintf("\t%s\t%-*s// %s", mnemonic, a.OperandPad, operands, a.Hex(c.Want))
	}
	if !a.TabbedTestdata {
		pad := 32
		if a.InsnPad != 0 {
			pad = a.InsnPad
		}
		return fmt.Sprintf("\t%-*s// %s", pad, insn, a.Hex(c.Want))
	}
	// Operands start at column 16, with tabs every 8 columns.
	mnemonic, operands, _ := strings.Cut(insn, " ")
	tabs := "\t"
	if len(mnemonic) < 8 {
		tabs = "\t\t"
	}
	return fmt.Sprintf("\t%s%s%s\t// %s", mnemonic, tabs, operands, a.Hex(c.Want))
}

var zeroOffset = regexp.MustCompile(`(^|[ ,])0\(`)

// ParseHex reads an encoding written by Hex.
func (a *Arch) ParseHex(s string) (uint64, error) {
	n := a.size()
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil || len(s) != 2*n {
		return 0, fmt.Errorf("gap: %q is not a %d-digit encoding", s, 2*n)
	}
	if a.BigEndianTestdata {
		return v, nil
	}
	var w uint64
	for i := 0; i < n; i++ {
		w = w<<8 | v>>(8*i)&0xff
	}
	return w, nil
}

// Insns returns the family's entries.
func (a *Arch) Insns() []*Insn { return a.insns }

func reg(arch, kind string, r, max int) uint64 {
	if r < 0 || r > max {
		panic(fmt.Sprintf("%s: %s register %d does not exist", arch, kind, r))
	}
	return uint64(r)
}
