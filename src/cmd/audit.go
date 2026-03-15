package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kamusis/axon-cli/internal/audit"
	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/llm"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit [target]",
	Short: "Run security audit on Hub content",
	Long: `Scan Hub content for security issues using AI-powered analysis.

Detects:
- Hardcoded secrets (API keys, passwords, tokens)
- Suspicious execution patterns (shell injection, eval/exec)
- Data exfiltration (unexpected network calls)
- PII (emails, phone numbers, addresses)

Examples:
  axon audit                  # scan entire Hub
  axon audit humanizer        # scan a single skill
  axon audit --fix            # interactive fix mode
  axon audit --force          # force re-scan, ignore cache`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAudit,
}

var flagFix bool
var flagForce bool

func init() {
	auditCmd.Flags().BoolVar(&flagFix, "fix", false, "Interactive redaction mode")
	auditCmd.Flags().BoolVar(&flagForce, "force", false, "Force re-scan, ignore cache")
	rootCmd.AddCommand(auditCmd)
}

func runAudit(_ *cobra.Command, args []string) error {
	// Check git availability
	if err := checkGitAvailable(); err != nil {
		return err
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Determine target
	target := ""
	if len(args) > 0 {
		target = args[0]
	}

	// Load LLM provider
	provider, err := llm.LoadProviderFromConfig()
	if err != nil {
		return fmt.Errorf("failed to load LLM provider: %w", err)
	}
	if provider == nil {
		return fmt.Errorf("LLM provider not configured. Please set AXON_AUDIT_PROVIDER, AXON_AUDIT_API_KEY, and AXON_AUDIT_MODEL in ~/.axon/.env")
	}

	// Print header
	if target == "" {
		printSection("Security Audit")
	} else {
		printSection(fmt.Sprintf("Security Audit: %s", target))
	}
	fmt.Println()

	// Print disclaimer
	printWarn("", "AI-powered analysis may produce false positives or miss issues.")
	fmt.Println("      All findings should be manually reviewed before taking action.")
	fmt.Println()

	// Scan files
	files, err := audit.ScanFiles(cfg.RepoPath, target, cfg)
	if err != nil {
		return fmt.Errorf("failed to scan files: %w", err)
	}

	if len(files) == 0 {
		printInfo("", "No files found to scan")
		return nil
	}

	fmt.Printf("  Scanning %d file(s)...\n", len(files))
	fmt.Println()

	// Check cache if --fix and not --force
	var findings []audit.Finding
	var usedCache bool

	if flagFix && !flagForce {
		// Try to load cached results
		cache, err := audit.LoadAuditResults(target, files)
		if err == nil && cache != nil {
			// Validate cache
			if audit.ValidateCache(cache, files) {
				findings = cache.Findings
				usedCache = true

				// Print cache info
				age := time.Since(cache.Timestamp)
				ageStr := formatDuration(age)
				printInfo("", fmt.Sprintf("Using cached audit results from %s (%s ago)",
					cache.Timestamp.Format("2006-01-02 15:04"), ageStr))
				fmt.Println("     Run with --force to re-scan.")
				fmt.Println()
			}
		}
	}

	// If not using cache, run fresh audit
	if !usedCache {
		ctx := context.Background()
		findings = []audit.Finding{}

		for i, file := range files {
			// Read file content
			content, err := os.ReadFile(file)
			if err != nil {
				printWarn(file, fmt.Sprintf("Failed to read: %v", err))
				continue
			}

			// Audit file
			fileFindings, err := audit.AuditFile(ctx, provider, file, string(content))
			if err != nil {
				printWarn(file, fmt.Sprintf("Audit failed: %v", err))
				continue
			}

			findings = append(findings, fileFindings...)

			// Progress indicator
			if (i+1)%10 == 0 || i+1 == len(files) {
				fmt.Printf("\r  Progress: %d/%d files scanned", i+1, len(files))
			}
		}
		fmt.Println()
		fmt.Println()

		// Save results to cache
		if err := audit.SaveAuditResults(target, files, findings); err != nil {
			printWarn("", fmt.Sprintf("Failed to save cache: %v", err))
		}
	}

	// Print findings
	printBullet("Findings")
	fmt.Println()

	if len(findings) == 0 {
		printOK("", "No issues found.")
		return nil
	}

	// Group findings by severity
	highSev := []audit.Finding{}
	mediumSev := []audit.Finding{}
	lowSev := []audit.Finding{}

	for _, f := range findings {
		switch f.Severity {
		case "high":
			highSev = append(highSev, f)
		case "medium":
			mediumSev = append(mediumSev, f)
		case "low":
			lowSev = append(lowSev, f)
		default:
			lowSev = append(lowSev, f)
		}
	}

	// Print findings by severity
	printFindings(highSev)
	printFindings(mediumSev)
	printFindings(lowSev)

	fmt.Println()
	fmt.Printf("  %d potential issue(s) found. Review manually", len(findings))
	if !flagFix {
		fmt.Printf(" or run 'axon audit --fix'.")
	}
	fmt.Println()

	// Enter fix mode if requested
	if flagFix {
		fmt.Println()
		return runFixMode(findings)
	}

	return nil
}

// printFindings prints a list of findings.
func printFindings(findings []audit.Finding) {
	for _, f := range findings {
		location := fmt.Sprintf("%s:%d", f.FilePath, f.LineNumber)
		msg := fmt.Sprintf("%s (%s)", f.Description, f.Severity)
		printWarn(location, msg)
		if f.Snippet != "" {
			fmt.Printf("      \"%s\"\n", f.Snippet)
		}
		fmt.Println()
	}
}

// formatDuration formats a duration in human-readable form.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

// runFixMode enters interactive fix mode.
func runFixMode(findings []audit.Finding) error {
	if len(findings) == 0 {
		printInfo("", "No findings to fix")
		return nil
	}

	stats, err := audit.FixInteractive(findings, os.Stdin, os.Stdout)
	if err != nil {
		return fmt.Errorf("fix mode failed: %w", err)
	}

	// Print final summary
	fmt.Println()
	if stats.Redacted > 0 || stats.Deleted > 0 {
		printOK("", fmt.Sprintf("Fixed %d issue(s) (%d redacted, %d deleted)",
			stats.Redacted+stats.Deleted, stats.Redacted, stats.Deleted))
	}

	return nil
}

