package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv_NotExist(t *testing.T) {
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })

	m, err := LoadDotEnv()
	if err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
}

func TestLoadDotEnv_ParsesKeyValue(t *testing.T) {
	oldHome := os.Getenv("HOME")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })

	axonDir := filepath.Join(home, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(axonDir, ".env"), []byte("# comment\nA=1\nB=two\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := LoadDotEnv()
	if err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if m["A"] != "1" || m["B"] != "two" {
		t.Fatalf("unexpected map: %v", m)
	}
}

func TestGetConfigValue_EnvOverridesDotEnv(t *testing.T) {
	oldHome := os.Getenv("HOME")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })

	axonDir := filepath.Join(home, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(axonDir, ".env"), []byte("K=fromdotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// env override
	t.Setenv("K", "fromenv")

	v, err := GetConfigValue("K")
	if err != nil {
		t.Fatalf("GetConfigValue: %v", err)
	}
	if v != "fromenv" {
		t.Fatalf("expected env override, got %q", v)
	}
}

func TestEnsureDotEnvTemplate_DoesNotOverwrite(t *testing.T) {
	oldHome := os.Getenv("HOME")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })

	axonDir := filepath.Join(home, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(axonDir, ".env")
	if err := os.WriteFile(p, []byte("AXON_EMBEDDINGS_PROVIDER=keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDotEnvTemplate(); err != nil {
		t.Fatalf("EnsureDotEnvTemplate: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "AXON_EMBEDDINGS_PROVIDER=keep\n" {
		t.Fatalf("template overwrote existing file: %q", string(b))
	}
}

func TestEnsureDotEnvTemplate_CreatesWhenMissing(t *testing.T) {
	oldHome := os.Getenv("HOME")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })

	axonDir := filepath.Join(home, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(axonDir, ".env")

	if err := EnsureDotEnvTemplate(); err != nil {
		t.Fatalf("EnsureDotEnvTemplate: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatalf("expected non-empty template")
	}
}
