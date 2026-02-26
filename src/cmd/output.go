package cmd

import (
	"fmt"
	"os"
)

// ── Unified output helpers ────────────────────────────────────────────────────
// All commands use these functions to ensure consistent icon usage and
// indentation throughout axon's CLI output.
//
// Icon semantics:
//   ✓  success / healthy
//   ✗  error / failure          (written to stderr)
//   ⚠  warning
//   ○  skipped / not applicable
//   -  not found / missing
//   ~  neutral info / state change
//   ↑  backup created
//   ↓  backup restored

// printSection prints a top-level section header, e.g. "=== Link ===".
func printSection(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

// printBullet prints a grouped-section bullet, e.g. "● Already linked:".
func printBullet(title string) {
	fmt.Printf("\n● %s\n", title)
}

// printOK prints a success line.
//   name = "" → "  ✓  msg"
//   name set  → "  ✓  [name] msg"
func printOK(name, msg string) {
	if name == "" {
		fmt.Printf("  ✓  %s\n", msg)
	} else {
		fmt.Printf("  ✓  [%s] %s\n", name, msg)
	}
}

// printErr prints an error line to stderr.
func printErr(name, msg string) {
	if name == "" {
		fmt.Fprintf(os.Stderr, "  ✗  %s\n", msg)
	} else {
		fmt.Fprintf(os.Stderr, "  ✗  [%s] %s\n", name, msg)
	}
}

// printWarn prints a warning line.
func printWarn(name, msg string) {
	if name == "" {
		fmt.Printf("  ⚠  %s\n", msg)
	} else {
		fmt.Printf("  ⚠  [%s] %s\n", name, msg)
	}
}

// printSkip prints a skipped / not-applicable line.
func printSkip(name, msg string) {
	if name == "" {
		fmt.Printf("  ○  %s\n", msg)
	} else {
		fmt.Printf("  ○  [%s] %s\n", name, msg)
	}
}

// printMiss prints a not-found / missing line.
func printMiss(name, msg string) {
	if name == "" {
		fmt.Printf("  -  %s\n", msg)
	} else {
		fmt.Printf("  -  [%s] %s\n", name, msg)
	}
}

// printInfo prints a neutral informational / state-change line.
func printInfo(name, msg string) {
	if name == "" {
		fmt.Printf("  ~  %s\n", msg)
	} else {
		fmt.Printf("  ~  [%s] %s\n", name, msg)
	}
}
