package s390x

import "github.com/go-asmgen/asmgen/emit"

// golden is the one kernel, under go-fft's symbol.
var golden = map[string]func(f *emit.File){
	"cmul": func(f *emit.File) { CMulVX(f, "cmulVX") },
}

// goFFTFiles returns go-fft's s390x file, its text as committed.
func goFFTFiles() map[string]string {
	f := emit.NewFile("s390x")
	CMulVX(f, "cmulVX")
	return map[string]string{"cmul_s390x.s": f.String()}
}
