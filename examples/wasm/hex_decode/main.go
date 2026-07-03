// Command hex_decode builds hex_decode.wat programmatically via the
// wasm-emit package — the eighth kernel and the companion of hex_encode.
// It shapes like the reverse of encoding/hex.Encode: 16 hex chars in →
// 8 raw bytes out.
//
// Algorithm (per 16-char block):
//
//	chars    = v128.load(src + 16*i)                           ; 16 ASCII chars
//	digit    = (chars >= '0') & (chars <= '9')                  ; 0xff on 0-9
//	upper    = (chars >= 'A') & (chars <= 'F')                  ; 0xff on A-F
//	lower    = (chars >= 'a') & (chars <= 'f')                  ; 0xff on a-f
//	offset   = (digit & 48) | (upper & 55) | (lower & 87)       ; per-lane
//	nibbles  = i8x16.sub(chars, offset)                         ; each lane 0..15
//	hi       = i8x16.shuffle(nibbles, nibbles, [0,2,4,6,8,10,12,14,·,·,…])
//	lo       = i8x16.shuffle(nibbles, nibbles, [1,3,5,7,9,11,13,15,·,·,…])
//	bytes    = (hi << 4) | lo                                   ; low 8 lanes packed
//	v128.store64_lane bytes @ (dst + 8*i, lane 0)               ; store 8 bytes
//
// The three range-mask offsets encode the ASCII-to-nibble arithmetic:
//   '0' (48) - 0  = 48   → subtract 48 from digits so they become 0..9
//   'A' (65) - 10 = 55   → subtract 55 from uppercase so they become 10..15
//   'a' (97) - 10 = 87   → subtract 87 from lowercase so they become 10..15
// AND-with-mask + OR combines them into a single per-lane offset vector so
// each lane sees exactly the offset for its own range. Anything outside
// the three ranges (whitespace, punctuation, invalid input) subtracts 0,
// which produces garbage — callers should validate the input separately
// or use a validating variant.
//
// Signature (as seen by //go:wasmimport consumers):
//
//	(func $hex_decode (param $dstPtr i32) (param $srcPtr i32)
//	                  (param $nBlocks i32))
//
// Each block consumes 16 input chars and writes 8 output bytes. Caller
// passes nBlocks = len(src)/16 and handles the tail scalarly.
//
// Run with:
//
//	go run . > hex_decode.wat
package main

import (
	"fmt"
	"io"
	"os"

	wasm "github.com/go-asmgen/asmgen/wasm"
)

var (
	runFunc = run
	osExit  = os.Exit
)

func main() {
	if err := runFunc(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		osExit(1)
	}
}

