package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
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
- Suspicious execution patterns (shell injection, eval/exec, base64 obfuscation)
- Data exfiltration (unexpected network calls, IP-based connections)
- Unauthorized file access (credentials, Agent memory files)
- Privilege escalation (sudo, su)
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

	// Check cache if not --force
	var findings []audit.Finding
	var permissions audit.PermissionScope
	var usedCache bool

	if !flagForce {
		// Try to load cached results
		cache, err := audit.LoadAuditResults(target, files)
		if err == nil && cache != nil {
			// Validate cache
			if audit.ValidateCache(cache, files) {
				findings = cache.Findings
				permissions = cache.Permissions
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

			// Audit file — returns FileAuditResult with findings + permissions
			result, err := audit.AuditFile(ctx, provider, file, string(content))
			if err != nil {
				printWarn(file, fmt.Sprintf("Audit failed: %v", err))
				continue
			}

			findings = append(findings, result.Findings...)
			audit.MergePermissions(&permissions, result.Permissions)

			// Progress indicator
			if (i+1)%10 == 0 || i+1 == len(files) {
				fmt.Printf("\r  Progress: %d/%d files scanned", i+1, len(files))
			}
		}
		fmt.Println()
		fmt.Println()

		// Save results to cache
		if err := audit.SaveAuditResults(target, files, findings, permissions); err != nil {
			printWarn("", fmt.Sprintf("Failed to save cache: %v", err))
		}
	}

	// Print structured audit report
	printAuditReport(target, files, findings, permissions)

	// Enter fix mode if requested
	if flagFix && len(findings) > 0 {
		fmt.Println()
		return runFixMode(findings)
	}

	return nil
}

// printAuditReport renders the structured SECURITY AUDIT REPORT to stdout.
func printAuditReport(target string, files []string, findings []audit.Finding, permissions audit.PermissionScope) {
	border := strings.Repeat("═", 47)
	divider := strings.Repeat("─", 47)

	fmt.Println()
	fmt.Println("  SECURITY AUDIT REPORT")
	fmt.Println("  " + border)

	targetLabel := target
	if targetLabel == "" {
		targetLabel = "(entire Hub)"
	}
	fmt.Printf("  Target: %-28s Files: %d\n", targetLabel, len(files))
	fmt.Println("  " + divider)

	// Findings section
	if len(findings) == 0 {
		printOK("", "No issues found.")
		fmt.Println("  " + divider)
	} else {
		fmt.Printf("  RED FLAGS FOUND: %d\n", len(findings))
		fmt.Println()

		// Group by severity order: extreme → high → medium → low
		for _, sev := range []string{"extreme", "high", "medium", "low"} {
			for _, f := range findings {
				if f.Severity != sev {
					continue
				}
				label := fmt.Sprintf("[%s]", strings.ToUpper(f.Severity))
				location := fmt.Sprintf("(L%d) %s", f.LineNumber, f.FilePath)
				fmt.Printf("  • %-10s %s\n", label, location)
				fmt.Printf("    %s\n", f.Description)
				if f.Snippet != "" {
					fmt.Printf("    \"%s\"\n", f.Snippet)
				}
				fmt.Println()
			}
		}
		fmt.Println("  " + divider)
	}

	// Permissions section
	fmt.Println("  PERMISSIONS REQUIRED (estimated):")
	fmt.Printf("  • File Reads  : %s\n", formatList(permissions.FileReads))
	fmt.Printf("  • File Writes : %s\n", formatList(permissions.FileWrites))
	fmt.Printf("  • Network     : %s\n", formatList(permissions.Network))
	fmt.Printf("  • Commands    : %s\n", formatList(permissions.Commands))
	fmt.Println("  " + divider)

	// Risk level and verdict
	riskLevel := audit.ComputeRiskLevel(findings)
	verdict := audit.ComputeVerdict(findings)
	fmt.Printf("  RISK LEVEL: %s\n", riskLevel)
	fmt.Printf("  VERDICT   : %s\n", verdict)
	fmt.Println("  " + border)

	if len(findings) > 0 && !flagFix {
		fmt.Println()
		fmt.Printf("  %d potential issue(s) found. Review manually or run 'axon audit --fix'.\n", len(findings))
	}
}

// formatList formats a string slice for display, returning "None" when empty or when items are just variations of "none".
func formatList(items []string) string {
	if len(items) == 0 {
		return "None"
	}
	
	validItems := make([]string, 0, len(items))
	for _, item := range items {
		clean := strings.TrimSpace(strings.ToLower(item))
		if clean != "" && clean != "none" && clean != "n/a" && clean != "null" {
			validItems = append(validItems, strings.TrimSpace(item))
		}
	}
	
	if len(validItems) == 0 {
		return "None"
	}
	return strings.Join(validItems, ", ")
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
