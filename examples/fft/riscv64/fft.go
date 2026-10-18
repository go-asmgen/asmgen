// Package fft runs the kernel built by go-asmgen's fft/riscv64 package: the
// pointwise complex product.
package fft

//go:generate go run gen.go

// cmulRVV computes a[i] *= b[i] for i < n (fft.CMulRVV).
func cmulRVV(a, b *complex128, n int)
