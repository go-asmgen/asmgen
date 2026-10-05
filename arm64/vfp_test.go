package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// vfpGolden (vfp_golden_test.go) was produced by the system assembler (Apple
// clang's `as -arch arm64`, read back with `otool -t`) from the assembly in
// each comment, covering every register field at 0, 31 and in between. The
// test hands the builder's text to cmd/asm and requires the same words back:
// that checks the mnemonic, the operand order and the register numbers in one
// go, with the Go assembler doing the encoding.

func TestVectorFloatEncodings(t *testing.T) {
	b := NewFunc("k", Layout(nil, nil, nil, nil), 0)
	for _, g := range vfpGolden {
		r := g.regs
		switch g.op {
		case "fadd":
			b.VFADD2D(r[0], r[1], r[2])
		case "fsub":
			b.VFSUB2D(r[0], r[1], r[2])
		case "fmul":
			b.VFMUL2D(r[0], r[1], r[2])
		case "fmla":
			b.VFMLA2D(r[0], r[1], r[2])
		case "fmls":
			b.VFMLS2D(r[0], r[1], r[2])
		case "fneg":
			b.VFNEG2D(r[0], r[1])
		case "fadd4s":
			b.VFADD4S(r[0], r[1], r[2])
		case "fsub4s":
			b.VFSUB4S(r[0], r[1], r[2])
		case "fmul4s":
			b.VFMUL4S(r[0], r[1], r[2])
		case "fmla4s":
			b.VFMLA4S(r[0], r[1], r[2])
		case "fmls4s":
			b.VFMLS4S(r[0], r[1], r[2])
		case "fneg4s":
			b.VFNEG4S(r[0], r[1])
		}
	}
	src := b.Func().String()
	if strings.Contains(src, "WORD") {
		t.Fatalf("hand-encoded instruction in the output; cmd/asm must do the encoding:\n%s", src)
	}
	got := assembleArm64(t, src)
	if len(got) < len(vfpGolden) {
		t.Fatalf("cmd/asm produced %d words for %d instructions:\n%s", len(got), len(vfpGolden), src)
	}
	lines := strings.Split(strings.TrimSpace(src), "\n")[1:]
	for i, g := range vfpGolden {
		if got[i] != g.want {
			t.Errorf("%s %v: %q assembled to %#08x, the system assembler gives %#08x",
				g.op, g.regs, strings.TrimSpace(lines[i]), got[i], g.want)
		}
	}
}

// assembleArm64 runs the Go assembler on one TEXT block and returns the
// instruction words, in order, as objdump reports them.
func assembleArm64(t *testing.T, text string) []uint32 {
	t.Helper()
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("no go command to assemble with: %v", err)
	}
	dir := t.TempDir()
	s := filepath.Join(dir, "k.s")
	o := filepath.Join(dir, "k.o")
	if err := os.WriteFile(s, []byte("#include \"textflag.h\"\n"+text+"\tRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inc := filepath.Join(strings.TrimSpace(string(goroot)), "pkg", "include")
	run := func(args ...string) []byte {
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return out
	}
	run("tool", "asm", "-p", "k", "-I", inc, "-o", o, s)
	var words []uint32
	for _, line := range strings.Split(string(run("tool", "objdump", o)), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || !strings.HasPrefix(f[1], "0x") {
			continue
		}
		w, err := strconv.ParseUint(f[2], 16, 32)
		if err != nil {
			t.Fatalf("objdump line %q: %v", line, err)
		}
		words = append(words, uint32(w))
	}
	return words
}

func TestVectorFloatRefusesARegisterThatDoesNotExist(t *testing.T) {
	for _, f := range []func(*Builder){
		func(b *Builder) { b.VFADD2D(32, 0, 0) },
		func(b *Builder) { b.VFMUL2D(0, -1, 0) },
		func(b *Builder) { b.VFMLS2D(0, 0, 40) },
		func(b *Builder) { b.VFNEG2D(0, 32) },
		func(b *Builder) { b.VFADD4S(0, 0, 32) },
		func(b *Builder) { b.VFNEG4S(-1, 0) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("an out-of-range vector register was encoded instead of refused")
				}
			}()
			f(NewFunc("k", Layout(nil, nil, nil, nil), 0))
		}()
	}
}
