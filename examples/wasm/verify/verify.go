// Command verify runs a generated wasm kernel through wazero and cross-checks
// its output against a Go reference. Usage:
//
//	go run . <kernel> <wasm-path>
//
//	<kernel>: matchlen | hex | popcount
//	<wasm-path>: path to the .wasm module (produced by wat2wasm)
//
// Example:
//
//	go run . hex ../hex/hex.wasm
//	go run . popcount ../popcount/popcount.wasm
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"hash/adler32"
	"math/bits"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// envWasm is a 32-page (2 MiB) linear-memory module exported as "env.memory",
// satisfying every kernel's (import "env" "memory") declaration.
var envWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x05, 0x03, 0x01, 0x00, 0x20, // 32 pages
	0x07, 0x0a, 0x01, 0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00,
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: verify <kernel> <wasm-path>")
		fmt.Fprintln(os.Stderr, "  kernel: matchlen | hex | hex_decode | popcount | toupper | memchr | isascii | utf8len | json_clean | adler32")
		os.Exit(2)
	}
	kernel := os.Args[1]
	wasmPath := os.Args[2]

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	envMod, err := r.InstantiateWithConfig(ctx, envWasm,
		wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		die("instantiate env: %v", err)
	}
	mem := envMod.Memory()

	kbytes, err := os.ReadFile(wasmPath)
	if err != nil {
		die("read %s: %v", wasmPath, err)
	}
	kern, err := r.Instantiate(ctx, kbytes)
	if err != nil {
		die("instantiate kernel: %v", err)
	}

	var runErr error
	switch kernel {
	case "matchlen":
		runErr = verifyMatchlen(ctx, kern, mem)
	case "hex":
		runErr = verifyHex(ctx, kern, mem)
	case "popcount":
		runErr = verifyPopcount(ctx, kern, mem)
	case "toupper":
		runErr = verifyToupper(ctx, kern, mem)
	case "memchr":
		runErr = verifyMemchr(ctx, kern, mem)
	case "isascii":
		runErr = verifyIsascii(ctx, kern, mem)
	case "utf8len":
		runErr = verifyUtf8len(ctx, kern, mem)
	case "hex_decode":
		runErr = verifyHexDecode(ctx, kern, mem)
	case "json_clean":
		runErr = verifyJsonClean(ctx, kern, mem)
	case "adler32":
		runErr = verifyAdler32(ctx, kern, mem)
	default:
		die("unknown kernel %q (want matchlen | hex | hex_decode | popcount | toupper | memchr | isascii | utf8len | json_clean | adler32)", kernel)
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
}

// -----------------------------------------------------------------------------
// matchlen
// -----------------------------------------------------------------------------

func verifyMatchlen(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("matchlen16")
	if fn == nil {
		return fmt.Errorf("kernel does not export matchlen16")
	}
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"x", "x", 1},
		{"x", "y", 0},
		{"hello", "hello", 5},
		{"hello", "help!", 3},
		{"abcdefghijklmnop", "abcdefghijklmnop", 16},
		{"abcdefghijklmnop", "abcdefghijklmnoq", 15},
		{"abcdefghijklmnopq", "abcdefghijklmnopq", 17},
		{"abcdefghijklmnopqrstuvwxyz012345", "abcdefghijklmnopqrstuvwxyz012345", 32},
		{"abcdefghijklmnopqrstuvwxyz012345", "abcdefghijklmnopqrstuvwxyz01234!", 31},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaab", 51},
	}
	fail := 0
	for i, c := range cases {
		aOff := uint32(0)
		bOff := uint32(0x100000)
		mem.Write(aOff, []byte(c.a))
		mem.Write(bOff, []byte(c.b))
		limit := uint32(len(c.a))
		if uint32(len(c.b)) < limit {
			limit = uint32(len(c.b))
		}
		results, err := fn.Call(ctx, uint64(aOff), uint64(bOff), uint64(limit))
		if err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		got := int(uint32(results[0]))
		status := "OK  "
		if got != c.want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s matchlen(%q, %q) = %d, want %d\n",
			i, status, snip(c.a), snip(c.b), got, c.want)
	}
	if fail > 0 {
		return fmt.Errorf("%d matchlen case(s) failed", fail)
	}
	fmt.Println("\nAll matchlen cases passed.")
	return nil
}

