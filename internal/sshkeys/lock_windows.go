//go:build windows

package sshkeys

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	lockFileEx       = kernel32.NewProc("LockFileEx")
	unlockFileEx     = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock = 0x2
)

func lockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	r, _, err := lockFileEx.Call(
		f.Fd(),
		lockfileExclusiveLock,
		0,
		1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r == 0 {
		return err
	}
	return nil
}

func unlockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	r, _, err := unlockFileEx.Call(
		f.Fd(),
		0,
		1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r == 0 {
		return err
	}
	return nil
}
