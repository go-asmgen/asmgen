package riscv64

import "github.com/go-asmgen/asmgen/emit"

// golden is the one kernel, under go-fft's symbol.
var golden = map[string]func(f *emit.File){
	"cmul": func(f *emit.File) { CMulRVV(f, "cmulRVV") },
}

// goFFTFiles returns go-fft's riscv64 file, its text as committed.
func goFFTFiles() map[string]string {
	f := emit.NewFile("riscv64")
	CMulRVV(f, "cmulRVV")
	return map[string]string{"cmul_riscv64.s": f.String()}
}
