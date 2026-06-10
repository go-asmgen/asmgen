package types

import (
	"math"
	"testing"
)

func TestAddInt32(t *testing.T) {
	if got := addInt32(2, 3); got != 5 {
		t.Errorf("addInt32(2,3)=%d want 5", got)
	}
	// Wraps modulo 2^32 like Go's int32 addition.
	if got := addInt32(math.MaxInt32, 1); got != math.MinInt32 {
		t.Errorf("addInt32(MaxInt32,1)=%d want %d", got, int32(math.MinInt32))
	}
}

func TestAddUint32(t *testing.T) {
	if got := addUint32(4_000_000_000, 500_000_000); got != 205032704 {
		t.Errorf("addUint32 wrap=%d want 205032704", got) // (4.5e9) mod 2^32
	}
}

func TestAddInt16(t *testing.T) {
	if got := addInt16(100, 200); got != 300 {
		t.Errorf("addInt16(100,200)=%d want 300", got)
	}
	if got := addInt16(30000, 30000); got != -5536 { // 60000 mod 2^16 as int16
		t.Errorf("addInt16(30000,30000)=%d want -5536", got)
	}
}

func TestAddInt8(t *testing.T) {
	if got := addInt8(-40, 50); got != 10 {
		t.Errorf("addInt8(-40,50)=%d want 10", got)
	}
	if got := addInt8(100, 100); got != -56 { // 200 mod 2^8 as int8
		t.Errorf("addInt8(100,100)=%d want -56", got)
	}
}

func TestWidenInt8(t *testing.T) {
	// The MOVB load must sign-extend.
	if got := widenInt8(-1); got != -1 {
		t.Errorf("widenInt8(-1)=%d want -1", got)
	}
	if got := widenInt8(127); got != 127 {
		t.Errorf("widenInt8(127)=%d want 127", got)
	}
}

func TestWidenUint8(t *testing.T) {
	// The MOVBU load must zero-extend: 0xFF stays 255, not -1.
	if got := widenUint8(0xFF); got != 255 {
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
