package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden file with the current run() output")

func TestRunMatchesGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := buf.String()

	golden, err := os.ReadFile("matchlen.wat.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(golden) {
		if *update {
			if err := os.WriteFile("matchlen.wat.golden", []byte(got), 0o644); err != nil {
				t.Fatalf("update golden: %v", err)
			}
			t.Logf("golden refreshed")
			return
		}
		t.Fatalf("output diverges from golden — run with -update to refresh (first mismatched line + surrounding)\ngot:\n%s\ngolden:\n%s",
			previewDiff(got, string(golden)), previewDiff(string(golden), got))
	}
}

func TestRunErrorPropagates(t *testing.T) {
	err := run(errWriter{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want boom error, got %v", err)
	}
}

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) { return 0, errors.New("boom") }

func previewDiff(a, b string) string {
	linesA := strings.Split(a, "\n")
	linesB := strings.Split(b, "\n")
	for i := 0; i < len(linesA) && i < len(linesB); i++ {
		if linesA[i] != linesB[i] {
			return "line " + itoaHelper(i+1) + ": " + linesA[i]
		}
	}
	return "(no line-level diff; check trailing bytes)"
}

func itoaHelper(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// TestMainSuccessPath drives the happy path: runFunc returns nil, osExit is
// not invoked. Uses the runFunc/osExit seams declared in main.go.
func TestMainSuccessPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	exited := false
	runFunc = func(io.Writer) error { return nil }
	osExit = func(int) { exited = true }
	main()
	if exited {
		t.Fatal("osExit called on success path")
	}
}

// TestMainErrorPath drives the sad path: runFunc returns an error and main
// forwards the error to Stderr + calls osExit(1). The fake osExit panics
// so the tests catch the exit code via recover.
func TestMainErrorPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	code := -1
	runFunc = func(io.Writer) error { return errors.New("boom") }
	osExit = func(c int) { code = c; panic("halt") }
	defer func() {
		_ = recover()
		if code != 1 {
			t.Fatalf("osExit code = %d, want 1", code)
		}
	}()
	main()
	t.Fatal("main did not exit on error")
}
