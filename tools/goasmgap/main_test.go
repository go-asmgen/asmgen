package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/internal/gap"
)

// fake assembles from a table: a line it does not know is refused.
type fake map[string]uint32

func (f fake) assemble(_ string, lines []string) ([]uint32, error) {
	var words []uint32
	for _, l := range lines {
		w, ok := f[l]
		if !ok {
			return nil, errors.New("unrecognized instruction")
		}
		words = append(words, w)
	}
	return words, nil
}

func knowing(in *gap.Insn, skip int) fake {
	f := fake{}
	for i, c := range in.Golden {
		if i != skip {
			f[in.Syntax(c.Ops...)] = c.Want
		}
	}
	return f
}

func TestClassify(t *testing.T) {
	in := gap.Lookup("ppc64", "xvadddp")
	wrong := knowing(in, -1)
	wrong[in.Syntax(in.Golden[2].Ops...)] ^= 1
	short := short{}
	for _, tc := range []struct {
		name string
		asm  assembler
		want string
	}{
		{"all", knowing(in, -1), "SUPPORTED"},
		{"one refused", knowing(in, 4), "PARTIAL"},
		{"none", fake{}, "ABSENT"},
		{"one bit off", wrong, "WRONG"},
	} {
		got, detail, err := classify(tc.asm, "ppc64le", in)
		if err != nil || got != tc.want {
			t.Errorf("%s: %s (%s), %v; want %s", tc.name, got, detail, err, tc.want)
		}
	}
	if _, _, err := classify(short, "ppc64le", in); err == nil {
		t.Error("an assembler that returned too few words went unnoticed")
	}
}

// A WRONG on a case only reached one by one (the batch was refused) is still
// WRONG, and a case producing no word is an error.
func TestClassifyCaseByCase(t *testing.T) {
	in := gap.Lookup("ppc64", "xvadddp")
	f := knowing(in, 0)
	f[in.Syntax(in.Golden[1].Ops...)] ^= 1
	if got, _, _ := classify(f, "ppc64", in); got != "WRONG" {
		t.Errorf("got %s", got)
	}
	if _, _, err := classify(empty{}, "ppc64", in); err == nil {
		t.Error("no words went unnoticed")
	}
}

type short struct{}

func (short) assemble(string, []string) ([]uint32, error) { return []uint32{1}, nil }

// fake and the others return no word for an empty function; verify's
// preflight needs the RET a real assembler would give.
type withRET struct{ assembler }

func (r withRET) assemble(goarch string, lines []string) ([]uint32, error) {
	if len(lines) == 0 {
		return []uint32{0x4e800020}, nil
	}
	return r.assembler.assemble(goarch, lines)
}

func TestVerify(t *testing.T) {
	ppc := []*gap.Arch{gap.ArchOf("ppc64")}
	all := fake{}
	for _, in := range gap.ArchOf("ppc64").Insns() {
		for k, v := range knowing(in, -1) {
			all[k] = v
		}
	}
	var b bytes.Buffer
	if ok, err := verify(&b, withRET{all}, ppc, true); !ok || err != nil {
		t.Errorf("all supported, -require: %v %v\n%s", ok, err, b.String())
	}
	if ok, err := verify(&b, withRET{fake{}}, ppc, false); !ok || err != nil {
		t.Errorf("all absent without -require must pass: %v %v", ok, err)
	}
	if ok, _ := verify(&b, withRET{fake{}}, ppc, true); ok {
		t.Error("all absent passed -require")
	}
	in := gap.Lookup("ppc64", "xvmuldp")
	all[in.Syntax(in.Golden[0].Ops...)] ^= 1
	if ok, _ := verify(&b, withRET{all}, ppc, false); ok {
		t.Error("a WRONG encoding passed")
	}
	// The trap this guards: a toolchain that cannot assemble anything.
	if _, err := verify(&b, fake{}, ppc, false); err == nil {
		t.Error("a toolchain that assembles nothing reported a verdict")
	}
	if _, err := verify(&b, withRET{empty{}}, ppc, false); err == nil {
		t.Error("a case producing no word went unnoticed")
	}
}

type empty struct{}

func (empty) assemble(_ string, lines []string) ([]uint32, error) {
	if len(lines) > 1 {
		return nil, errors.New("refused")
	}
	return nil, nil
}

func TestObjdumpWords(t *testing.T) {
	out := "TEXT k(SB) k.s\n  k.s:3\t\t0x0\t\tf0000300\t\tXVADDDP VS0,VS0,VS0\n  k.s:4\t\t0x4\t\t4e800020\t\tRET\n"
	w, err := objdumpWords(out)
	if err != nil || len(w) != 2 || w[0] != 0xf0000300 {
		t.Errorf("%#x, %v", w, err)
	}
	if _, err := objdumpWords("  k.s:3 0x0 xyz RET\n"); err == nil {
		t.Error("a malformed word was accepted")
	}
}

