package types

import (
	"math"
	"testing"
)

func TestAddInt64(t *testing.T) {
	if got := addInt64(1<<40, 1<<40); got != 1<<41 {
		t.Errorf("addInt64(2^40,2^40)=%d want 2^41", got)
	}
	if got := addInt64(-5, 5); got != 0 {
		t.Errorf("addInt64(-5,5)=%d want 0", got)
	}
}

func TestAddInt32(t *testing.T) {
	if got := addInt32(2, 3); got != 5 {
		t.Errorf("addInt32(2,3)=%d want 5", got)
	}
	if got := addInt32(math.MaxInt32, 1); got != math.MinInt32 {
		t.Errorf("addInt32(MaxInt32,1)=%d want %d", got, int32(math.MinInt32))
	}
}

func TestWidenInt8(t *testing.T) {
	if got := widenInt8(-1); got != -1 { // MOVBQSX must sign-extend
		t.Errorf("widenInt8(-1)=%d want -1", got)
	}
	if got := widenInt8(127); got != 127 {
		t.Errorf("widenInt8(127)=%d want 127", got)
	}
}

func TestWidenUint8(t *testing.T) {
	if got := widenUint8(0xFF); got != 255 { // MOVBQZX must zero-extend
		t.Errorf("widenUint8(0xFF)=%d want 255", got)
	}
}

func TestAddFloat64(t *testing.T) {
	if got := addFloat64(1.5, 2.25); got != 3.75 {
		t.Errorf("addFloat64(1.5,2.25)=%v want 3.75", got)
	}
}

func TestAddFloat32(t *testing.T) {
	if got := addFloat32(1.5, 2.25); got != 3.75 {
		t.Errorf("addFloat32(1.5,2.25)=%v want 3.75", got)
	}
}