// -----------------------------------------------------------------------------
// hex
// -----------------------------------------------------------------------------

func verifyHex(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("hex_encode")
	if fn == nil {
		return fmt.Errorf("kernel does not export hex_encode")
	}
	cases := []string{
		"0000000000000000000000000000000000000000000000000000000000000000",
		"0123456789abcdef00112233445566778899aabbccddeeff",
		"0123456789abcdef0123456789abcdef",
		"deadbeefcafebabe0123456789abcdef",
		"ffffffffffffffffffffffffffffffffeeeeeeeeeeeeeeee00000000000000001111111111111111",
	}
	fail := 0
	for i, hexIn := range cases {
		bytesIn, err := hex.DecodeString(hexIn)
		if err != nil {
			return fmt.Errorf("case[%d] decode: %w", i, err)
		}
		nBlocks := uint32(len(bytesIn) / 16)
		if nBlocks == 0 {
			fmt.Printf("[%d] SKIP  input < 16 bytes\n", i)
			continue
		}
		srcOff := uint32(0)
		dstOff := uint32(0x10000)
		blank := make([]byte, int(nBlocks)*32+64)
		mem.Write(dstOff, blank)
		if ok := mem.Write(srcOff, bytesIn); !ok {
			return fmt.Errorf("write src")
		}
		if _, err := fn.Call(ctx, uint64(dstOff), uint64(srcOff), uint64(nBlocks)); err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		gotBytes, ok := mem.Read(dstOff, nBlocks*32)
		if !ok {
			return fmt.Errorf("read dst")
		}
		got := string(gotBytes)
		want := hex.EncodeToString(bytesIn[:nBlocks*16])
		status := "OK  "
		if got != want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s  nBlocks=%d  got=%s  want=%s\n", i, status, nBlocks, snip(got), snip(want))
	}
	if fail > 0 {
		return fmt.Errorf("%d hex case(s) failed", fail)
	}
	fmt.Println("\nAll hex cases passed.")
	return nil
}

// -----------------------------------------------------------------------------
// popcount
// -----------------------------------------------------------------------------

func verifyPopcount(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("popcount")
	if fn == nil {
		return fmt.Errorf("kernel does not export popcount")
	}
	// Test inputs — each is a multiple of 16 bytes.
	cases := [][]byte{
		make([]byte, 16), // all zero
		bytes16(0xff),    // all 0xff → 16*8 = 128 bits
		bytes16(0x55),    // alternating 01010101 → 16*4 = 64 bits
		bytesN(32, 0xff), // 32 bytes of 0xff → 256 bits
		bytesRange(48),   // 48 bytes 0..47
		bytesN(1024, 0xa5), // 1024 bytes of 0xa5 → 1024*4 = 4096 bits
	}
	fail := 0
	for i, in := range cases {
		nBlocks := uint32(len(in) / 16)
		srcOff := uint32(0)
		mem.Write(srcOff, in)

		results, err := fn.Call(ctx, uint64(srcOff), uint64(nBlocks))
		if err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		got := int(uint32(results[0]))
		want := 0
		for _, b := range in {
			want += bits.OnesCount8(b)
		}
		status := "OK  "
		if got != want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s  n=%d  got=%d  want=%d\n", i, status, len(in), got, want)
	}
	if fail > 0 {
		return fmt.Errorf("%d popcount case(s) failed", fail)
	}
	fmt.Println("\nAll popcount cases passed.")
	return nil
}

// -----------------------------------------------------------------------------
// toupper
// -----------------------------------------------------------------------------

