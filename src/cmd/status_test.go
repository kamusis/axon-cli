package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
)

func TestTargetAssetCategory(t *testing.T) {
	tests := []struct {
		target config.Target
		want   string
	}{
		{
			target: config.Target{Source: "skills", Type: "directory"},
			want:   "Skills",
		},
		{
			target: config.Target{Source: "workflows", Type: "directory"},
			want:   "Workflows",
		},
		{
			target: config.Target{Source: "commands", Type: "directory"},
			want:   "Commands",
		},
		{
			target: config.Target{Source: "global_rules.md", Type: "file"},
			want:   "Rules",
		},
		{
			target: config.Target{Source: "rules", Type: "directory"},
			want:   "Rules",
		},
		{
			target: config.Target{Source: "notes.md", Type: "file"},
			want:   "Files",
		},
		{
			target: config.Target{Source: "prompts", Type: "directory"},
			want:   "Prompts",
		},
	}

	for _, tt := range tests {
		got := targetAssetCategory(tt.target)
		if got != tt.want {
			t.Errorf("targetAssetCategory(%+v) = %q, want %q", tt.target, got, tt.want)
		}
	}
}

func TestSortCategories(t *testing.T) {
	cats := []string{"Workflows", "Prompts", "Files", "Skills", "Commands", "Rules", "Alpha"}
	sortCategories(cats)

	want := []string{"Skills", "Rules", "Workflows", "Commands", "Files", "Alpha", "Prompts"}
	if len(cats) != len(want) {
		t.Fatalf("len(cats) = %d, want %d", len(cats), len(want))
	}
	for i := range want {
		if cats[i] != want[i] {
			t.Errorf("cats[%d] = %q, want %q", i, cats[i], want[i])
		}
	}
}

func TestFormatTargetList(t *testing.T) {
	names := []string{"apple", "banana", "cherry"}
	out := formatTargetList(names, "  ✓  ", "     ", 20)
	expectedLines := []string{
		"  ✓  apple, banana,",
		"     cherry",
	}
	if out != strings.Join(expectedLines, "\n") {
		t.Errorf("formatTargetList =\n%q\nwant\n%q", out, strings.Join(expectedLines, "\n"))
	}
}

func TestPrintStatusSymlinkHealth_AllHealthy(t *testing.T) {
	temp := t.TempDir()
	hub := filepath.Join(temp, "hub")
	skillsDir := filepath.Join(hub, "skills")
	rulesFile := filepath.Join(hub, "global_rules.md")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesFile, []byte("rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	toolADir := filepath.Join(temp, "toolA")
	toolBDir := filepath.Join(temp, "toolB")
	if err := os.MkdirAll(toolADir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(toolBDir, 0o755); err != nil {
		t.Fatal(err)
	}

	destA := filepath.Join(toolADir, "skills")
	destB := filepath.Join(toolBDir, "rules.md")
	if err := os.Symlink(skillsDir, destA); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rulesFile, destB); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		RepoPath: hub,
		Targets: []config.Target{
			{Name: "tool-a-skills", Source: "skills", Destination: destA, Type: "directory"},
			{Name: "tool-b-rules", Source: "global_rules.md", Destination: destB, Type: "file"},
		},
	}

	var buf bytes.Buffer
	healthy, installed, issues, err := printStatusSymlinkHealth(&buf, cfg)
	if err != nil {
		t.Fatalf("printStatusSymlinkHealth error: %v", err)
	}

	if healthy != 2 || installed != 2 || issues != 0 {
		t.Errorf("healthy=%d, installed=%d, issues=%d, want 2, 2, 0", healthy, installed, issues)
	}

	out := buf.String()
	if !strings.Contains(out, "=== Symlink Health (by Asset) ===") {
		t.Errorf("missing header in output: %s", out)
	}
	if !strings.Contains(out, "● Skills (1 target linked):") {
		t.Errorf("missing Skills section in output: %s", out)
	}
	if !strings.Contains(out, "tool-a-skills") {
		t.Errorf("missing tool-a-skills in output: %s", out)
	}
	if !strings.Contains(out, "● Rules (1 target linked):") {
		t.Errorf("missing Rules section in output: %s", out)
	}
	if !strings.Contains(out, "tool-b-rules") {
		t.Errorf("missing tool-b-rules in output: %s", out)
	}
	if !strings.Contains(out, "2/2 links healthy across 2 categories") {
		t.Errorf("missing summary line in output: %s", out)
	}
}

