//go:build windows

package cmd

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// cleanupBackup removes backupPath if possible.
//
// On Windows, antivirus/indexers can temporarily hold a handle to the file even
// after the parent process exits; we retry for a short period and fall back to
// scheduling deletion at next reboot.
func cleanupBackup(backupPath string) error {
	if backupPath == "" {
		return nil
	}

	tryRemove := func() error {
		err := os.Remove(backupPath)
		if err == nil {
			return nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var lastErr error
	for i := 0; i < 15; i++ {
		if err := tryRemove(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}

	p, err := windows.UTF16PtrFromString(backupPath)
	if err != nil {
		return lastErr
	}
	if err := windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT); err != nil {
		return lastErr
	}
	return nil
}
