package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
	"github.com/spf13/cobra"
)

func TestVendorSyncStatus_Pending(t *testing.T) {
	resetVendorCache(t)

	v := config.Vendor{Name: "never-synced", Dest: "skills/foo"}
	status, err := vendorSyncStatus(t.TempDir(), v)
	if err != nil {
		t.Fatalf("vendorSyncStatus: %v", err)
	}
	if status != "pending" {
		t.Errorf("status = %q, want %q", status, "pending")
	}
}

func TestVendorSyncStatus_Synced(t *testing.T) {
	resetVendorCache(t)

	hubRoot := t.TempDir()
	destAbs := filepath.Join(hubRoot, "skills", "foo")
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		t.Fatal(err)
	}

	v := config.Vendor{Name: "foo-skill", Dest: "skills/foo"}
	if err := vendor.WriteVendorSHA(v.Name, "7d2a8f10abcdef"); err != nil {
		t.Fatal(err)
	}

	status, err := vendorSyncStatus(hubRoot, v)
	if err != nil {
		t.Fatalf("vendorSyncStatus: %v", err)
	}
	if status != "synced (7d2a8f10)" {
		t.Errorf("status = %q, want %q", status, "synced (7d2a8f10)")
	}
}

func TestVendorSyncStatus_Synced_FromProvenance(t *testing.T) {
	resetVendorCache(t) // Cache has NO sha recorded (e.g. on a second machine)

	hubRoot := t.TempDir()
	destAbs := filepath.Join(hubRoot, "skills", "prov-skill")
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := vendor.WriteProvenance(destAbs, vendor.Provenance{
		Vendor: "prov-skill",
		Commit: "1234567890abcdef",
	}); err != nil {
		t.Fatal(err)
	}

	v := config.Vendor{Name: "prov-skill", Dest: "skills/prov-skill"}
	status, err := vendorSyncStatus(hubRoot, v)
	if err != nil {
		t.Fatalf("vendorSyncStatus: %v", err)
	}
	if status != "synced (12345678)" {
		t.Errorf("status = %q, want %q", status, "synced (12345678)")
	}
}


func TestVendorSyncStatus_MissingDest(t *testing.T) {
	resetVendorCache(t)

	hubRoot := t.TempDir()
	// Do NOT create skills/foo — destination was deleted after a prior sync.

	v := config.Vendor{Name: "foo-skill", Dest: "skills/foo"}
	if err := vendor.WriteVendorSHA(v.Name, "7d2a8f10abcdef"); err != nil {
		t.Fatal(err)
	}

	status, err := vendorSyncStatus(hubRoot, v)
	if err != nil {
		t.Fatalf("vendorSyncStatus: %v", err)
	}
	if status != "missing dest" {
		t.Errorf("status = %q, want %q", status, "missing dest")
	}
}

func TestVendorNames(t *testing.T) {
	vendors := []config.Vendor{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := vendorNames(vendors)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("vendorNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("vendorNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCompleteVendorNames_FiltersByPrefix(t *testing.T) {
	// os.UserHomeDir() reads USERPROFILE on Windows and HOME on Unix.
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(key, home)
	}

	axonDir := filepath.Join(home, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "repo_path: " + filepath.Join(home, "hub") + "\nvendors:\n" +
		"  - name: book-to-skill\n    repo: https://example.com/a/b\n    subdir: .\n    dest: skills/book\n" +
		"  - name: cangjie-skill\n    repo: https://example.com/a/c\n    subdir: .\n    dest: skills/cangjie\n"
	if err := os.WriteFile(filepath.Join(axonDir, "axon.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	matches, directive := completeVendorNames(nil, nil, "book")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if len(matches) != 1 || matches[0] != "book-to-skill" {
		t.Errorf("matches = %v, want [book-to-skill]", matches)
	}
}
