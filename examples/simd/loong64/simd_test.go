package simd

import "testing"

func TestAddI32x4(t *testing.T) {
	a := [4]int32{1, 2, 3, 4}
	b := [4]int32{10, 20, 30, 40}
	var out [4]int32
	addI32x4(&a, &b, &out)
	if want := ([4]int32{11, 22, 33, 44}); out != want {
		t.Errorf("addI32x4 = %v want %v", out, want)
	}
}

func TestAddI32x4Wrap(t *testing.T) {
	a := [4]int32{1 << 30, 1 << 30, -1, 0}
	b := [4]int32{1 << 30, 1 << 30, 1, 0}
	var out [4]int32
	addI32x4(&a, &b, &out)
	if want := ([4]int32{-1 << 31, -1 << 31, 0, 0}); out != want {
		t.Errorf("addI32x4 wrap = %v want %v", out, want)
	}
}

func TestAddI32x8(t *testing.T) {
	a := [8]int32{1, 2, 3, 4, 5, 6, 7, 8}
	b := [8]int32{10, 20, 30, 40, 50, 60, 70, 80}
	var out [8]int32
	addI32x8(&a, &b, &out)
	if want := ([8]int32{11, 22, 33, 44, 55, 66, 77, 88}); out != want {
		t.Errorf("addI32x8 = %v want %v", out, want)
	}
}
