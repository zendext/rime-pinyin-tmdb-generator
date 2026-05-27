//go:build windows

package lock

import (
	"errors"
	"syscall"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION|syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}
	defer syscall.CloseHandle(handle)

	event, err := syscall.WaitForSingleObject(handle, 0)
	if err != nil {
		return true
	}
	return event == syscall.WAIT_TIMEOUT
}
