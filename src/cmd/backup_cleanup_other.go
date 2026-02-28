//go:build !windows

package cmd

import (
	"errors"
	"os"
)

// cleanupBackup removes backupPath if possible.
func cleanupBackup(backupPath string) error {
	if backupPath == "" {
		return nil
	}
	err := os.Remove(backupPath)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
