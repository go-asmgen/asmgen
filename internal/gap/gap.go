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
	// Mask and Bits fix the opcode: a word w is this instruction, whatever
	// its operands, iff w&Mask == Bits. This finds the instruction in Go's
	// test data under any spelling.
	Mask, Bits uint32
	// Encode returns the machine word for operands in ISA order. It panics
	// on an operand the instruction cannot encode.
	Encode func(ops ...int) uint32
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
	Want uint32
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
func (in *Insn) Word(ops ...int) string {
	return fmt.Sprintf("WORD $0x%08x // %s", in.Encode(ops...), in.Syntax(ops...))
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
	// BigEndianTestdata says how that file writes an encoding: as the word
	// in hex (ppc64, whose tests assemble big-endian) or as its bytes in
	// little-endian order (loong64).
	BigEndianTestdata bool

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

// Hex writes w the way the family's test data does.
func (a *Arch) Hex(w uint32) string {
	if a.BigEndianTestdata {
		return fmt.Sprintf("%08x", w)
	}
	return fmt.Sprintf("%02x%02x%02x%02x", byte(w), byte(w>>8), byte(w>>16), byte(w>>24))
}

// ParseHex reads an encoding written by Hex.
func (a *Arch) ParseHex(s string) (uint32, error) {
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil || len(s) != 8 {
		return 0, fmt.Errorf("gap: %q is not an 8-digit encoding", s)
	}
	w := uint32(v)
	if !a.BigEndianTestdata {
		w = w>>24 | w>>8&0xff00 | w<<8&0xff0000 | w<<24
	}
	return w, nil
}

// Insns returns the family's entries.
func (a *Arch) Insns() []*Insn { return a.insns }

func reg(arch, kind string, r, max int) uint32 {
	if r < 0 || r > max {
		panic(fmt.Sprintf("%s: %s register %d does not exist", arch, kind, r))
	}
	return uint32(r)
}
