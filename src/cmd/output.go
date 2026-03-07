package cmd

import (
	"fmt"
	"os"
)

// ── Unified output helpers ────────────────────────────────────────────────────
// All commands use these functions to ensure consistent icon usage and
// indentation throughout axon's CLI output.
//
const (
	iconOK      = "✓" // success / healthy
	iconError   = "✗" // error / failure
	iconWarn    = "⚠" // warning
	iconSkip    = "○" // skipped / not applicable
	iconMiss    = "-" // not found / missing
	iconInfo    = "~" // neutral info / state change
	iconBackup  = "↑" // backup created
	iconRestore = "↓" // backup restored
	iconDir     = "+" // folder / directory
	iconItem    = "·" // file / item (default for list items)
)

// printSection prints a top-level section header, e.g. "=== Link ===".
func printSection(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

// printBullet prints a grouped-section bullet, e.g. "● Already linked:".
func printBullet(title string) {
	fmt.Printf("\n● %s\n", title)
}

// printOK prints a success line.
//
//	name = "" → "  ✓  msg"
//	name set  → "  ✓  [name] msg"
func printOK(name, msg string) {
	if name == "" {
		fmt.Printf("  %s  %s\n", iconOK, msg)
	} else {
		fmt.Printf("  %s  [%s] %s\n", iconOK, name, msg)
	}
}

// printErr prints an error line to stderr.
func printErr(name, msg string) {
	if name == "" {
		fmt.Fprintf(os.Stderr, "  %s  %s\n", iconError, msg)
	} else {
		fmt.Fprintf(os.Stderr, "  %s  [%s] %s\n", iconError, name, msg)
	}
}

// printWarn prints a warning line.
func printWarn(name, msg string) {
	if name == "" {
		fmt.Printf("  %s  %s\n", iconWarn, msg)
	} else {
		fmt.Printf("  %s  [%s] %s\n", iconWarn, name, msg)
	}
}

// printSkip prints a skipped / not-applicable line.
func printSkip(name, msg string) {
	if name == "" {
		fmt.Printf("  %s  %s\n", iconSkip, msg)
	} else {
		fmt.Printf("  %s  [%s] %s\n", iconSkip, name, msg)
	}
}

// printMiss prints a not-found / missing line.
func printMiss(name, msg string) {
	if name == "" {
		fmt.Printf("  %s  %s\n", iconMiss, msg)
	} else {
		fmt.Printf("  %s  [%s] %s\n", iconMiss, name, msg)
	}
}

// printInfo prints a neutral informational / state-change line.
func printInfo(name, msg string) {
	if name == "" {
		fmt.Printf("  %s  %s\n", iconInfo, msg)
	} else {
		fmt.Printf("  %s  [%s] %s\n", iconInfo, name, msg)
	}
}

// printListItem prints a bulleted list item with a custom icon.
func printListItem(icon, name string) {
	fmt.Printf("  %s  %s\n", icon, name)
}
