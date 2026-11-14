package simd

import (
	"math/bits"
	"testing"
)

func TestYmmLogic(t *testing.T) {
	if !hasAVX2() {
		t.Skip("no AVX2")
	}
	a := [8]uint32{0, 1, 0xffffffff, 0x80000000, 0xdeadbeef, 0x12345678, 0x0f0f0f0f, 7}
	b := [8]uint32{0xffffffff, 3, 0xffffffff, 1, 0xcafebabe, 0x87654321, 0xf0f0f0f0, 0x55555555}
	var out [9][8]uint32
	ymmLogic(&a, &b, &out)
	for i := range a {
		want := [9]uint32{
			a[i] ^ b[i], a[i] & b[i], a[i] | b[i], ^a[i] & b[i], ^a[i],
			bits.RotateLeft32(a[i], 13), a[i] << 3, a[i] >> 7, b[1],
		}
		for k := range want {
			if out[k][i] != want[k] {
				t.Errorf("op %d lane %d: got %#x want %#x", k, i, out[k][i], want[k])
			}
		}
	}
}

func TestYmmTranspose(t *testing.T) {
	if !hasAVX2() {
		t.Skip("no AVX2")
	}
	var p, want [4][8]uint32
	for r := range p {
		for w := range p[r] {
			p[r][w] = uint32(r*100 + w)
		}
	}
	// Inside each 128-bit half h (words 4h..4h+3), row r word w moves to row
	// w word r.
	for h := 0; h < 2; h++ {
		for r := 0; r < 4; r++ {
			for w := 0; w < 4; w++ {
				want[w][4*h+r] = p[r][4*h+w]
			}
		}
	}
	got := p
	ymmTranspose(&got)
	if got != want {
		t.Fatalf("transpose:\n got %v\nwant %v", got, want)
	}
	ymmTranspose(&got)
	if got != p {
		t.Fatalf("transposing twice did not give the input back: %v", got)
	}
}
