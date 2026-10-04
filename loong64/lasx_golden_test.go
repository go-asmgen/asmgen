package loong64

// lasxGolden is GNU as 2.43 (Debian, on a Loongson 3C5000L) assembling each
// instruction, read back with objdump: the reference the encoders are held to.
var lasxGolden = []struct {
	op   string
	args []int
	want uint32
}{
	{"xvfmadd.d", []int{0, 0, 0, 0}, 0x0a200000},     // xvfmadd.d $xr0,$xr0,$xr0,$xr0
	{"vfmadd.d", []int{0, 0, 0, 0}, 0x09200000},      // vfmadd.d $vr0,$vr0,$vr0,$vr0
	{"xvfmadd.d", []int{3, 2, 1, 4}, 0x0a220443},     // xvfmadd.d $xr3,$xr2,$xr1,$xr4
	{"vfmadd.d", []int{3, 2, 1, 4}, 0x09220443},      // vfmadd.d $vr3,$vr2,$vr1,$vr4
	{"xvfmadd.d", []int{31, 0, 0, 0}, 0x0a20001f},    // xvfmadd.d $xr31,$xr0,$xr0,$xr0
	{"vfmadd.d", []int{31, 0, 0, 0}, 0x0920001f},     // vfmadd.d $vr31,$vr0,$vr0,$vr0
	{"xvfmadd.d", []int{0, 31, 0, 0}, 0x0a2003e0},    // xvfmadd.d $xr0,$xr31,$xr0,$xr0
	{"vfmadd.d", []int{0, 31, 0, 0}, 0x092003e0},     // vfmadd.d $vr0,$vr31,$vr0,$vr0
	{"xvfmadd.d", []int{0, 0, 31, 0}, 0x0a207c00},    // xvfmadd.d $xr0,$xr0,$xr31,$xr0
	{"vfmadd.d", []int{0, 0, 31, 0}, 0x09207c00},     // vfmadd.d $vr0,$vr0,$vr31,$vr0
	{"xvfmadd.d", []int{0, 0, 0, 31}, 0x0a2f8000},    // xvfmadd.d $xr0,$xr0,$xr0,$xr31
	{"vfmadd.d", []int{0, 0, 0, 31}, 0x092f8000},     // vfmadd.d $vr0,$vr0,$vr0,$vr31
	{"xvfmadd.d", []int{17, 9, 26, 5}, 0x0a22e931},   // xvfmadd.d $xr17,$xr9,$xr26,$xr5
	{"vfmadd.d", []int{17, 9, 26, 5}, 0x0922e931},    // vfmadd.d $vr17,$vr9,$vr26,$vr5
	{"xvfmadd.d", []int{31, 31, 31, 31}, 0x0a2fffff}, // xvfmadd.d $xr31,$xr31,$xr31,$xr31
	{"vfmadd.d", []int{31, 31, 31, 31}, 0x092fffff},  // vfmadd.d $vr31,$vr31,$vr31,$vr31
	{"xvldrepl.d", []int{0, 4, 0}, 0x32100080},       // xvldrepl.d $xr0,$r4,0
	{"vldrepl.d", []int{0, 4, 0}, 0x30100080},        // vldrepl.d $vr0,$r4,0
	{"xvldrepl.d", []int{4, 4, 8}, 0x32100484},       // xvldrepl.d $xr4,$r4,8
	{"vldrepl.d", []int{4, 4, 8}, 0x30100484},        // vldrepl.d $vr4,$r4,8
	{"xvldrepl.d", []int{31, 4, 0}, 0x3210009f},      // xvldrepl.d $xr31,$r4,0
	{"vldrepl.d", []int{31, 4, 0}, 0x3010009f},       // vldrepl.d $vr31,$r4,0
	{"xvldrepl.d", []int{0, 31, 0}, 0x321003e0},      // xvldrepl.d $xr0,$r31,0
	{"vldrepl.d", []int{0, 31, 0}, 0x301003e0},       // vldrepl.d $vr0,$r31,0
	{"xvldrepl.d", []int{7, 12, 2040}, 0x3213fd87},   // xvldrepl.d $xr7,$r12,2040
	{"vldrepl.d", []int{7, 12, 2040}, 0x3013fd87},    // vldrepl.d $vr7,$r12,2040
	{"xvldrepl.d", []int{9, 5, -2048}, 0x321400a9},   // xvldrepl.d $xr9,$r5,-2048
	{"vldrepl.d", []int{9, 5, -2048}, 0x301400a9},    // vldrepl.d $vr9,$r5,-2048
	{"xvldrepl.d", []int{3, 6, -8}, 0x3217fcc3},      // xvldrepl.d $xr3,$r6,-8
	{"vldrepl.d", []int{3, 6, -8}, 0x3017fcc3},       // vldrepl.d $vr3,$r6,-8
	{"xvldrepl.d", []int{31, 31, 2040}, 0x3213ffff},  // xvldrepl.d $xr31,$r31,2040
	{"vldrepl.d", []int{31, 31, 2040}, 0x3013ffff},   // vldrepl.d $vr31,$r31,2040
}
