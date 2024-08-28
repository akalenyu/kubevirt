package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

func init() {
	// main needs to be locked on one thread and no go routines
	runtime.LockOSThread()
}

func main() {
	PrintMemUsage()

	value := &syscall.Rlimit{
		Cur: uint64(1000000000),
		Max: uint64(1000000000),
	}
	err := syscall.Setrlimit(unix.RLIMIT_AS, value)
	if err != nil {
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "hi")
}

// PrintMemUsage outputs the current, total and OS memory being used. As well as the number
// of garage collection cycles completed.
func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
	fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
	fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
	fmt.Printf("\tLookups = %v\n", m.Lookups)
	fmt.Printf("\tMallocs = %v\n", m.Mallocs)
	fmt.Printf("\tStackSys = %v\n", m.StackSys)
	fmt.Printf("\tStackInUse = %v\n", m.StackInuse)
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
