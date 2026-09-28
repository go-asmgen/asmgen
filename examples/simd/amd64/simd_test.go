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
	if !hasAVX2() {
		t.Skip("no AVX2 on this CPU: addI32x8 would be an illegal instruction")
	}
	a := [8]int32{1, 2, 3, 4, 5, 6, 7, 8}
	b := [8]int32{10, 20, 30, 40, 50, 60, 70, 80}
	var out [8]int32
	addI32x8(&a, &b, &out)
	if want := ([8]int32{11, 22, 33, 44, 55, 66, 77, 88}); out != want {
		t.Errorf("addI32x8 = %v want %v", out, want)
	}
}

// TestTheGateAgreesWithItself: a probe is only useful if calling the kernel it
// guards is safe whenever it says yes. There is no way to prove the negative
// here -- this machine either has AVX2 or it does not -- so what is checked is
// that the answer is stable and that the guarded call survives it.
func TestTheGateAgreesWithItself(t *testing.T) {
	first := hasAVX2()
	for range 100 {
		if hasAVX2() != first {
			t.Fatal("the probe gave two different answers on one CPU")
		}
	}
	t.Logf("hasAVX2() = %v on this runner", first)
	if first {
		var a, b, out [8]int32
		addI32x8(&a, &b, &out) // must not fault
	}
}
