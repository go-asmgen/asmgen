// Package fft runs kernels built by go-asmgen's fft/arm64 package: the
// pointwise complex product and a 16-point FFT of two radix-4 passes.
package fft

//go:generate go run gen.go

// cmulNEON computes a[i] *= b[i] for i < n (fft.CMulNEON).
func cmulNEON(a, b *complex128, n int)

// pass4 is the forward radix-4 Stockham pass for ido >= 2 and last4 its
// final pass (fft.Pass, fft.Last). tw holds the twiddles split, pair by pair
// (see TestFFT16).
func pass4(cc, ch *complex128, tw *float64, ido, l1 int)
func last4(cc, ch *complex128, l1 int)
