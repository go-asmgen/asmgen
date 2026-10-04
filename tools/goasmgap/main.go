// Command goasmgap works on the instructions go-asmgen needs and cmd/asm
// lacks (internal/gap): it writes the cmd/asm test lines an upstream patch
// adds, finds those instructions in a Go tree's test data under whatever name
// Go already gives them, and checks a toolchain's encodings against the
// independent references.
//
//	goasmgap testdata -arch ppc64      # lines for testdata/ppc64.s
//	goasmgap scan -goroot DIR          # Go's test data, searched by opcode
//	goasmgap verify -goroot DIR        # assemble every reference case
//
// verify prints one line per instruction and target:
//
//	SUPPORTED  every reference case assembles to the reference word
//	PARTIAL    some do; the others are refused (the line names them)
//	ABSENT     none assembles
//	WRONG      a case assembles to a different word
//
// It exits 1 on WRONG, and with -require, on anything but SUPPORTED: the
// check a patched toolchain must pass.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/go-asmgen/asmgen/internal/gap"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: goasmgap testdata|scan|verify [flags]")
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	arch := fs.String("arch", "", "architecture family (default: all)")
	goroot := fs.String("goroot", "", "Go tree to scan or toolchain to verify")
	require := fs.Bool("require", false, "verify: fail unless every case is supported")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	arches, err := selectArches(*arch)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	switch args[0] {
	case "testdata":
		for _, a := range arches {
			testdata(stdout, a)
		}
		return 0
	case "scan", "verify":
		if *goroot == "" {
			fmt.Fprintf(stderr, "%s: -goroot is required\n", args[0])
			return 2
		}
		if args[0] == "scan" {
			if err := scan(stdout, *goroot, arches); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		tc, cleanup, err := newToolchain(*goroot)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		defer cleanup()
		ok, err := verify(stdout, tc, arches, *require)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !ok {
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "unknown command %q\n", args[0])
	return 2
}

func selectArches(name string) ([]*gap.Arch, error) {
	if name == "" {
		return gap.Arches(), nil
	}
	if a := gap.ArchOf(name); a != nil {
		return []*gap.Arch{a}, nil
	}
	return nil, fmt.Errorf("no gap entries for architecture %q", name)
}

// testdata writes the lines a cmd/asm patch adds to the family's test file,
// in that file's layout.
func testdata(w io.Writer, a *gap.Arch) {
	fmt.Fprintf(w, "// %s: add to src/cmd/asm/internal/asm/testdata/%s, next to the\n", a.Name, a.Testdata)
	fmt.Fprintf(w, "// lines of the nearest instruction family (scan shows where Go has kin).\n")
	for _, in := range a.Insns() {
		for _, c := range in.Golden {
			fmt.Fprintln(w, a.TestdataLine(in, c))
		}
	}
}

var encodingComment = regexp.MustCompile(`//\s*([0-9a-f]{8})\s*$`)

// scan reads the test data of a Go tree and reports every line whose
// encoding is one of the registry's instructions: the spelling Go already
// gives it, when it has one under another name.
func scan(w io.Writer, goroot string, arches []*gap.Arch) error {
	dir := filepath.Join(goroot, "src", "cmd", "asm", "internal", "asm", "testdata")
	for _, a := range arches {
		files, err := filepath.Glob(filepath.Join(dir, a.Name+"*.s"))
		if err != nil || len(files) == 0 {
			return fmt.Errorf("scan: no %s test data under %s", a.Name, dir)
		}
		sort.Strings(files)
		hits := map[*gap.Insn][]string{}
		for _, f := range files {
			if err := scanFile(f, a, hits); err != nil {
				return err
			}
		}
		for _, in := range a.Insns() {
			if len(hits[in]) == 0 {
				fmt.Fprintf(w, "%-8s %-11s not in Go's test data\n", a.Name, in.ISA)
				continue
			}
			fmt.Fprintf(w, "%-8s %-11s Go spells it:\n", a.Name, in.ISA)
			for i, h := range hits[in] {
				if i == 3 {
					fmt.Fprintf(w, "\t... and %d more\n", len(hits[in])-3)
					break
				}
				fmt.Fprintf(w, "\t%s\n", h)
			}
		}
	}
	return nil
}

func scanFile(path string, a *gap.Arch, hits map[*gap.Insn][]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		m := encodingComment.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		word, _ := a.ParseHex(m[1]) // the pattern admits only valid hex
		for _, in := range a.Insns() {
			if word&in.Mask == in.Bits {
				insn := strings.Join(strings.Fields(line[:strings.Index(line, "//")]), " ")
				hits[in] = append(hits[in], fmt.Sprintf("%-32s %s:%d", insn, filepath.Base(path), n))
			}
		}
	}
	return sc.Err()
}

// toolchain runs one Go tree's assembler, built from that tree's source:
// the cmd/asm installed under pkg/tool may predate the source (a patch
// applied but not reinstalled, or one reverted), and verify must judge the
// source.
type toolchain struct {
	goroot string
	bin    string // asm and objdump, freshly built
}

func newToolchain(goroot string) (toolchain, func(), error) {
	bin, err := os.MkdirTemp("", "goasmgap-bin")
	if err != nil {
		return toolchain{}, nil, err
	}
	tc := toolchain{goroot: goroot, bin: bin}
	cleanup := func() { os.RemoveAll(bin) }
	for _, tool := range []string{"asm", "objdump"} {
		out, err := tc.run("", filepath.Join(tc.goroot, "bin", "go"), "build", "-o", filepath.Join(bin, tool+exeSuffix()), "cmd/"+tool)
		if err != nil {
			cleanup()
			return toolchain{}, nil, fmt.Errorf("verify: building cmd/%s from %s: %s", tool, goroot, firstLine(out, err))
		}
	}
	return tc, cleanup, nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func (tc toolchain) run(goarch, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "GOROOT="+tc.goroot, "GOTOOLCHAIN=local")
	if goarch != "" {
		cmd.Env = append(cmd.Env, "GOOS=linux", "GOARCH="+goarch)
	}
	return cmd.CombinedOutput()
}

