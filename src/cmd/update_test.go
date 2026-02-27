package cmd

import (
	"strings"
	"testing"
)

func TestExpectedArchiveName(t *testing.T) {
	got := expectedArchiveName("v0.1.9", "linux", "amd64")
	want := "axon_0.1.9_linux_amd64.tar.gz"
	if got != want {
		t.Fatalf("expectedArchiveName mismatch: got %q want %q", got, want)
	}

	got = expectedArchiveName("v0.1.9", "windows", "amd64")
	want = "axon_0.1.9_windows_amd64.zip"
	if got != want {
		t.Fatalf("expectedArchiveName mismatch: got %q want %q", got, want)
	}
}

func TestNormalizeReleaseVersion(t *testing.T) {
	if got := normalizeReleaseVersion("v0.1.9"); got != "0.1.9" {
		t.Fatalf("normalizeReleaseVersion mismatch: %q", got)
	}
	if got := normalizeReleaseVersion("0.1.9"); got != "0.1.9" {
		t.Fatalf("normalizeReleaseVersion mismatch: %q", got)
	}
}

func TestParseExpectedSHA256(t *testing.T) {
	manifest := strings.NewReader("" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa axon_0.1.9_linux_amd64.tar.gz\n" +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb checksums.txt\n")

	h, err := parseExpectedSHA256(manifest, "axon_0.1.9_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("parseExpectedSHA256 unexpected error: %v", err)
	}
	if h != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected hash: %s", h)
	}

	missingManifest := strings.NewReader("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc otherfile\n")
	_, err = parseExpectedSHA256(missingManifest, "nope")
	if err == nil {
		t.Fatalf("expected error for missing checksum")
	}
}

func TestSanitizeArchivePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"axon", "axon"},
		{"./axon", "axon"},
		{"dir/axon", "dir/axon"},
		{"../axon", ""},
		{"dir/../axon", ""},
		{"/abs/axon", ""},
		{"dir\\axon", "dir/axon"},
	}
	for _, c := range cases {
		got := sanitizeArchivePath(c.in)
		if got != c.want {
			t.Fatalf("sanitizeArchivePath(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
