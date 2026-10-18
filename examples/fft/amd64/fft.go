// Package fft runs kernels built by go-asmgen's fft/amd64 package: the
// pointwise complex products and a 16-point FFT of two radix-4 passes.
package fft

//go:generate go run gen.go

// cmulSSE2 computes a[i] *= b[i] for i < n (fft.CMulSSE2).
func cmulSSE2(a, b *complex128, n int)

// cmulAVX2 computes dst[i] = a[i]·b[i] for i < 4·quads (fft.CMulAVX2).
func cmulAVX2(dst, a, b *complex128, quads int)

// pass4 is a radix-4 Stockham pass for ido >= 2 and last4 its final pass
// (Kernels.Pass, Kernels.Last). k is the constants table.
func pass4(cc, ch, tw *complex128, k *float64, ido, l1 int)
func last4(cc, ch *complex128, k *float64, l1 int)

// hasAVX2 reports whether the CPU and the OS support AVX2, which cmulAVX2,
// pass4 and last4 need.
func hasAVX2() bool
