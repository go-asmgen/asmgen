package wasm

import (
	"strings"
	"testing"
)

// TestFunctionEmptyBody covers the exact shape of a function with no body ops.
// This is the baseline every op-adding test builds on.
func TestFunctionEmptyBody(t *testing.T) {
	fn := NewFunction("foo", "foo",
		[]Param{{Name: "x", Type: I32}},
		[]ValueType{I32},
	)
	got := fn.String()
	want := "  (func $foo (export \"foo\") (param $x i32) (result i32)\n  )\n"
	if got != want {
		t.Errorf("empty body: got %q want %q", got, want)
	}
}

// TestFunctionNotExported covers the branch where Export == "".
func TestFunctionNotExported(t *testing.T) {
	fn := NewFunction("helper", "", nil, nil)
	got := fn.String()
	if strings.Contains(got, "export") {
		t.Errorf("unexpected export in %q", got)
	}
	if !strings.HasPrefix(got, "  (func $helper\n") {
		t.Errorf("wrong prefix: %q", got)
	}
}

// TestLocalReturnsHandle covers Local's return value contract — the caller
// uses it as the WAT reference for subsequent LocalGet/Set.
func TestLocalReturnsHandle(t *testing.T) {
	fn := NewFunction("f", "f", nil, nil)
	name := fn.Local("counter", I32)
	if name != "$counter" {
		t.Errorf("Local returned %q, want $counter", name)
	}
	if len(fn.Locals) != 1 || fn.Locals[0].Name != "counter" || fn.Locals[0].Type != I32 {
		t.Errorf("Locals slice = %+v", fn.Locals)
	}
	out := fn.String()
	if !strings.Contains(out, "(local $counter i32)") {
		t.Errorf("missing local decl in %q", out)
	}
}

// TestRawFormatting covers Raw's fmt.Sprintf pass-through with args.
func TestRawFormatting(t *testing.T) {
	fn := NewFunction("f", "f", nil, nil)
	fn.Raw("(i32.const %d)", 42)
	if !strings.Contains(fn.String(), "    (i32.const 42)\n") {
		t.Errorf("Raw missed formatting: %q", fn.String())
	}
}

// TestOpWithAndWithoutArgs covers both Op branches.
func TestOpWithAndWithoutArgs(t *testing.T) {
	fn := NewFunction("f", "f", nil, nil)
	fn.Op("drop")
	fn.Op("i32.const", 7)
	out := fn.String()
	if !strings.Contains(out, "(drop)") || !strings.Contains(out, "(i32.const 7)") {
		t.Errorf("Op emit shape wrong: %q", out)
	}
}

// TestStackOpsShape checks each generated stack op has the exact string it
// promises — one representative per op family so a rename or typo is caught.
func TestStackOpsShape(t *testing.T) {
	fn := NewFunction("f", "f", nil, nil)
	fn.LocalGet("a")
	fn.LocalSet("b")
	fn.LocalTee("c")
	fn.I32Const(-3)
	fn.V128Load(16)
	fn.V128Store(32)
	fn.V128And()
	fn.V128Or()
	fn.V128Xor()
	fn.V128Zero()
	fn.I8x16Eq()
	fn.I8x16AllTrue()
	fn.I8x16Swizzle()
	fn.I8x16ShrU()
	fn.I8x16Popcnt()
	fn.I16x8ExtaddPairwiseI8x16U()
	fn.I32x4ExtaddPairwiseI16x8U()
	fn.I32x4Add()
	fn.I32Eqz()
	fn.I32Add()
	fn.I32Mul()
	fn.I32Load8U()
	fn.I32GtS()
	fn.I32GeS()
	fn.I32Ne()
	fn.I8x16GeS()
	fn.I8x16LeS()
	fn.I8x16Sub()
	fn.I8x16Splat()
	fn.I8x16Bitmask()
	fn.I32Ctz()
	fn.I32Sub()
	fn.I32Popcnt()
	fn.I8x16Shl()
	fn.V128Store64Lane(0, 0)
	fn.V128Store64Lane(16, 1)
	fn.I32Store(0)
	fn.I32Store(4)
	fn.I16x8ExtmulLowI8x16U()
	fn.I16x8ExtmulHighI8x16U()
	fn.I16x8Mul()
	fn.I32x4ExtmulLowI16x8U()
	fn.I32x4ExtmulHighI16x8U()
	fn.I32x4ShrU()
	fn.I16x8NarrowI32x4U()
	fn.I8x16GtU()
	fn.I8x16LtU()
	fn.I8x16Add()
	fn.Return()

	body := fn.String()
	wants := []string{
		"(local.get $a)",
		"(local.set $b)",
		"(local.tee $c)",
		"(i32.const -3)",
		"(v128.load offset=16)",
		"(v128.store offset=32)",
		"(v128.and)",
		"(v128.or)",
		"(v128.xor)",
		"(v128.const i32x4 0 0 0 0)",
		"(i8x16.eq)",
		"(i8x16.all_true)",
		"(i8x16.swizzle)",
		"(i8x16.shr_u)",
		"(i8x16.popcnt)",
		"(i16x8.extadd_pairwise_i8x16_u)",
		"(i32x4.extadd_pairwise_i16x8_u)",
		"(i32x4.add)",
		"(i32.eqz)",
		"(i32.add)",
		"(i32.mul)",
		"(i32.load8_u)",
		"(i32.gt_s)",
		"(i32.ge_s)",
		"(i32.ne)",
		"(i8x16.ge_s)",
		"(i8x16.le_s)",
		"(i8x16.sub)",
		"(i8x16.splat)",
		"(i8x16.bitmask)",
		"(i32.ctz)",
		"(i32.sub)",
		"(i32.popcnt)",
		"(i8x16.shl)",
		"(v128.store64_lane offset=0 0)",
		"(v128.store64_lane offset=16 1)",
		"(i32.store offset=0)",
		"(i32.store offset=4)",
		"(i16x8.extmul_low_i8x16_u)",
		"(i16x8.extmul_high_i8x16_u)",
		"(i16x8.mul)",
		"(i32x4.extmul_low_i16x8_u)",
		"(i32x4.extmul_high_i16x8_u)",
		"(i32x4.shr_u)",
		"(i16x8.narrow_i32x4_u)",
		"(i8x16.gt_u)",
		"(i8x16.lt_u)",
		"(i8x16.add)",
		"(return)",
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing op %q in body:\n%s", w, body)
		}
	}
}

