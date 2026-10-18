package amd64

import (
	"fmt"

	asm "github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

// The kernels go-fft (github.com/go-fft/fft) generates with this package, as
// its internal/kernels/asmgen/amd64/gen.go does: every file, in its order,
// with its symbols and its constants tables. testdata/gofft.sha256 holds the
// digest of each file as go-fft commits it, so the tests hold the whole set
// to go-fft's bytes, and assemble it.

// goFFT is go-fft's interleaved and split constants tables.
var goFFT = &Kernels{
	K: Consts{
		Sign: 0, Half: 32, Sin120: 64, C51: 96, C52: 128, S51: 160, S52: 192, H: 224,
		AddSub: 256,
		R16C1:  288, R16S1: 320, R16NH: 352, R16NC1: 384,
		R16SS1: 416, R16SSH: 448, R16SSC1: 480, R16SSN1: 512,
		R9C1: 544, R9S1: 576, R9C2: 608, R9S2: 640, R9C4: 672, R9S4: 704,
		R32: 736,
	},
	SplitK: SplitConsts{H: 0, Neg: 32, Half: 64, Sin120: 96, C51: 128, C52: 160, S51: 192, S52: 224, R32: 256},
}

// goFFTFMA builds go-fft's FMA twins (its Round 41).
var goFFTFMA = &Kernels{K: goFFT.K, SplitK: goFFT.SplitK, FMA: true}

func forSplits(f func(inS, outS, inverse bool)) {
	for _, inverse := range []bool{false, true} {
		for _, io := range [][2]bool{{false, true}, {true, true}, {true, false}, {false, false}} {
			f(io[0], io[1], inverse)
		}
	}
}

func splitName(prefix string, r int, inS, outS, inverse bool) string {
	io := map[bool]string{false: "I", true: "S"}
	dir := "Fwd"
	if inverse {
		dir = "Inv"
	}
	return fmt.Sprintf("%s%d%s%s%s", prefix, r, io[inS], io[outS], dir)
}

// goFFTFiles returns go-fft's amd64 files by name, their text as committed.
func goFFTFiles() map[string]string {
	k, kf := goFFT, goFFTFMA
	files := map[string]string{}
	f := emit.NewFile("amd64")
	CMulSSE2(f, "cmulSSE2")
	files["cmul_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	Radix2SSE2(f, "radix2StageSSE2")
	Radix4SSE2(f, "radix4StageSSE2Fwd", false)
	Radix4SSE2(f, "radix4StageSSE2Inv", true)
	for _, r := range []int{2, 3, 4, 5, 8} {
		k.Pass(f, fmt.Sprintf("skPass%dAVX2", r), r)
	}
	for _, r := range []int{2, 3, 4, 5, 8} {
		k.Last(f, fmt.Sprintf("skLast%dAVX2", r), r)
	}
	for _, r := range []int{2, 4, 8} {
		k.Pass512(f, fmt.Sprintf("skPass%dAVX512", r), r)
		k.Last512(f, fmt.Sprintf("skLast%dAVX512", r), r)
	}
	for _, r := range []int{2, 3, 4, 5, 8} {
		k.Batch(f, fmt.Sprintf("skBatch%dAVX2", r), r)
	}
	k.Pass(f, "skPass16AVX2", 16)
	k.Last(f, "skLast16AVX2", 16)
	k.Pass(f, "skPass12AVX2", 12)
	k.Last(f, "skLast12AVX2", 12)
	for _, r := range []int{2, 4, 8} {
		k.LastRun(f, fmt.Sprintf("skLastRun%dAVX2", r), r)
		k.LastRun512(f, fmt.Sprintf("skLastRun%dAVX512", r), r)
	}
	for _, r := range []int{4, 8} {
		k.Strided(f, fmt.Sprintf("skStrided%dAVX2", r), r)
		k.Strided512(f, fmt.Sprintf("skStrided%dAVX512", r), r)
	}
	files["butterfly_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	f.Add(asm.FeatureProbe("supportsAVX2", asm.AVX2))
	f.Add(asm.FeatureProbe("supportsAVX512F", asm.AVX512F))
	f.Add(asm.VendorProbe("isGenuineIntel", "GenuineIntel"))
	files["cpu_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	for _, r := range []int{2, 3, 4, 5, 8} {
		k.Pass32(f, fmt.Sprintf("sk32Pass%dAVX2", r), r)
	}
	for _, r := range []int{2, 3, 4, 5, 8} {
		k.Last32(f, fmt.Sprintf("sk32Last%dAVX2", r), r)
	}
	files["stockham32_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	for _, r := range []int{4, 8, 3, 5} {
		forSplits(func(inS, outS, inverse bool) {
			k.Split(f, splitName("skSplit", r, inS, outS, inverse), r, inS, outS, inverse)
		})
	}
	files["stockhamsplit_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	k.Untangle32(f, "f32rUntangleAVX2")
	k.Retangle32(f, "f32rRetangleAVX2")
	files["untangle32_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	for _, r := range []int{2, 3, 4, 5, 8} {
		k.Batch32(f, fmt.Sprintf("sk32Batch%dAVX2", r), r)
	}
	files["batch32_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	for _, r := range []int{10, 15, 20} {
		k.Pass(f, fmt.Sprintf("skPass%dAVX2", r), r)
		k.Last(f, fmt.Sprintf("skLast%dAVX2", r), r)
	}
	files["comp2_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	for _, r := range []int{4, 8} {
		forSplits(func(inS, outS, inverse bool) {
			k.Split512(f, splitName("intelSplit512R", r, inS, outS, inverse), r, inS, outS, inverse)
		})
	}
	files["stockhamsplit512_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	k.Untangle(f, "smallUntangleAVX2", false)
	k.Untangle(f, "smallRetangleAVX2", true)
	k.Untangle512(f, "clUntangleAVX512", "smallUntangleAVX2", false)
	k.Untangle512(f, "clRetangleAVX512", "smallRetangleAVX2", true)
	files["untangle_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	TwoPass(f, "znTwoPassAVX2", 256)
	ThreePass(f, "znThreePassAVX2", 512)
	AddrTable(f, "znKernelAddrs", goFFTAddrs())
	files["zntwopass_amd64.s"] = Funcdata(f.String())

	f = emit.NewFile("amd64")
	k.Pass(f, "csPass9AVX2", 9)
	k.Last(f, "csLast9AVX2", 9)
	files["radix9_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	CMulAVX2(f, "zmCMulAVX2")
	files["zmcmul_amd64.s"] = f.String()

	// The FMA twins, as go-fft's Round 41 branch generates them.
	f = emit.NewFile("amd64")
	for _, r := range []int{2, 3, 4, 5, 8} {
		kf.Pass(f, fmt.Sprintf("skPass%dAVX2FMA", r), r)
	}
	for _, r := range []int{2, 4, 8} {
		kf.Pass512(f, fmt.Sprintf("skPass%dAVX512FMA", r), r)
	}
	for _, r := range []int{2, 3, 4, 5, 8} {
		kf.Batch(f, fmt.Sprintf("skBatch%dAVX2FMA", r), r)
	}
	kf.Pass(f, "skPass16AVX2FMA", 16)
	kf.Pass(f, "skPass12AVX2FMA", 12)
	for _, r := range []int{4, 8} {
		kf.Strided(f, fmt.Sprintf("skStrided%dAVX2FMA", r), r)
		kf.Strided512(f, fmt.Sprintf("skStrided%dAVX512FMA", r), r)
	}
	for _, r := range []int{10, 15, 20} {
		kf.Pass(f, fmt.Sprintf("skPass%dAVX2FMA", r), r)
	}
	kf.Pass(f, "csPass9AVX2FMA", 9)
	kf.Last(f, "skLast8AVX2FMA", 8)
	kf.Last512(f, "skLast8AVX512FMA", 8)
	kf.Last(f, "skLast16AVX2FMA", 16)
	kf.Last(f, "csLast9AVX2FMA", 9)
	kf.LastRun(f, "skLastRun8AVX2FMA", 8)
	kf.LastRun512(f, "skLastRun8AVX512FMA", 8)
	files["fma_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	for _, r := range []int{4, 8, 3, 5} {
		forSplits(func(inS, outS, inverse bool) {
			kf.Split(f, splitName("skSplit", r, inS, outS, inverse)+"FMA", r, inS, outS, inverse)
		})
	}
	files["fmasplit_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	for _, r := range []int{4, 8} {
		forSplits(func(inS, outS, inverse bool) {
			kf.Split512(f, splitName("intelSplit512R", r, inS, outS, inverse)+"FMA", r, inS, outS, inverse)
		})
	}
	files["fmasplit512_amd64.s"] = f.String()

	// The radix-32 kernels of the FMA plans (go-fft's Round 42).
	f = emit.NewFile("amd64")
	forSplits(func(inS, outS, inverse bool) {
		kf.SplitR32(f, splitName("skSplit", 32, inS, outS, inverse)+"FMA", false, inS, outS, inverse)
	})
	forSplits(func(inS, outS, inverse bool) {
		kf.SplitR32(f, splitName("intelSplit512R", 32, inS, outS, inverse)+"FMA", true, inS, outS, inverse)
	})
	kf.LastR32(f, "skLast32AVX2FMA", false)
	kf.LastR32(f, "skLast32AVX512FMA", true)
	files["r32fma_amd64.s"] = f.String()

	f = emit.NewFile("amd64")
	forSplits(func(inS, outS, inverse bool) {
		kf.SplitR32InFrame(f, splitName("skSplit", 32, inS, outS, inverse)+"FMAF", false, inS, outS, inverse)
	})
	forSplits(func(inS, outS, inverse bool) {
		kf.SplitR32InFrame(f, splitName("intelSplit512R", 32, inS, outS, inverse)+"FMAF", true, inS, outS, inverse)
	})
	files["r32frame_amd64.s"] = Funcdata(f.String())
	return files
}

// goFFTAddrs is go-fft's znKernelAddrs table (without its FMA half, which
// lives on its unmerged Round 41 branch).
func goFFTAddrs() []Addr {
	var t []Addr
	put := func(sym string, slot int) { t = append(t, Addr{Sym: sym, Slot: slot}) }
	modes := []string{1: "IS", 2: "SS", 3: "SI", 4: "II"}
	for _, r := range []int{2, 3, 4, 5, 8, 9, 10, 12, 15, 16, 20} {
		pre := "sk"
		if r == 9 {
			pre = "cs"
		}
		put(fmt.Sprintf("%sPass%dAVX2", pre, r), r)
		put(fmt.Sprintf("%sLast%dAVX2", pre, r), 21+r)
	}
	for d, dir := range []string{"Fwd", "Inv"} {
		for _, r := range []int{4, 8, 3, 5} {
			for m := 1; m <= 4; m++ {
				put(fmt.Sprintf("skSplit%d%s%s", r, modes[m], dir), 42+(9*d+r)*5+m)
			}
		}
	}
	for d, dir := range []string{"Fwd", "Inv"} {
		for _, r := range []int{4, 8} {
			for m := 1; m <= 4; m++ {
				put(fmt.Sprintf("intelSplit512R%d%s%s", r, modes[m], dir), 132+(9*d+r)*5+m)
			}
		}
	}
	for _, r := range []int{2, 4, 8} {
		put(fmt.Sprintf("skLast%dAVX512", r), 222+r)
	}
	return t
}
