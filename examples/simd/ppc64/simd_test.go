package simd

import "testing"

func TestXor16(t *testing.T) {
	var a, b, out [16]byte
	for i := range a {
		a[i] = byte(i)
		b[i] = byte(0xA5 ^ i)
	}
	xor16(&a, &b, &out)
	for i := range out {
		want := a[i] ^ b[i]
		if out[i] != want {
			t.Errorf("xor16 lane %d = %#x want %#x", i, out[i], want)
		}
	}
}

func TestEq16(t *testing.T) {
	var a, b, out [16]byte
	for i := range a {
		a[i] = byte(i)
		b[i] = byte(i)
	}
	// Make a few lanes differ.
	b[3] ^= 0xFF
	b[10] ^= 0x01
	eq16(&a, &b, &out)
	for i := range out {
		var want byte
		if a[i] == b[i] {
			want = 0xFF
		}
		if out[i] != want {
			t.Errorf("eq16 lane %d = %#x want %#x", i, out[i], want)
		}
	}
}
