package importer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/importer"
)

func TestImportDir_BasicAndConflict(t *testing.T) {
	tmp := t.TempDir()
	windsurf := filepath.Join(tmp, "windsurf")
	antigravity := filepath.Join(tmp, "antigravity")
	hub := filepath.Join(tmp, "hub")

	for _, d := range []string{windsurf, antigravity, hub} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	excludes := []string{".DS_Store", "Thumbs.db", "*.tmp", "*.bak", "*~"}

	// ── Populate source dirs ──────────────────────────────────────────────────

	// windsurf: oracle V5, common, windsurf_only, junk
	writeFile(t, windsurf, "oracle_expert.md", "V5 Oracle 23ai syntax")
	writeFile(t, windsurf, "common.md", "shared content identical")
	writeFile(t, windsurf, "windsurf_tips.md", "windsurf only")
	writeFile(t, windsurf, ".DS_Store", "junk — must be excluded")

	// antigravity: oracle V1 (conflict), common (identical), ag_only
	writeFile(t, antigravity, "oracle_expert.md", "V1 Oracle basic")
	writeFile(t, antigravity, "common.md", "shared content identical")
	writeFile(t, antigravity, "ag_tips.md", "antigravity only")

	// ── Import windsurf ───────────────────────────────────────────────────────
	r1, err := importer.ImportDir(windsurf, hub, "windsurf", excludes)
	if err != nil {
		t.Fatalf("import windsurf: %v", err)
	}
	if r1.Imported != 3 { // oracle, common, windsurf_tips (.DS_Store excluded)
		t.Errorf("windsurf: want 3 imported, got %d", r1.Imported)
	}
	if r1.Skipped != 0 {
		t.Errorf("windsurf: want 0 skipped, got %d", r1.Skipped)
	}
	if len(r1.Conflicts) != 0 {
		t.Errorf("windsurf: want 0 conflicts, got %d", len(r1.Conflicts))
	}

	// .DS_Store must NOT be in hub.
	if _, err := os.Stat(filepath.Join(hub, ".DS_Store")); !os.IsNotExist(err) {
		t.Error(".DS_Store should have been excluded but exists in hub")
	}

	// ── Import antigravity ────────────────────────────────────────────────────
	r2, err := importer.ImportDir(antigravity, hub, "antigravity", excludes)
	if err != nil {
		t.Fatalf("import antigravity: %v", err)
	}

	// common.md is identical → skipped; oracle_expert.md conflicts; ag_tips.md is new.
	if r2.Skipped != 1 {
		t.Errorf("antigravity: want 1 skipped (common.md), got %d", r2.Skipped)
	}
	if len(r2.Conflicts) != 1 {
		t.Errorf("antigravity: want 1 conflict (oracle_expert.md), got %d", len(r2.Conflicts))
	}

	// Conflict file must follow naming convention.
	conflictPath := filepath.Join(hub, "oracle_expert.conflict-antigravity.md")
	if _, err := os.Stat(conflictPath); os.IsNotExist(err) {
		t.Errorf("conflict file not created: %s", conflictPath)
	}

	// Original V5 must still be intact.
	data, _ := os.ReadFile(filepath.Join(hub, "oracle_expert.md"))
	if string(data) != "V5 Oracle 23ai syntax\n" {
		t.Errorf("original oracle_expert.md was overwritten: %q", string(data))
	}

	// ag_tips.md (new file) must be in hub.
	if _, err := os.Stat(filepath.Join(hub, "ag_tips.md")); os.IsNotExist(err) {
		t.Error("ag_tips.md should have been imported")
	}

	t.Logf("windsurf import: %+v", r1)
	t.Logf("antigravity import: %+v", r2)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
