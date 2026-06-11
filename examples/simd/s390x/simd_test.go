package simd

import "testing"

// reference computes the expected XOR over 16 bytes in pure Go (native byte
// order), which must match the s390x big-endian vector kernel because the op is
// byte-element-wise.
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
			t.Errorf("xor16 byte %d = %#02x want %#02x", i, out[i], want)
		}
	}
}

func TestXor16AllOnesAllZeros(t *testing.T) {
	var a, b, out [16]byte
	for i := range a {
		a[i] = 0xFF
		b[i] = 0x0F
	}
	xor16(&a, &b, &out)
	for i := range out {
		if out[i] != 0xF0 {
			t.Errorf("xor16 byte %d = %#02x want 0xf0", i, out[i])
		}
	}
}

func TestEq16(t *testing.T) {
	var a, b, out [16]byte
	for i := range a {
		a[i] = byte(i)
		// Make even lanes equal, odd lanes differ — this also catches any lane
		// ordering mistake (the big-endian gotcha) because the pattern is
		// position-dependent.
		if i%2 == 0 {
			b[i] = byte(i)
		} else {
			b[i] = byte(i) ^ 0x01
		}
	}
	eq16(&a, &b, &out)
	for i := range out {
		var want byte
		if a[i] == b[i] {
			want = 0xFF
		}
		if out[i] != want {
			t.Errorf("eq16 byte %d = %#02x want %#02x (a=%#02x b=%#02x)", i, out[i], want, a[i], b[i])
		}
	}
}
