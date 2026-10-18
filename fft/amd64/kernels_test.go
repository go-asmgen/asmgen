package amd64

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-asmgen/asmgen/emit"
)

var update = flag.Bool("update", false, "rewrite testdata from the builders")

// golden are the representative kernels whose text is checked in under
// testdata: one or two of every builder, default and FMA, the smallest
// radices that reach each of its paths.
var golden = map[string]func(f *emit.File){
	"cmul_sse2":     func(f *emit.File) { CMulSSE2(f, "cmul") },
	"radix2_sse2":   func(f *emit.File) { Radix2SSE2(f, "radix2") },
	"radix4_sse2":   func(f *emit.File) { Radix4SSE2(f, "radix4Fwd", false); Radix4SSE2(f, "radix4Inv", true) },
	"cmul_avx2":     func(f *emit.File) { CMulAVX2(f, "cmul") },
	"pass2":         func(f *emit.File) { goFFT.Pass(f, "pass2", 2) },
	"pass8_fma":     func(f *emit.File) { goFFTFMA.Pass(f, "pass8", 8) },
	"pass9":         func(f *emit.File) { goFFT.Pass(f, "pass9", 9) },
	"pass10_fma":    func(f *emit.File) { goFFTFMA.Pass(f, "pass10", 10) },
	"pass12":        func(f *emit.File) { goFFT.Pass(f, "pass12", 12) },
	"pass16_fma":    func(f *emit.File) { goFFTFMA.Pass(f, "pass16", 16) },
	"last3":         func(f *emit.File) { goFFT.Last(f, "last3", 3) },
	"last9_fma":     func(f *emit.File) { goFFTFMA.Last(f, "last9", 9) },
	"pass512_4":     func(f *emit.File) { goFFT.Pass512(f, "pass4", 4) },
	"last512_8_fma": func(f *emit.File) { goFFTFMA.Last512(f, "last8", 8) },
	"lastrun2":      func(f *emit.File) { goFFT.LastRun(f, "lastRun2", 2) },
	"lastrun512_4":  func(f *emit.File) { goFFT.LastRun512(f, "lastRun4", 4) },
	"strided4":      func(f *emit.File) { goFFT.Strided(f, "strided4", 4) },
	"strided512_4":  func(f *emit.File) { goFFTFMA.Strided512(f, "strided4", 4) },
	"batch2_fma":    func(f *emit.File) { goFFTFMA.Batch(f, "batch2", 2) },
	"pass32_3":      func(f *emit.File) { goFFT.Pass32(f, "pass3", 3) },
	"last32_2":      func(f *emit.File) { goFFT.Last32(f, "last2", 2) },
	"batch32_2":     func(f *emit.File) { goFFT.Batch32(f, "batch2", 2) },
	"untangle32":    func(f *emit.File) { goFFT.Untangle32(f, "untangle"); goFFT.Retangle32(f, "retangle") },
	"split4_is":     func(f *emit.File) { goFFT.Split(f, "split4", 4, false, true, false) },
	"split3_si_fma": func(f *emit.File) { goFFTFMA.Split(f, "split3", 3, true, false, true) },
	"split512_4_ii": func(f *emit.File) { goFFTFMA.Split512(f, "split4", 4, false, false, true) },
	"untangle":      func(f *emit.File) { goFFT.Untangle(f, "untangle", false); goFFT.Untangle(f, "retangle", true) },
	"untangle512":   func(f *emit.File) { goFFT.Untangle512(f, "untangle512", "untangle", false) },
	"frames":        func(f *emit.File) { TwoPass(f, "two", 64); ThreePass(f, "three", 64) },
	"addrtable":     func(f *emit.File) { AddrTable(f, "addrs", []Addr{{"pass2", 2}, {"last2", 23}}) },
}

func TestGolden(t *testing.T) {
	for name, build := range golden {
		f := emit.NewFile("amd64")
		build(f)
		got := f.String()
		path := filepath.Join("testdata", name+".s")
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run go test -update to write it)", name, err)
		}
		// A checkout may have turned the line ends into CRLF (Windows).
		if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
			t.Errorf("%s: the builder's text differs from %s", name, path)
		}
	}
}

// TestGoFFTDigests holds every file go-fft generates with this package to
// the bytes go-fft commits: testdata/gofft.sha256 is `sha256sum` of its
// internal/kernels/*.s (the FMA files and the FMA-less cpu_amd64.s and
// zntwopass_amd64.s as each was committed).
func TestGoFFTDigests(t *testing.T) {
	files := goFFTFiles()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%x  %s\n", sha256.Sum256([]byte(files[n])), n)
	}
	path := filepath.Join("testdata", "gofft.sha256")
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.String() != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("go-fft's kernels changed:\ngot\n%swant\n%s", b.String(), want)
	}
}

// TestAssemble hands every file of goFFTFiles and every golden file to the Go
// assembler for amd64, which checks every mnemonic, operand and register.
func TestAssemble(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the Go assembler")
	}
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("no go command to assemble with: %v", err)
	}
	inc := filepath.Join(strings.TrimSpace(string(goroot)), "pkg", "include")
	dir := t.TempDir()
	srcs := goFFTFiles()
	for name, build := range golden {
		f := emit.NewFile("amd64")
		build(f)
		srcs["golden_"+name+".s"] = Funcdata(f.String())
	}
	for name, src := range srcs {
		s := filepath.Join(dir, name)
		if err := os.WriteFile(s, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("go", "tool", "asm", "-p", "k", "-I", inc, "-o", s+".o", s)
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s does not assemble: %v\n%s", name, err, out)
		}
	}
}

func TestRadixRefused(t *testing.T) {
	k := goFFT
	for _, f := range []func(*emit.File){
		func(f *emit.File) { k.Pass(f, "x", 7) },
		func(f *emit.File) { k.Last(f, "x", 6) },
		func(f *emit.File) { k.Pass512(f, "x", 3) },
		func(f *emit.File) { k.Last512(f, "x", 16) },
		func(f *emit.File) { k.LastRun(f, "x", 5) },
		func(f *emit.File) { k.LastRun512(f, "x", 3) },
		func(f *emit.File) { k.Strided(f, "x", 2) },
		func(f *emit.File) { k.Strided512(f, "x", 16) },
		func(f *emit.File) { k.Batch(f, "x", 16) },
		func(f *emit.File) { k.Pass32(f, "x", 16) },
		func(f *emit.File) { k.Last32(f, "x", 9) },
		func(f *emit.File) { k.Batch32(f, "x", 12) },
		func(f *emit.File) { k.Split(f, "x", 2, false, true, false) },
		func(f *emit.File) { k.Split512(f, "x", 3, false, true, false) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("an unsupported radix was built instead of refused")
				}
			}()
			f(emit.NewFile("amd64"))
		}()
	}
}

func TestFuncdata(t *testing.T) {
	got := Funcdata(emit.NewFile("amd64").String())
	if !strings.Contains(got, "#include \"textflag.h\"\n#include \"funcdata.h\"\n") {
		t.Errorf("funcdata.h not included:\n%s", got)
	}
}

func TestFPRefusesAnUnknownArgument(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an argument that does not exist was addressed")
		}
	}()
	fp([]string{"a"}, "b")
}