// verifyToupper runs the toupper kernel and cross-checks against
// strings.ToUpper for ASCII-only inputs (the kernel only case-folds the
// [a..z] range; non-ASCII / multibyte UTF-8 is out of scope).
func verifyToupper(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("toupper")
	if fn == nil {
		return fmt.Errorf("kernel does not export toupper")
	}
	cases := []string{
		"aaaaaaaaaaaaaaaa",                 // 16B all lower → all upper
		"AAAAAAAAAAAAAAAA",                 // 16B all upper → unchanged
		"abcdefghijklmnop",                 // 16B mixed lower letters
		"aBcDeFgHiJkLmNoP",                 // 16B mixed case
		"0123456789!@#$%^",                 // 16B non-letter (unchanged)
		"hello, world!!!!hello, world!!!!", // 32B two blocks
		"the quick brown fox jumps over th", // 32B (first block of 32 chars)
	}
	fail := 0
	for i, in := range cases {
		nBlocks := uint32(len(in) / 16)
		if nBlocks == 0 {
			fmt.Printf("[%d] SKIP  input < 16 bytes\n", i)
			continue
		}
		srcOff := uint32(0)
		dstOff := uint32(0x10000)
		blank := make([]byte, int(nBlocks)*16+16)
		mem.Write(dstOff, blank)
		if ok := mem.Write(srcOff, []byte(in)); !ok {
			return fmt.Errorf("write src")
		}
		if _, err := fn.Call(ctx, uint64(dstOff), uint64(srcOff), uint64(nBlocks)); err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		gotBytes, ok := mem.Read(dstOff, nBlocks*16)
		if !ok {
			return fmt.Errorf("read dst")
		}
		got := string(gotBytes)
		want := strings.ToUpper(in[:nBlocks*16])
		status := "OK  "
		if got != want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s  in=%q got=%q want=%q\n", i, status, snip(in), snip(got), snip(want))
	}
	if fail > 0 {
		return fmt.Errorf("%d toupper case(s) failed", fail)
	}
	fmt.Println("\nAll toupper cases passed.")
	return nil
}

// -----------------------------------------------------------------------------
// memchr
// -----------------------------------------------------------------------------

// verifyMemchr runs the memchr kernel and cross-checks against
// bytes.IndexByte on 16-byte-aligned inputs. The kernel signature is
// (srcPtr, nBlocks, needle) → int32; -1 means "not present in any of the
// nBlocks × 16 scanned bytes".
func verifyMemchr(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("memchr")
	if fn == nil {
		return fmt.Errorf("kernel does not export memchr")
	}
	type c struct {
		buf    []byte
		needle byte
		want   int
	}
	cases := []c{
		// 16 bytes, needle at offset 0
		{append(bytes.Repeat([]byte{0}, 0), []byte("Xaaaaaaaaaaaaaaa")...), 'X', 0},
		// 16 bytes, needle at last lane
		{[]byte("aaaaaaaaaaaaaaaX"), 'X', 15},
		// 16 bytes, needle in the middle
		{[]byte("aaaaaaaXbbbbbbbb"), 'X', 7},
		// 16 bytes, needle absent
		{[]byte("aaaaaaaaaaaaaaaa"), 'X', -1},
		// 32 bytes, needle in second block
		{append([]byte("aaaaaaaaaaaaaaaa"), []byte("bbbXbbbbbbbbbbbb")...), 'X', 19},
		// 32 bytes, needle absent
		{append([]byte("aaaaaaaaaaaaaaaa"), []byte("bbbbbbbbbbbbbbbb")...), 'X', -1},
		// 64 bytes, needle at offset 33 (2nd block lane 1 of 4-block scan)
		{withByte(bytes.Repeat([]byte("a"), 64), 33, 'X'), 'X', 33},
		// zero-byte needle
		{[]byte("aaaaaaaaaaaaaa\x00a"), 0, 14},
	}
	fail := 0
	for i, tc := range cases {
		nBlocks := uint32(len(tc.buf) / 16)
		if nBlocks == 0 {
			fmt.Printf("[%d] SKIP  input < 16 bytes\n", i)
			continue
		}
		srcOff := uint32(0)
		// Zero the region after the scan too, so a broken kernel that
		// scanned past nBlocks would definitely miss.
		mem.Write(srcOff, make([]byte, int(nBlocks)*16+32))
		if ok := mem.Write(srcOff, tc.buf); !ok {
			return fmt.Errorf("write src")
		}
		results, err := fn.Call(ctx, uint64(srcOff), uint64(nBlocks), uint64(tc.needle))
		if err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		got := int(int32(results[0]))
		status := "OK  "
		if got != tc.want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s  n=%d needle=%q  got=%d want=%d\n",
			i, status, len(tc.buf), tc.needle, got, tc.want)
	}
	if fail > 0 {
		return fmt.Errorf("%d memchr case(s) failed", fail)
	}
	fmt.Println("\nAll memchr cases passed.")
	return nil
}

