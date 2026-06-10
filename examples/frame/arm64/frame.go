package frame

//go:generate go run gen.go

// spillSum returns a + b, but computed via a 16-byte stack frame: it spills both
// arguments to stack locals, reloads them, and adds. The generated function is
// emitted WITHOUT the NOSPLIT flag (via NewFuncFlags) — so the assembler inserts
// the stack-growth preamble, as a non-leaf or large-frame function would need.
func spillSum(a, b int64) int64
