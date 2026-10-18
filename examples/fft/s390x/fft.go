// Package fft runs the kernel built by go-asmgen's fft/s390x package: the
// pointwise complex product.
package fft

//go:generate go run gen.go

// cmulVX computes a[i] *= b[i] for i < n (fft.CMulVX).
func cmulVX(a, b *complex128, n int)