func TestPrintStatusSymlinkHealth_WithUninstalledAndIssues(t *testing.T) {
	temp := t.TempDir()
	hub := filepath.Join(temp, "hub")
	skillsDir := filepath.Join(hub, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	toolInstalled := filepath.Join(temp, "toolInstalled")
	if err := os.MkdirAll(toolInstalled, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Healthy link
	destHealthy := filepath.Join(toolInstalled, "skills")
	if err := os.Symlink(skillsDir, destHealthy); err != nil {
		t.Fatal(err)
	}

	// 2. Real directory (not yet converted)
	destReal := filepath.Join(toolInstalled, "real-skills")
	if err := os.MkdirAll(destReal, 0o755); err != nil {
		t.Fatal(err)
	}

	// 3. Missing symlink
	destMissing := filepath.Join(toolInstalled, "missing-skills")

	// 4. Uninstalled tool (parent dir does not exist)
	destUninstalled := filepath.Join(temp, "uninstalled-tool", "skills")

	cfg := &config.Config{
		RepoPath: hub,
		Targets: []config.Target{
			{Name: "healthy-skills", Source: "skills", Destination: destHealthy, Type: "directory"},
			{Name: "real-skills", Source: "skills", Destination: destReal, Type: "directory"},
			{Name: "missing-skills", Source: "skills", Destination: destMissing, Type: "directory"},
			{Name: "uninstalled-skills", Source: "skills", Destination: destUninstalled, Type: "directory"},
		},
	}

	var buf bytes.Buffer
	healthy, installed, issues, err := printStatusSymlinkHealth(&buf, cfg)
	if err != nil {
		t.Fatalf("printStatusSymlinkHealth error: %v", err)
	}

	if healthy != 1 || installed != 3 || issues != 2 {
		t.Errorf("healthy=%d, installed=%d, issues=%d, want 1, 3, 2", healthy, installed, issues)
	}

	out := buf.String()
	if !strings.Contains(out, "● Skills (1 linked, 2 issues):") {
		t.Errorf("missing Skills header with issues in output: %s", out)
	}
	if !strings.Contains(out, "healthy-skills") {
		t.Errorf("missing healthy-skills in output: %s", out)
	}
	if !strings.Contains(out, "real directory — run 'axon link real-skills' to convert") {
		t.Errorf("missing real directory remediation in output: %s", out)
	}
	if !strings.Contains(out, "not linked (run: axon link missing-skills)") {
		t.Errorf("missing not linked remediation in output: %s", out)
	}
	if !strings.Contains(out, "● Not Installed Tools (skipped):") {
		t.Errorf("missing Not Installed Tools in output: %s", out)
	}
	if !strings.Contains(out, "uninstalled") {
		t.Errorf("missing uninstalled tool in output: %s", out)
	}
	if !strings.Contains(out, "1/3 links healthy, 2 issues across 1 category (1 tool not installed)") {
		t.Errorf("missing summary line in output: %s", out)
	}
}

func TestPrintStatusVendorHealth_NoneConfigured(t *testing.T) {
	temp := t.TempDir()
	cfg := &config.Config{
		RepoPath: temp,
	}

	var buf bytes.Buffer
	if err := printStatusVendorHealth(&buf, cfg); err != nil {
		t.Fatalf("printStatusVendorHealth error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "=== Hub Assets & Vendors ===") {
		t.Errorf("missing header in output: %s", out)
	}
	if !strings.Contains(out, "Vendors: none configured") {
		t.Errorf("missing none configured in output: %s", out)
	}
}

func TestPrintStatusVendorHealth_AllSynced(t *testing.T) {
	temp := t.TempDir()
	v1Dest := filepath.Join(temp, "skills", "v1")
	v2Dest := filepath.Join(temp, "skills", "v2")
	if err := os.MkdirAll(v1Dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(v2Dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := vendor.WriteProvenance(v1Dest, vendor.Provenance{
		Vendor: "v1",
		Commit: "1234567890abcdef",
	}); err != nil {
		t.Fatal(err)
	}
	if err := vendor.WriteProvenance(v2Dest, vendor.Provenance{
		Vendor: "v2",
		Commit: "abcdef1234567890",
	}); err != nil {
		t.Fatal(err)
	}

	manifestVendors := []config.Vendor{
		{Name: "v1", Dest: "skills/v1"},
		{Name: "v2", Dest: "skills/v2"},
	}
	if err := vendor.WriteManifest(temp, manifestVendors); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		RepoPath: temp,
	}

	var buf bytes.Buffer
	if err := printStatusVendorHealth(&buf, cfg); err != nil {
		t.Fatalf("printStatusVendorHealth error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Vendors: 2 synced (v1@1234567, v2@abcdef1)") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestPrintStatusVendorHealth_WithIssues(t *testing.T) {
	temp := t.TempDir()
	vSyncedDest := filepath.Join(temp, "skills", "v-synced")
	vPendingDest := filepath.Join(temp, "skills", "v-pending")
	if err := os.MkdirAll(vSyncedDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(vPendingDest, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := vendor.WriteProvenance(vSyncedDest, vendor.Provenance{
		Vendor: "v-synced",
		Commit: "abcdef1234567890",
	}); err != nil {
		t.Fatal(err)
	}

	manifestVendors := []config.Vendor{
		{Name: "v-synced", Dest: "skills/v-synced"},
		{Name: "v-missing", Dest: "skills/v-missing"},
		{Name: "v-pending", Dest: "skills/v-pending"},
	}
	if err := vendor.WriteManifest(temp, manifestVendors); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		RepoPath: temp,
	}

	var buf bytes.Buffer
	if err := printStatusVendorHealth(&buf, cfg); err != nil {
		t.Fatalf("printStatusVendorHealth error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Vendors: 1 synced, 1 missing dest, 1 pending") {
		t.Errorf("unexpected summary line in output: %s", out)
	}
	if !strings.Contains(out, "[v-missing] destination missing (run: axon vendor sync v-missing)") {
		t.Errorf("missing v-missing remediation in output: %s", out)
	}
	if !strings.Contains(out, "[v-pending] pending initial sync (run: axon vendor sync v-pending)") {
		t.Errorf("missing v-pending remediation in output: %s", out)
	}
}
