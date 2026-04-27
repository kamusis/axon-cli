//go:build windows

package cmd

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// windowsSymlinkCapability describes why symlink creation is (or is not) permitted.
type windowsSymlinkCapability struct {
	// Permitted is true if symlinks can be created in the current session.
	Permitted bool
	// ViaDevMode is true when permission comes from Developer Mode rather than
	// Administrator elevation.
	ViaDevMode bool
}

// checkWindowsSymlinkCapability probes whether the current process can create
// symlinks on Windows, and how that capability was granted.
//
// Windows allows symlink creation in two ways:
//  1. The process is running with Administrator (elevated) privileges.
//  2. Developer Mode is enabled (Windows 10 1703+ / Windows 11), which grants
//     unprivileged symlink creation via SeCreateSymbolicLinkPrivilege.
//
// The function first queries the Developer Mode registry key, then falls back
// to an actual os.Symlink probe to determine the final result.
func checkWindowsSymlinkCapability() windowsSymlinkCapability {
	devMode := isWindowsDeveloperModeEnabled()

	// Probe with a real symlink attempt — this is the ground truth regardless
	// of how the capability was acquired.
	permitted := probeSymlink()

	return windowsSymlinkCapability{
		Permitted:  permitted,
		ViaDevMode: permitted && devMode,
	}
}

// isWindowsDeveloperModeEnabled reads the registry key that Windows sets when
// Developer Mode is active.
//
// Key: HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock
// Value: AllowDevelopmentWithoutDevLicense (DWORD) = 1
func isWindowsDeveloperModeEnabled() bool {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return false
	}
	defer k.Close()

	val, _, err := k.GetIntegerValue("AllowDevelopmentWithoutDevLicense")
	if err != nil {
		return false
	}
	return val == 1
}

// probeSymlink creates a temporary file and attempts to symlink it to verify
// that the OS will actually permit symlink creation in the current session.
func probeSymlink() bool {
	tmp := os.TempDir()
	src := filepath.Join(tmp, "axon-probe-src")
	dst := filepath.Join(tmp, "axon-probe-link")

	if err := os.WriteFile(src, []byte("probe"), 0o644); err != nil {
		return false
	}
	defer os.Remove(src)
	defer os.Remove(dst)

	return os.Symlink(src, dst) == nil
}

// windowsSymlinkRemediation is the shared fix guidance shown in both axon link
// (as an error message) and axon doctor (as a Remediation field).
const windowsSymlinkRemediation = "Fix option 1 (recommended): Run axon in an Administrator terminal.\n" +
	"  Fix option 2: Enable Developer Mode in Windows Settings → System → Advanced → For developers."

// windowsSymlinkPreflight returns an error if the current Windows session
// cannot create symlinks.  It is a no-op on non-Windows platforms (see
// symlink_check_other.go).
func windowsSymlinkPreflight() error {
	cap := checkWindowsSymlinkCapability()
	if cap.Permitted {
		return nil
	}
	// Neither Admin elevation nor Developer Mode grants permission.
	return &windowsSymlinkError{}
}

// windowsSymlinkError is a structured error for symlink permission failures.
type windowsSymlinkError struct{}

func (e *windowsSymlinkError) Error() string {
	return "symlink creation is not permitted in this Windows session.\n  " +
		windowsSymlinkRemediation
}

// checkWindowsSymlink returns a DiagnosticResult slice for the doctor command,
// describing whether symlink creation is permitted and how (Admin or Dev Mode).
func checkWindowsSymlink() []DiagnosticResult {
	cat := "Windows symlink permission"
	cap := checkWindowsSymlinkCapability()
	if !cap.Permitted {
		return []DiagnosticResult{{
			Category:    cat,
			Passed:      false,
			Message:     "Symlink creation is not permitted in this Windows session",
			Remediation: windowsSymlinkRemediation,
		}}
	}
	if cap.ViaDevMode {
		return []DiagnosticResult{{
			Category: cat,
			Passed:   true,
			Message:  "Symlink creation permitted (Developer Mode is enabled)",
		}}
	}
	return []DiagnosticResult{{
		Category: cat,
		Passed:   true,
		Message:  "Symlink creation permitted (Administrator elevation)",
	}}
}
