package data

//go:generate go run gen.go

// addVec computes out = in + {10, 20, 30, 40} element-wise over four int32
// lanes. The constant addend is a DATA/GLOBL table emitted by go-asmgen
// (emit.File.Data) and loaded straight into an SSE register — demonstrating
// constant-table support. Implemented in data_amd64.s.
func addVec(in, out *[4]int32)