func run(out io.Writer) error {
	m := wasm.NewModule()
	m.ImportMemory("env", "memory")

	// Broadcast constants for the range checks + arithmetic offsets.
	broadcast := func(b byte) []byte {
		v := make([]byte, 16)
		for i := range v {
			v[i] = b
		}
		return v
	}
	c0 := broadcast('0')
	c9 := broadcast('9')
	cA := broadcast('A')
	cF := broadcast('F')
	ca := broadcast('a')
	cf := broadcast('f')
	off48 := broadcast(48) // digit offset
	off55 := broadcast(55) // uppercase offset ('A' - 10)
	off87 := broadcast(87) // lowercase offset ('a' - 10)

	// Shuffle patterns to gather even / odd nibbles into the low 8 lanes.
	// The high 8 lanes of the shuffle output are unused (the v128.store64_lane
	// only stores the low 8 bytes), so any legal index works there — we use
	// 0 for compactness.
	shufHi := []byte{0, 2, 4, 6, 8, 10, 12, 14, 0, 0, 0, 0, 0, 0, 0, 0}
	shufLo := []byte{1, 3, 5, 7, 9, 11, 13, 15, 0, 0, 0, 0, 0, 0, 0, 0}

	fn := wasm.NewFunction("hex_decode", "hex_decode",
		[]wasm.Param{
			{Name: "dstPtr", Type: wasm.I32},
			{Name: "srcPtr", Type: wasm.I32},
			{Name: "nBlocks", Type: wasm.I32},
		},
		nil,
	)
	fn.Local("i", wasm.I32)
	fn.Local("chars", wasm.V128)
	fn.Local("digit", wasm.V128)
	fn.Local("upper", wasm.V128)
	fn.Local("lower", wasm.V128)
	fn.Local("offset", wasm.V128)
	fn.Local("nibbles", wasm.V128)

	fn.I32Const(0)
	fn.LocalSet("i")

	done := fn.Block("done")
	loop := fn.Loop("loop")

	// if i >= nBlocks → exit
	fn.LocalGet("i")
	fn.LocalGet("nBlocks")
	fn.I32GeS()
	fn.BrIf(done)

	// chars = v128.load(srcPtr + i*16)
	fn.LocalGet("srcPtr")
	fn.LocalGet("i")
	fn.I32Const(16)
	fn.I32Mul()
	fn.I32Add()
	fn.V128Load(0)
	fn.LocalSet("chars")

	// digit = (chars >= '0') & (chars <= '9')
	fn.LocalGet("chars")
	fn.V128Const16(c0)
	fn.I8x16GeS()
	fn.LocalGet("chars")
	fn.V128Const16(c9)
	fn.I8x16LeS()
	fn.V128And()
	fn.LocalSet("digit")

	// upper = (chars >= 'A') & (chars <= 'F')
	fn.LocalGet("chars")
	fn.V128Const16(cA)
	fn.I8x16GeS()
	fn.LocalGet("chars")
	fn.V128Const16(cF)
	fn.I8x16LeS()
	fn.V128And()
	fn.LocalSet("upper")

	// lower = (chars >= 'a') & (chars <= 'f')
	fn.LocalGet("chars")
	fn.V128Const16(ca)
	fn.I8x16GeS()
	fn.LocalGet("chars")
	fn.V128Const16(cf)
	fn.I8x16LeS()
	fn.V128And()
	fn.LocalSet("lower")

	// offset = (digit & 48) | (upper & 55) | (lower & 87)
	fn.LocalGet("digit")
	fn.V128Const16(off48)
	fn.V128And()
	fn.LocalGet("upper")
	fn.V128Const16(off55)
	fn.V128And()
	fn.V128Or()
	fn.LocalGet("lower")
	fn.V128Const16(off87)
	fn.V128And()
	fn.V128Or()
	fn.LocalSet("offset")

	// nibbles = i8x16.sub(chars, offset)
	fn.LocalGet("chars")
	fn.LocalGet("offset")
	fn.I8x16Sub()
	fn.LocalSet("nibbles")

	// Pack: bytes = ( shuffle(nibbles, nibbles, shufHi) << 4 ) | shuffle(nibbles, nibbles, shufLo)
	// Stack the output address first so v128.store64_lane can pop it below the
	// packed v128 at the end.
	fn.LocalGet("dstPtr")
	fn.LocalGet("i")
	fn.I32Const(8)
	fn.I32Mul()
	fn.I32Add()

	// hi_shuffled << 4
	fn.LocalGet("nibbles")
	fn.LocalGet("nibbles")
	fn.I8x16Shuffle(shufHi)
	fn.I32Const(4)
	fn.I8x16Shl()

	// | lo_shuffled
	fn.LocalGet("nibbles")
	fn.LocalGet("nibbles")
	fn.I8x16Shuffle(shufLo)
	fn.V128Or()

	// v128.store64_lane offset=0 lane=0 (8 bytes) @ dst + 8*i
	fn.V128Store64Lane(0, 0)

	// i += 1
	fn.LocalGet("i")
	fn.I32Const(1)
	fn.I32Add()
	fn.LocalSet("i")
	fn.Br(loop)

	fn.End() // loop
	fn.End() // done block

	m.Add(fn)
	_, err := fmt.Fprint(out, m.String())
	return err
}
