//go:build windows

package opencode

import (
	"fmt"
	"syscall"
)

func lockFile(path string) (func(), error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// 独占打开（shareMode = 0），其他进程再次打开将失败
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, NewSyncError(SyncReasonConflict, fmt.Errorf("同步进行中，请稍后再试 (lock busy)"))
	}

	unlock := func() {
		_ = syscall.CloseHandle(h)
	}
	return unlock, nil
}
