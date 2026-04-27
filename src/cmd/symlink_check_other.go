//go:build !windows

package cmd

// windowsSymlinkPreflight is a no-op on non-Windows platforms.
func windowsSymlinkPreflight() error { return nil }

// checkWindowsSymlink is a no-op stub on non-Windows platforms.
// The real implementation lives in symlink_check_windows.go.
func checkWindowsSymlink() []DiagnosticResult { return nil }
