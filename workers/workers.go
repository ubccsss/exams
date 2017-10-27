package workers

import "runtime"

// Count is the number of workers desired at a time.
var Count = -1

func init() {
	// We want 4x the number of CPUs for workers or 8 minimum since network
	// requests tend to be blocked on network and not CPU.
	Count = runtime.NumCPU() * 4
	if Count < 8 {
		Count = 8
	}
}