// TestV128Const16 covers both the happy path (16 bytes) and the panic on
// wrong length.
func TestV128Const16(t *testing.T) {
	fn := NewFunction("f", "f", nil, nil)
	buf := make([]byte, 16)
	for i := range buf {
		buf[i] = byte(i - 8) // spans negative + positive i8 values
	}
	fn.V128Const16(buf)
	body := fn.String()
	// -8 signed byte range: -8..7 — check a few values landed:
	if !strings.Contains(body, "(v128.const i8x16 -8 -7 -6 -5 -4 -3 -2 -1 0 1 2 3 4 5 6 7)") {
		t.Errorf("v128.const shape wrong: %q", body)
	}
}

func TestV128Const16WrongLenPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on non-16 length")
		}
	}()
	fn := NewFunction("f", "f", nil, nil)
	fn.V128Const16([]byte{1, 2, 3})
}

// TestI8x16Shuffle covers valid + panic.
func TestI8x16Shuffle(t *testing.T) {
	fn := NewFunction("f", "f", nil, nil)
	idx := []byte{0, 16, 1, 17, 2, 18, 3, 19, 4, 20, 5, 21, 6, 22, 7, 23}
	fn.I8x16Shuffle(idx)
	if !strings.Contains(fn.String(), "(i8x16.shuffle 0 16 1 17 2 18 3 19 4 20 5 21 6 22 7 23)") {
		t.Errorf("shuffle shape wrong: %q", fn.String())
	}
}

func TestI8x16ShuffleWrongLenPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on non-16 immediates")
		}
	}()
	fn := NewFunction("f", "f", nil, nil)
	fn.I8x16Shuffle([]byte{0})
}

// TestI32x4ExtractLane covers valid lane 0..3 + panic on out-of-range.
func TestI32x4ExtractLane(t *testing.T) {
	fn := NewFunction("f", "f", nil, nil)
	for lane := 0; lane < 4; lane++ {
		fn.I32x4ExtractLane(lane)
	}
	body := fn.String()
	for lane := 0; lane < 4; lane++ {
		want := "(i32x4.extract_lane " + itoaByte(lane) + ")"
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %q", want, body)
		}
	}
}

func TestI32x4ExtractLaneOutOfRangePanicsHigh(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on lane > 3")
		}
	}()
	fn := NewFunction("f", "f", nil, nil)
	fn.I32x4ExtractLane(4)
}

// TestV128Store64LaneOutOfRangePanicsHigh/Low cover the guard rails on
// V128Store64Lane's lane argument — only 0 and 1 are legal i64 lanes.
func TestV128Store64LaneOutOfRangePanicsHigh(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on lane > 1")
		}
	}()
	fn := NewFunction("f", "f", nil, nil)
	fn.V128Store64Lane(0, 2)
}

func TestV128Store64LaneOutOfRangePanicsLow(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on lane < 0")
		}
	}()
	fn := NewFunction("f", "f", nil, nil)
	fn.V128Store64Lane(0, -1)
}

func TestI32x4ExtractLaneOutOfRangePanicsLow(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on lane < 0")
		}
	}()
	fn := NewFunction("f", "f", nil, nil)
	fn.I32x4ExtractLane(-1)
}

// TestControlFlow covers Block / Loop / BrIf / Br / End.
func TestControlFlow(t *testing.T) {
	fn := NewFunction("f", "f", nil, nil)
	done := fn.Block("done")
	loop := fn.Loop("loop")
	fn.BrIf(done)
	fn.Br(loop)
	fn.End()
	fn.End()
	body := fn.String()
	for _, w := range []string{"(block $done", "(loop $loop", "(br_if $done)", "(br $loop)", ")"} {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in %q", w, body)
		}
	}
}

// TestModuleRender covers Module.String, imports, and multi-function assembly.
func TestModuleRender(t *testing.T) {
	m := NewModule()
	m.ImportMemory("env", "memory")
	m.Add(NewFunction("a", "a", nil, nil))
	m.Add(NewFunction("b", "", nil, nil))
	got := m.String()
	if !strings.HasPrefix(got, ";; Code generated by") {
		t.Errorf("missing generator header: %q", got)
	}
	if !strings.Contains(got, `(import "env" "memory" (memory 0))`) {
		t.Errorf("missing memory import: %q", got)
	}
	if !strings.Contains(got, "(func $a (export \"a\")") {
		t.Errorf("missing exported func: %q", got)
	}
	if !strings.Contains(got, "(func $b\n") {
		t.Errorf("missing unexported func: %q", got)
	}
	if !strings.HasSuffix(got, ")\n") {
		t.Errorf("wrong module suffix: %q", got)
	}
}

// itoaByte is used by TestI32x4ExtractLane; kept tiny + pure to avoid
// needing strconv as a test-only import.
func itoaByte(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return ""
}
