package audit

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// FixStats tracks statistics for the fix session.
type FixStats struct {
	Total    int
	Redacted int
	Deleted  int
	Skipped  int
}

// FixInteractive enters interactive mode to fix findings.
// For each finding, prompts the user for an action:
//   - r: redact (replace with [REDACTED])
//   - d: delete the entire line
//   - s: skip this finding
//   - q: quit (stop processing)
func FixInteractive(findings []Finding, input io.Reader, output io.Writer) (*FixStats, error) {
	if len(findings) == 0 {
		return &FixStats{}, nil
	}

	stats := &FixStats{Total: len(findings)}
	reader := bufio.NewReader(input)

	fmt.Fprintln(output, "=== Interactive Fix Mode ===")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "For each finding, choose an action:")
	fmt.Fprintln(output, "  r - Redact (replace with [REDACTED])")
	fmt.Fprintln(output, "  d - Delete the entire line")
	fmt.Fprintln(output, "  s - Skip this finding")
	fmt.Fprintln(output, "  q - Quit (stop processing)")
	fmt.Fprintln(output)

	for i, finding := range findings {
		fmt.Fprintf(output, "[%d/%d] %s:%d\n", i+1, len(findings), finding.FilePath, finding.LineNumber)
		fmt.Fprintf(output, "  Issue: %s (%s)\n", finding.Description, finding.Severity)
		if finding.Snippet != "" {
			fmt.Fprintf(output, "  Snippet: \"%s\"\n", finding.Snippet)
		}
		fmt.Fprintln(output)

		// Prompt for action
		fmt.Fprint(output, "Action [r/d/s/q]: ")

		action, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(output, "\nInterrupted")
				break
			}
			return stats, fmt.Errorf("failed to read input: %w", err)
		}

		action = strings.TrimSpace(strings.ToLower(action))

		switch action {
		case "r":
			// Redact
			if err := redactFinding(finding); err != nil {
				fmt.Fprintf(output, "  ✗ Failed to redact: %v\n", err)
			} else {
				fmt.Fprintln(output, "  ✓ Redacted")
				stats.Redacted++
			}
		case "d":
			// Delete line
			if err := deleteLine(finding); err != nil {
				fmt.Fprintf(output, "  ✗ Failed to delete: %v\n", err)
			} else {
				fmt.Fprintln(output, "  ✓ Deleted")
				stats.Deleted++
			}
		case "s":
			// Skip
			fmt.Fprintln(output, "  ○ Skipped")
			stats.Skipped++
		case "q":
			// Quit
			fmt.Fprintln(output, "  Quitting...")
			// Count remaining findings as skipped (including current one)
			stats.Skipped += len(findings) - i
			goto done
		default:
			fmt.Fprintln(output, "  Invalid action, skipping")
			stats.Skipped++
		}

		fmt.Fprintln(output)
	}

done:
	// Print summary
	fmt.Fprintln(output, "=== Summary ===")
	fmt.Fprintf(output, "  Total findings: %d\n", stats.Total)
	fmt.Fprintf(output, "  Redacted: %d\n", stats.Redacted)
	fmt.Fprintf(output, "  Deleted: %d\n", stats.Deleted)
	fmt.Fprintf(output, "  Skipped: %d\n", stats.Skipped)

	return stats, nil
}

// redactFinding replaces the snippet in the file with [REDACTED].
func redactFinding(finding Finding) error {
	// Read file
	content, err := os.ReadFile(finding.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Replace snippet with [REDACTED]
	original := string(content)
	if finding.Snippet == "" {
		return fmt.Errorf("no snippet to redact")
	}

	// Try exact match first
	replaced := strings.Replace(original, finding.Snippet, "[REDACTED]", 1)
	if replaced == original {
		// Try trimmed match
		trimmedSnippet := strings.TrimSpace(finding.Snippet)
		replaced = strings.Replace(original, trimmedSnippet, "[REDACTED]", 1)
		if replaced == original {
			return fmt.Errorf("snippet not found in file")
		}
	}

	// Write back
	if err := os.WriteFile(finding.FilePath, []byte(replaced), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// deleteLine removes the line at the specified line number.
func deleteLine(finding Finding) error {
	if finding.LineNumber <= 0 {
		return fmt.Errorf("invalid line number: %d", finding.LineNumber)
	}

	// Read file
	content, err := os.ReadFile(finding.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Split into lines
	lines := strings.Split(string(content), "\n")

	// Check line number is valid
	if finding.LineNumber > len(lines) {
		return fmt.Errorf("line number %d exceeds file length %d", finding.LineNumber, len(lines))
	}

	// Remove line (convert to 0-indexed)
	lineIdx := finding.LineNumber - 1
	newLines := append(lines[:lineIdx], lines[lineIdx+1:]...)

	// Write back
	newContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(finding.FilePath, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