// withByte returns a copy of b with position pos overwritten to c.
func withByte(b []byte, pos int, c byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	out[pos] = c
	return out
}

// -----------------------------------------------------------------------------
// isascii
// -----------------------------------------------------------------------------

// verifyIsascii runs the isascii kernel and cross-checks against a plain
// Go loop that scans for any byte >= 128. Returns 1 iff every scanned
// byte is in the 7-bit ASCII range, else 0.
func verifyIsascii(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("isascii")
	if fn == nil {
		return fmt.Errorf("kernel does not export isascii")
	}
	type c struct {
		buf  []byte
		want int
	}
	cases := []c{
		// 16 bytes all ASCII
		{[]byte("aaaaaaaaaaaaaaaa"), 1},
		// 16 bytes with a non-ASCII in the middle
		{append([]byte("aaaaaaa\xe9bbbbbbbb"), 0), 0},
		// 16 bytes with high bit set in the last lane
		{[]byte{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 0x80}, 0},
		// 16 bytes with high bit set in the first lane
		{[]byte{0xff, 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p'}, 0},
		// 32 bytes: first block ASCII, second block ASCII
		{append([]byte("aaaaaaaaaaaaaaaa"), []byte("bbbbbbbbbbbbbbbb")...), 1},
		// 32 bytes: first block ASCII, second block has non-ASCII
		{append([]byte("aaaaaaaaaaaaaaaa"), []byte("bbbb\xc3\xbb bbbbbbbbb")...), 0},
		// 64 bytes all ASCII
		{bytes.Repeat([]byte("hello world!!!!!"), 4), 1},
		// 64 bytes with a stray 0x80 in the third block
		{withByte(bytes.Repeat([]byte{'A'}, 64), 33, 0x80), 0},
		// 16 zero bytes are ASCII (\x00 is < 128)
		{make([]byte, 16), 1},
	}
	fail := 0
	for i, tc := range cases {
		nBlocks := uint32(len(tc.buf) / 16)
		if nBlocks == 0 {
			fmt.Printf("[%d] SKIP  input < 16 bytes\n", i)
			continue
		}
		srcOff := uint32(0)
		mem.Write(srcOff, make([]byte, int(nBlocks)*16+32))
		if ok := mem.Write(srcOff, tc.buf); !ok {
			return fmt.Errorf("write src")
		}
		results, err := fn.Call(ctx, uint64(srcOff), uint64(nBlocks))
		if err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		got := int(uint32(results[0]))
		status := "OK  "
		if got != tc.want {
			status = "FAIL"
			fail++
		}
		// Cross-check the want against a plain Go loop as a sanity anchor.
		refWant := 1
		for _, b := range tc.buf[:nBlocks*16] {
			if b >= 0x80 {
				refWant = 0
				break
			}
		}
		if refWant != tc.want {
			return fmt.Errorf("case %d: hand-written want=%d disagrees with Go reference=%d", i, tc.want, refWant)
		}
		fmt.Printf("[%d] %s  n=%d  got=%d want=%d\n", i, status, len(tc.buf), got, tc.want)
	}
	if fail > 0 {
		return fmt.Errorf("%d isascii case(s) failed", fail)
	}
	fmt.Println("\nAll isascii cases passed.")
	return nil
}

// -----------------------------------------------------------------------------
// hex_decode
// -----------------------------------------------------------------------------

// verifyHexDecode runs the hex_decode kernel and cross-checks against
// encoding/hex.DecodeString on inputs whose length is a multiple of 16
// hex chars → 8 bytes per block.
func verifyHexDecode(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("hex_decode")
	if fn == nil {
		return fmt.Errorf("kernel does not export hex_decode")
	}
	// Each case: an even-length hex string of at least 16 chars, length
	// a multiple of 16 so nBlocks = len/16 covers it fully.
	cases := []string{
		"0000000000000000",                                                     // 16 chars → 8 zero bytes
		"0123456789abcdef",                                                     // 16 chars → 0x01,0x23,0x45,0x67,0x89,0xab,0xcd,0xef
		"0123456789ABCDEF",                                                     // uppercase variant
		"deadbeefcafebabe",                                                     // 16 chars → 8 bytes
		"DEADBEEFCAFEBABE",                                                     // uppercase
		"0123456789abcdef0123456789ABCDEF",                                     // 32 chars → 16 bytes (2 blocks)
		"ffffffffffffffff00000000000000001111111111111111eeeeeeeeeeeeeeee",     // 64 chars → 32 bytes (4 blocks)
	}
	fail := 0
	for i, hexIn := range cases {
		nBlocks := uint32(len(hexIn) / 16)
		if nBlocks == 0 {
			fmt.Printf("[%d] SKIP  input < 16 chars\n", i)
			continue
		}
		srcOff := uint32(0)
		dstOff := uint32(0x10000)
		// Zero output region so a broken kernel writing garbage is visible.
		mem.Write(dstOff, make([]byte, int(nBlocks)*8+32))
		if ok := mem.Write(srcOff, []byte(hexIn)); !ok {
			return fmt.Errorf("write src")
		}
		if _, err := fn.Call(ctx, uint64(dstOff), uint64(srcOff), uint64(nBlocks)); err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		gotBytes, ok := mem.Read(dstOff, nBlocks*8)
		if !ok {
			return fmt.Errorf("read dst")
		}
		want, err := hex.DecodeString(hexIn[:nBlocks*16])
		if err != nil {
			return fmt.Errorf("case %d: reference DecodeString: %w", i, err)
		}
		status := "OK  "
		if !bytes.Equal(gotBytes, want) {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s  nBlocks=%d  got=%s  want=%s\n",
			i, status, nBlocks, hex.EncodeToString(gotBytes), hex.EncodeToString(want))
	}
	if fail > 0 {
		return fmt.Errorf("%d hex_decode case(s) failed", fail)
	}
	fmt.Println("\nAll hex_decode cases passed.")
	return nil
}

// -----------------------------------------------------------------------------
// utf8len
// -----------------------------------------------------------------------------

// verifyUtf8len runs the utf8len kernel and cross-checks against
// unicode/utf8.RuneCount on valid UTF-8 inputs. The kernel counts
// "rune-start" bytes (bytes whose top two bits are NOT 10) — for valid
// UTF-8 this equals the rune count exactly.
func verifyUtf8len(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("utf8len")
	if fn == nil {
		return fmt.Errorf("kernel does not export utf8len")
	}
	// Each case is a valid UTF-8 buffer whose byte length is a multiple of 16.
	// utf8.RuneCount is the reference; the kernel MUST match it exactly.
	cases := []string{
		"aaaaaaaaaaaaaaaa",                      // 16B ASCII (16 runes)
		strings.Repeat("é", 8),                  // 8×2B = 16B (8 runes)
		"héllo, world! ok",                      // 16B mixed (15 runes: é is 2B)
		"aaaaaaaaaaaaaaaa" + "bbbbbbbbbbbbbbbb", // 32B all ASCII (32 runes)
		"héllo, world! ok" + "aaaaaaaaaaaaaaaa", // 32B (31 runes)
		strings.Repeat("世界", 8),                 // 48B, 16 runes (3B each)
		strings.Repeat("𝔸", 4),                  // 4×4B = 16B, 4 runes
		strings.Repeat("a", 16*4),               // 64B ASCII (64 runes)
	}
	fail := 0
	for i, in := range cases {
		buf := []byte(in)
		if len(buf)%16 != 0 {
			// Pad with ASCII 'a' up to next 16-byte boundary.
			pad := 16 - (len(buf) % 16)
			buf = append(buf, bytes.Repeat([]byte("a"), pad)...)
		}
		if !utf8.Valid(buf) {
			return fmt.Errorf("case %d: reference buffer is not valid UTF-8", i)
		}
		nBlocks := uint32(len(buf) / 16)
		srcOff := uint32(0)
		mem.Write(srcOff, make([]byte, int(nBlocks)*16+32))
		if ok := mem.Write(srcOff, buf); !ok {
			return fmt.Errorf("write src")
		}
		results, err := fn.Call(ctx, uint64(srcOff), uint64(nBlocks))
		if err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		got := int(uint32(results[0]))
		want := utf8.RuneCount(buf[:nBlocks*16])
		status := "OK  "
		if got != want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s  n=%d  got=%d want=%d\n", i, status, len(buf), got, want)
	}
	if fail > 0 {
		return fmt.Errorf("%d utf8len case(s) failed", fail)
	}
	fmt.Println("\nAll utf8len cases passed.")
	return nil
}

// -----------------------------------------------------------------------------
// json_clean
// -----------------------------------------------------------------------------

// verifyJsonClean runs the json_clean kernel and cross-checks against a
// plain Go loop that classifies every byte: `"` (0x22), `\` (0x5c),
// or any control char (< 0x20) makes the buffer "dirty" and returns 0.
// All other bytes are safe to bulk-copy into a JSON string body.
func verifyJsonClean(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("json_clean")
	if fn == nil {
		return fmt.Errorf("kernel does not export json_clean")
	}
	type c struct {
		buf  []byte
		want int
	}
	cases := []c{
		// 16 bytes clean
		{[]byte("clean text here!"), 1},
		// 16 bytes with a `"` in the middle
		{[]byte(`plain then " and`), 0},
		// 16 bytes with a `\` in the middle
		{[]byte(`plain then \ and`), 0},
		// 16 bytes with a control byte (\n = 0x0a)
		{append([]byte("no ctrl chars\nx"), 0), 0},
		// 16 bytes at the boundary — 0x1f (unit separator) is dirty, 0x20 (space) is clean
		{[]byte{0x1f, 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a'}, 0},
		{[]byte{0x20, 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a', 'a'}, 1},
		// 32 bytes: first block clean, second block dirty
		{append([]byte("aaaaaaaaaaaaaaaa"), []byte(`no ctrl but \ ok`)...), 0},
		// 32 bytes: both clean
		{append([]byte("aaaaaaaaaaaaaaaa"), []byte("bbbbbbbbbbbbbbbb")...), 1},
		// 64 bytes clean ASCII
		{bytes.Repeat([]byte("clean 16 bytes! "), 4), 1},
		// 64 bytes with dirty byte in block 3
		{withByte(bytes.Repeat([]byte{'A'}, 64), 33, 0x00), 0},
		// High-bit bytes are clean (they'd fail a strict-JSON UTF-8 check, but
		// json_clean's job is the "safe to bulk-copy" preflight, not full
		// validation — anything ≥ 0x20 that isn't " or \ passes).
		{bytes.Repeat([]byte{0xc3, 0xa9}, 8), 1},
	}
	fail := 0
	for i, tc := range cases {
		nBlocks := uint32(len(tc.buf) / 16)
		if nBlocks == 0 {
			fmt.Printf("[%d] SKIP  input < 16 bytes\n", i)
			continue
		}
		srcOff := uint32(0)
		mem.Write(srcOff, make([]byte, int(nBlocks)*16+32))
		if ok := mem.Write(srcOff, tc.buf); !ok {
			return fmt.Errorf("write src")
		}
		results, err := fn.Call(ctx, uint64(srcOff), uint64(nBlocks))
		if err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}
		got := int(uint32(results[0]))
		// Cross-check want against a plain Go loop.
		refWant := 1
		for _, b := range tc.buf[:nBlocks*16] {
			if b == '"' || b == '\\' || b < 0x20 {
				refWant = 0
				break
			}
		}
		if refWant != tc.want {
			return fmt.Errorf("case %d: hand-written want=%d disagrees with Go reference=%d", i, tc.want, refWant)
		}
		status := "OK  "
		if got != tc.want {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s  n=%d  got=%d want=%d\n", i, status, len(tc.buf), got, tc.want)
	}
	if fail > 0 {
		return fmt.Errorf("%d json_clean case(s) failed", fail)
	}
	fmt.Println("\nAll json_clean cases passed.")
	return nil
}

// -----------------------------------------------------------------------------
// adler32
// -----------------------------------------------------------------------------

// verifyAdler32 runs the adler32 kernel and cross-checks against
// hash/adler32. The kernel processes nBlocks × 16 bytes without doing
// the mod-65521 reduction, so the harness runs the kernel then applies
// the modulo to compare against the Go stdlib's checksum. Test inputs
// are all multiples of 16 bytes, capped at 5552 (NMAX) so intermediate
// a and b stay in u32 range.
func verifyAdler32(ctx context.Context, kern api.Module, mem api.Memory) error {
	fn := kern.ExportedFunction("adler32")
	if fn == nil {
		return fmt.Errorf("kernel does not export adler32")
	}
	cases := [][]byte{
		make([]byte, 16),   // 16 zero bytes
		bytes16(0xff),      // 16 x 0xff
		bytesRange(16),     // 16 bytes 0..15
		bytesN(32, 0x5a),   // 32 x 0x5a
		bytesRange(64),     // 64 bytes 0..63
		bytesN(1024, 0xa5), // 1024 x 0xa5
		bytesRange(240),    // 240 bytes 0..239 (255 max)
		// Larger but under NMAX (5552 bytes = 347 blocks × 16).
		mixedPattern(4096),
	}
	fail := 0
	for i, in := range cases {
		nBlocks := uint32(len(in) / 16)
		if nBlocks == 0 {
			fmt.Printf("[%d] SKIP  input < 16 bytes\n", i)
			continue
		}
		srcOff := uint32(0)
		aOutOff := uint32(0x10000)
		bOutOff := uint32(0x10004)
		mem.Write(srcOff, in)

		if _, err := fn.Call(ctx,
			uint64(srcOff), uint64(nBlocks),
			uint64(1), uint64(0),
			uint64(aOutOff), uint64(bOutOff)); err != nil {
			fmt.Printf("[%d] ERR   %v\n", i, err)
			fail++
			continue
		}

		aRaw, ok := mem.ReadUint32Le(aOutOff)
		if !ok {
			return fmt.Errorf("read a")
		}
		bRaw, ok := mem.ReadUint32Le(bOutOff)
		if !ok {
			return fmt.Errorf("read b")
		}
		gotAdler := ((bRaw % 65521) << 16) | (aRaw % 65521)
		// Reference: hash/adler32 over the same nBlocks × 16 bytes.
		h := goAdler32Checksum(in[:nBlocks*16])
		status := "OK  "
		if gotAdler != h {
			status = "FAIL"
			fail++
		}
		fmt.Printf("[%d] %s  n=%d  got=%08x want=%08x\n", i, status, len(in), gotAdler, h)
	}
	if fail > 0 {
		return fmt.Errorf("%d adler32 case(s) failed", fail)
	}
	fmt.Println("\nAll adler32 cases passed.")
	return nil
}

// goAdler32Checksum wraps hash/adler32 so we can pull it in only in the
// verifier (keeps the kernel package dependency-free).
func goAdler32Checksum(b []byte) uint32 {
	return adler32.Checksum(b)
}

// mixedPattern generates a deterministic non-uniform byte pattern of the
// given length (multiple of 16) — meant to catch alignment / lane-order
// bugs the constant-byte inputs would miss.
func mixedPattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 ^ (i >> 3))
	}
	return b
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

func bytes16(fill byte) []byte {
	b := make([]byte, 16)
	for i := range b {
		b[i] = fill
	}
	return b
}
func bytesN(n int, fill byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = fill
	}
	return b
}
func bytesRange(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func snip(s string) string {
	if len(s) > 40 {
		return s[:37] + "..."
	}
	return s
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "verify: "+format+"\n", args...)
	os.Exit(2)
}
