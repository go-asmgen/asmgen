package amd64

import "testing"

func TestZRegString(t *testing.T) {
	if Z0.String() != "Z0" || Z31.String() != "Z31" {
		t.Fatalf("got %s %s", Z0, Z31)
	}
	mustPanic(t, "invalid ZMM", func() { _ = ZReg(32).String() })
}

func TestZmmOperandOrder(t *testing.T) {
	b := ymmBuilder()
	b.VPXORD(Z0, Z1, Z2).VPXORDBcst(Z3, Z4, "16(DX)").VPADDD512(Z31, Z30, Z29).
		VPTERNLOGD(0x96, Z5, Z6, Z7).VPTERNLOGD(0xca, Z8, Z9, Z10).
		VPSLLD512(Z1, Z2, 3).VPSRLD512(Z1, Z2, 31).VPROLD(Z11, Z12, 13).
		VMOVDQA32(Z13, Z14).VMOVDQU32(Z15, "64(SI)").VMOVDQU32Store("(DI)", Z16).
		VPBROADCASTD512(Z17, "4(DX)")
	checkInsns(t, b, []string{
		"VPXORD Z2, Z1, Z0",
		"VPXORD.BCST 16(DX), Z4, Z3",
		"VPADDD Z29, Z30, Z31",
		"VPTERNLOGD $0x96, Z7, Z6, Z5",
		"VPTERNLOGD $0xca, Z10, Z9, Z8",
		"VPSLLD $3, Z2, Z1",
		"VPSRLD $31, Z2, Z1",
		"VPROLD $13, Z12, Z11",
		"VMOVDQA32 Z14, Z13",
		"VMOVDQU32 64(SI), Z15",
		"VMOVDQU32 Z16, (DI)",
		"VPBROADCASTD 4(DX), Z17",
	})
}

func TestZmmRefusalsAndTranspose(t *testing.T) {
	b := ymmBuilder()
	mustPanic(t, "out of 0..31", func() { b.VPSLLD512(Z0, Z0, 32) })
	mustPanic(t, "out of 0..31", func() { b.VPSRLD512(Z0, Z0, -1) })
	mustPanic(t, "out of 1..31", func() { b.VPROLD(Z0, Z0, 0) })
	mustPanic(t, "out of 1..31", func() { b.VPROLD(Z0, Z0, 32) })
	mustPanic(t, "distinct", func() { b.Transpose4x32Z(Z0, Z1, Z2, Z3, Z4, Z0) })
	mustPanic(t, "invalid ZMM", func() { b.VPXORD(ZReg(32), Z0, Z0) })

	b = ymmBuilder()
	b.VPUNPCKLDQ512(Z0, Z1, Z2).VPUNPCKHDQ512(Z0, Z1, Z2).VPUNPCKLQDQ512(Z0, Z1, Z2).VPUNPCKHQDQ512(Z0, Z1, Z2)
	checkInsns(t, b, []string{
		"VPUNPCKLDQ Z2, Z1, Z0",
		"VPUNPCKHDQ Z2, Z1, Z0",
		"VPUNPCKLQDQ Z2, Z1, Z0",
		"VPUNPCKHQDQ Z2, Z1, Z0",
	})
	b = ymmBuilder()
	b.Transpose4x32Z(Z0, Z1, Z2, Z3, Z4, Z5)
	checkInsns(t, b, []string{
		"VPUNPCKLDQ Z1, Z0, Z4",
		"VPUNPCKHDQ Z1, Z0, Z5",
		"VPUNPCKLDQ Z3, Z2, Z0",
		"VPUNPCKHDQ Z3, Z2, Z1",
		"VPUNPCKLQDQ Z1, Z5, Z2",
		"VPUNPCKHQDQ Z1, Z5, Z3",
		"VPUNPCKHQDQ Z0, Z4, Z5",
		"VPUNPCKLQDQ Z0, Z4, Z4",
		"VMOVDQA32 Z4, Z0",
		"VMOVDQA32 Z5, Z1",
	})
}
