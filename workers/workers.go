package workers

import "runtime"

// Count is the number of workers desired at a time.
var Count = -1

func init() {
	// We want 2x the number of CPUs for workers.
	Count = runtime.NumCPU() * 2
}
