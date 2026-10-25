//go:build windows

// Package proc ties child processes to the lifetime of muxplayer.
package proc

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"muxplayer/internal/diag"
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x2000
	processSetQuota                   = 0x0100
	processTerminate                  = 0x0001
)

// Layouts mirror JOBOBJECT_EXTENDED_LIMIT_INFORMATION; Go's natural struct
// alignment matches the C layout on both 32- and 64-bit Windows.
type ioCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount    uint64
}

type basicLimits struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type extendedLimits struct {
	Basic                 basicLimits
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW   = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJob  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJob = kernel32.NewProc("AssignProcessToJobObject")

	once sync.Once
	job  syscall.Handle // never closed: Windows closes it when muxplayer exits
)

func initJob() {
	h, _, err := procCreateJobObjectW.Call(0, 0)
	if h == 0 {
		diag.Debugf("proc: CreateJobObject failed: %v", err)
		return
	}
	var info extendedLimits
	info.Basic.LimitFlags = jobObjectLimitKillOnJobClose
	r, _, err := procSetInformationJob.Call(h, jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	if r == 0 {
		diag.Debugf("proc: SetInformationJobObject failed: %v", err)
		syscall.CloseHandle(syscall.Handle(h))
		return
	}
	job = syscall.Handle(h)
}

// KillOnExit makes Windows kill cmd's process when muxplayer exits for any
// reason (quit, Ctrl-C, closing the console window, taskkill, crash). Call it
// right after cmd.Start(). Failure is non-fatal: the normal shutdown path
// still kills the child.
func KillOnExit(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	once.Do(initJob)
	if job == 0 {
		return
	}
	h, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(cmd.Process.Pid))
	if err != nil {
		diag.Debugf("proc: OpenProcess(%d) failed: %v", cmd.Process.Pid, err)
		return
	}
	defer syscall.CloseHandle(h)
	if r, _, err := procAssignProcessToJob.Call(uintptr(job), uintptr(h)); r == 0 {
		diag.Debugf("proc: AssignProcessToJobObject failed: %v", err)
	}
}
