package simd

import (
	"math/bits"
	"testing"
)

func TestZmmLogic(t *testing.T) {
	if !hasAVX512F() {
		t.Skip("no AVX-512F")
	}
	var a [16]uint32
	var b [32]uint32
	for i := range a {
		a[i] = uint32(i)*0x9e3779b1 ^ 0xdeadbeef
	}
	for i := range b {
		b[i] = uint32(i)*0x85ebca6b + 0x1234567
	}
	var out [7][16]uint32
	zmmLogic(&a, &b, &out)
	for i := range a {
		x, y := b[i], b[16+i]
		want := [7]uint32{
			a[i] ^ x ^ y, (a[i] & x) | (^a[i] & y), bits.RotateLeft32(a[i], 13),
			a[i] ^ b[1], (a[i] << 3) >> 1, a[i] + x, b[2],
		}
		for k := range want {
			if out[k][i] != want[k] {
				t.Errorf("op %d lane %d: got %#x want %#x", k, i, out[k][i], want[k])
			}
		}
	}
}

func TestZmmTranspose(t *testing.T) {
	if !hasAVX512F() {
		t.Skip("no AVX-512F")
	}
	var p, want [4][16]uint32
	for r := range p {
		for w := range p[r] {
			p[r][w] = uint32(r*100 + w)
		}
	}
	for h := 0; h < 4; h++ {
		for r := 0; r < 4; r++ {
			for w := 0; w < 4; w++ {
				want[w][4*h+r] = p[r][4*h+w]
			}
		}
	}
	got := p
	zmmTranspose(&got)
	if got != want {
		t.Fatalf("transpose:\n got %v\nwant %v", got, want)
	}
	zmmTranspose(&got)
	if got != p {
		t.Fatal("transposing twice did not give the input back")
	}
}
