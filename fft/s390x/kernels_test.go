package s390x

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

func TestGolden(t *testing.T) {
	for name, build := range golden {
		f := emit.NewFile("s390x")
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
		if got != string(want) {
			t.Errorf("%s: the builder's text differs from %s", name, path)
		}
	}
}

// TestGoFFTDigests holds every file go-fft generates with this package to
// the bytes go-fft commits: testdata/gofft.sha256 is `sha256sum` of those
// files in its internal/kernels.
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
	if b.String() != string(want) {
		t.Errorf("go-fft's kernels changed:\ngot\n%swant\n%s", b.String(), want)
	}
}

// TestAssemble hands every file of goFFTFiles and every golden file to the Go
// assembler for s390x, which checks every mnemonic, operand and register.
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
		f := emit.NewFile("s390x")
		build(f)
		srcs["golden_"+name+".s"] = f.String()
	}
	for name, src := range srcs {
		s := filepath.Join(dir, name)
		if err := os.WriteFile(s, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("go", "tool", "asm", "-p", "k", "-I", inc, "-o", s+".o", s)
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=s390x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s does not assemble: %v\n%s", name, err, out)
		}
	}
}