// assemble returns the words of the instructions in lines, in order, or the
// assembler's complaint.
func (tc toolchain) assemble(goarch string, lines []string) ([]uint32, error) {
	dir, err := os.MkdirTemp("", "goasmgap")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	src := "#include \"textflag.h\"\nTEXT ·k(SB),NOSPLIT,$0\n"
	for _, l := range lines {
		src += "\t" + l + "\n"
	}
	src += "\tRET\n"
	s, o := filepath.Join(dir, "k.s"), filepath.Join(dir, "k.o")
	if err := os.WriteFile(s, []byte(src), 0o644); err != nil {
		return nil, err
	}
	inc := filepath.Join(tc.goroot, "pkg", "include")
	if out, err := tc.run(goarch, filepath.Join(tc.bin, "asm"+exeSuffix()), "-p", "k", "-I", inc, "-o", o, s); err != nil {
		return nil, fmt.Errorf("%s", firstLine(out, err))
	}
	out, err := tc.run(goarch, filepath.Join(tc.bin, "objdump"+exeSuffix()), o)
	if err != nil {
		return nil, fmt.Errorf("objdump: %s", firstLine(out, err))
	}
	return objdumpWords(string(out))
}

// objdumpWords reads the instruction words from go tool objdump, which
// prints each as a 32-bit value whatever the target's byte order.
func objdumpWords(out string) ([]uint32, error) {
	var words []uint32
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasPrefix(f[1], "0x") {
			continue
		}
		v, err := strconv.ParseUint(f[2], 16, 32)
		if err != nil || len(f[2]) != 8 {
			return nil, fmt.Errorf("objdump: unexpected line %q", line)
		}
		words = append(words, uint32(v))
	}
	return words, nil
}

// firstLine is the assembler's first complaint, without the temporary
// file's name in front of it.
func firstLine(out []byte, err error) string {
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return tempPrefix.ReplaceAllString(l, "")
		}
	}
	return err.Error()
}

var tempPrefix = regexp.MustCompile(`^\S*k\.s:\d+:\s*`)

// verify assembles every reference case with the toolchain and classifies
// each instruction per target.
func verify(w io.Writer, tc assembler, arches []*gap.Arch, require bool) (bool, error) {
	ok := true
	for _, a := range arches {
		for _, goarch := range a.GOARCH {
			// A toolchain that cannot assemble at all would refuse every
			// case and report every instruction ABSENT. Prove it works
			// first: an empty function is a RET.
			if words, err := tc.assemble(goarch, nil); err != nil || len(words) == 0 {
				return false, fmt.Errorf("verify: the toolchain does not assemble for %s: %v", goarch, err)
			}
			for _, in := range a.Insns() {
				status, detail, err := classify(tc, goarch, in)
				if err != nil {
					return false, err
				}
				fmt.Fprintf(w, "%-9s %-8s %-11s %-8s %s\n", status, a.Name, in.ISA, goarch, detail)
				if status == "WRONG" || require && status != "SUPPORTED" {
					ok = false
				}
			}
		}
	}
	return ok, nil
}

func classify(tc assembler, goarch string, in *gap.Insn) (status, detail string, err error) {
	// All cases in one file first; one per file only if that is refused, to
	// tell which.
	lines := make([]string, len(in.Golden))
	for i, c := range in.Golden {
		lines[i] = in.Syntax(c.Ops...)
	}
	if words, aerr := tc.assemble(goarch, lines); aerr == nil {
		if len(words) < len(lines) {
			return "", "", fmt.Errorf("verify: %d instructions produced %d words", len(lines), len(words))
		}
		for i, c := range in.Golden {
			if words[i] != c.Want {
				return "WRONG", fmt.Sprintf("%q assembled to %#08x, %s gives %#08x", lines[i], words[i], in.Source, c.Want), nil
			}
		}
		return "SUPPORTED", fmt.Sprintf("%d/%d cases identical", len(lines), len(lines)), nil
	}
	var refused []string
	var reason string
	good := 0
	for _, c := range in.Golden {
		line := in.Syntax(c.Ops...)
		words, aerr := tc.assemble(goarch, []string{line})
		if aerr != nil {
			refused = append(refused, line)
			reason = aerr.Error()
			continue
		}
		if len(words) < 1 {
			return "", "", fmt.Errorf("verify: %q produced no instruction", line)
		}
		if words[0] != c.Want {
			return "WRONG", fmt.Sprintf("%q assembled to %#08x, %s gives %#08x", line, words[0], in.Source, c.Want), nil
		}
		good++
	}
	switch {
	case len(refused) == 0:
		return "SUPPORTED", fmt.Sprintf("%d/%d cases identical", good, len(in.Golden)), nil
	case good == 0:
		return "ABSENT", reason, nil
	}
	return "PARTIAL", fmt.Sprintf("%d/%d; refused: %s (%s)", good, len(in.Golden), strings.Join(refused, "; "), reason), nil
}

// assembler is what classify needs from a toolchain; tests substitute one.
type assembler interface {
	assemble(goarch string, lines []string) ([]uint32, error)
}