func TestTestdataLayout(t *testing.T) {
	var b bytes.Buffer
	testdata(&b, gap.ArchOf("ppc64"))
	if !strings.Contains(b.String(), "\tXVADDDP VS2, VS1, VS3           // f0620b00\n") {
		t.Errorf("ppc64 layout:\n%.300s", b.String())
	}
	b.Reset()
	testdata(&b, gap.ArchOf("loong64"))
	if !strings.Contains(b.String(), "\tVFMADDD\t\tV4, V1, V2, V3\t// 43042209\n") {
		t.Errorf("loong64 layout:\n%.300s", b.String())
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "src", "cmd", "asm", "internal", "asm", "testdata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := "\tVMOVQ\t(R4), V3.V2\t// 83001030\n\tVADDD V1, V2, V3 // 00000000\n// comment only\n"
	for i := 0; i < 5; i++ {
		lines += "\tXVMOVQ\t(R4), X3.V4\t// 83001032\n"
	}
	os.WriteFile(filepath.Join(dir, "loong64enc1.s"), []byte(lines), 0o644)
	var b bytes.Buffer
	if err := scan(&b, root, []*gap.Arch{gap.ArchOf("loong64")}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"vfmadd.d    not in Go's test data",
		"vldrepl.d   Go spells it:\n\tVMOVQ (R4), V3.V2",
		"loong64enc1.s:1",
		"... and 2 more",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if err := scan(&b, root, []*gap.Arch{gap.ArchOf("ppc64")}); err == nil {
		t.Error("a tree without ppc64 test data was scanned")
	}
	// Windows ignores the permission bits and root reads anything.
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		return
	}
	os.Chmod(filepath.Join(dir, "loong64enc1.s"), 0)
	if err := scan(&b, root, []*gap.Arch{gap.ArchOf("loong64")}); err == nil {
		t.Error("an unreadable file went unnoticed")
	}
}

func TestRunArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{nil, 2},
		{[]string{"frobnicate"}, 2},
		{[]string{"testdata", "-bogus"}, 2},
		{[]string{"testdata", "-arch", "mips"}, 2},
		{[]string{"scan"}, 2},
		{[]string{"verify"}, 2},
		{[]string{"testdata", "-arch", "loong64"}, 0},
		{[]string{"testdata"}, 0},
		{[]string{"scan", "-goroot", "/nonexistent"}, 1},
		{[]string{"verify", "-goroot", "/nonexistent"}, 1},
	} {
		var out, errb bytes.Buffer
		if got := run(tc.args, &out, &errb); got != tc.code {
			t.Errorf("%v: exit %d, want %d (%s)", tc.args, got, tc.code, errb.String())
		}
	}
}

// The real assembler, anchored on a known positive: Go has assembled the
// loong64 broadcast load since 1.26, so it must not come back ABSENT, and
// its cases that do assemble must match the references.
func TestVerifyWithTheRealToolchain(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the Go assembler")
	}
	var b bytes.Buffer
	code := run([]string{"verify", "-goroot", runtime.GOROOT(), "-arch", "loong64"}, &b, &b)
	out := b.String()
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(l, "ldrepl") && !strings.HasPrefix(l, "SUPPORTED") && !strings.HasPrefix(l, "PARTIAL") {
			t.Errorf("known positive reported %q", l)
		}
	}
	if !strings.Contains(out, "vldrepl.d") {
		t.Errorf("no verdict for vldrepl.d:\n%s", out)
	}
	tc, cleanup, err := newToolchain(runtime.GOROOT())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := tc.assemble("loong64", []string{"NOTANINSN V1"}); err == nil ||
		strings.Contains(err.Error(), "k.s:") {
		t.Errorf("refusal should be reported without the temporary file name: %v", err)
	}
}

// An assembler that refuses the batch but takes each case alone still yields
// a verdict from the cases.
type onlySingles struct{ fake }

func (o onlySingles) assemble(goarch string, lines []string) ([]uint32, error) {
	if len(lines) > 1 {
		return nil, errors.New("refused")
	}
	return o.fake.assemble(goarch, lines)
}

func TestClassifySinglesAllGood(t *testing.T) {
	in := gap.Lookup("loong64", "vfmadd.d")
	if got, _, _ := classify(onlySingles{knowing(in, -1)}, "loong64", in); got != "SUPPORTED" {
		t.Errorf("got %s", got)
	}
}

func TestRunScanAndRequire(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the Go assembler")
	}
	var b bytes.Buffer
	if code := run([]string{"scan", "-goroot", runtime.GOROOT(), "-arch", "loong64"}, &b, &b); code != 0 ||
		!strings.Contains(b.String(), "VMOVQ") {
		t.Errorf("scan of this Go tree: exit %d\n%s", code, b.String())
	}
	// -require holds this toolchain to every loong64 entry. Whatever the Go
	// version, each entry gets exactly one verdict per target, and the exit
	// status agrees with them: the version only decides which.
	b.Reset()
	code := run([]string{"verify", "-goroot", runtime.GOROOT(), "-arch", "loong64", "-require"}, &b, &b)
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != len(gap.ArchOf("loong64").Insns()) {
		t.Fatalf("%d verdicts for %d entries:\n%s", len(lines), len(gap.ArchOf("loong64").Insns()), b.String())
	}
	allSupported := true
	for _, l := range lines {
		allSupported = allSupported && strings.HasPrefix(l, "SUPPORTED")
	}
	if (code == 0) != allSupported {
		t.Errorf("exit %d does not match the verdicts:\n%s", code, b.String())
	}
}
