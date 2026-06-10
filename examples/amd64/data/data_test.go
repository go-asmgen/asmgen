package data

import "testing"

func TestAddVec(t *testing.T) {
	in := [4]int32{1, 2, 3, 4}
	var out [4]int32
	addVec(&in, &out)
	if want := ([4]int32{11, 22, 33, 44}); out != want {
		t.Errorf("addVec = %v want %v", out, want)
	}
}
