// Package gitutil provides Git-related utility functions.
package gitutil

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Version holds parsed Git version components.
type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string // original version string (e.g., "2.28.0")
}

// ParseVersion parses git version output into structured components.
// Accepts output from "git --version" (e.g., "git version 2.28.0").
func ParseVersion(versionOutput string) (Version, error) {
	fields := strings.Fields(versionOutput)
	if len(fields) < 3 {
		return Version{}, fmt.Errorf("unexpected git version output: %q", strings.TrimSpace(versionOutput))
	}

	raw := fields[2]
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) < 2 {
		return Version{}, fmt.Errorf("cannot parse git version from %q", raw)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("cannot parse major version from %q", parts[0])
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("cannot parse minor version from %q", parts[1])
	}

	patch := 0
	if len(parts) >= 3 {
		// Patch may contain additional suffixes (e.g., "0.windows.1")
		patchStr := parts[2]
		if idx := strings.IndexFunc(patchStr, func(r rune) bool { return r < '0' || r > '9' }); idx != -1 {
			patchStr = patchStr[:idx]
		}
		if patchStr != "" {
			patch, _ = strconv.Atoi(patchStr) // ignore error, default to 0
		}
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Raw:   raw,
	}, nil
}

// AtLeast reports whether this version is greater than or equal to the specified version.
func (v Version) AtLeast(major, minor int) bool {
	if v.Major > major {
		return true
	}
	if v.Major == major && v.Minor >= minor {
		return true
	}
	return false
}

// GetInstalledVersion returns the version of git currently installed.
// Returns an error if git is not available or version cannot be parsed.
func GetInstalledVersion() (Version, error) {
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return Version{}, fmt.Errorf("git is not available: %w", err)
	}
	return ParseVersion(string(out))
}

// CheckMinVersion verifies that the installed git meets the minimum required version.
// Returns nil if satisfied, or a descriptive error otherwise.
func CheckMinVersion(requiredMajor, requiredMinor int) error {
	v, err := GetInstalledVersion()
	if err != nil {
		return err
	}
	if v.AtLeast(requiredMajor, requiredMinor) {
		return nil
	}
	return fmt.Errorf(
		"git %d.%d or later is required (installed: %s)\n"+
			"  Upgrade git from https://git-scm.com and try again.",
		requiredMajor, requiredMinor, v.Raw,
	)
}

// SupportsPartialClone reports whether the installed git supports --filter=blob:none
// (requires git >= 2.28).
func SupportsPartialClone() bool {
	v, err := GetInstalledVersion()
	if err != nil {
		return false
	}
	return v.AtLeast(2, 28)
}
