package vendor

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CacheRootOverride allows tests to redirect the cache root to a temp directory.
// When non-empty, CacheRoot returns this value instead of ~/.axon/cache/vendors.
var CacheRootOverride string

// CacheRoot returns the absolute path to ~/.axon/cache/vendors/.
func CacheRoot() (string, error) {
	if CacheRootOverride != "" {
		return CacheRootOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".axon", "cache", "vendors"), nil
}

// CachePath returns the cache directory for a vendor entry, derived from the
// repo URL. The path is ~/.axon/cache/vendors/<owner>/<repo> where owner and
// repo are the last two path components of the URL (with any .git suffix stripped).
//
// Examples:
//
//	https://github.com/anthropics/claude-code.git  →  …/vendors/anthropics/claude-code
//	git@github.com:anthropics/claude-code.git      →  …/vendors/anthropics/claude-code
func CachePath(repoURL string) (string, error) {
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	owner, repo, err := parseOwnerRepo(repoURL)
	if err != nil {
		return "", fmt.Errorf("cannot derive cache path from repo URL %q: %w", repoURL, err)
	}
	return filepath.Join(root, owner, repo), nil
}

// parseOwnerRepo extracts the owner and repository name from a Git remote URL.
// Supports HTTPS (https://host/owner/repo[.git]) and SCP-style SSH
// (git@host:owner/repo[.git]) formats.
func parseOwnerRepo(rawURL string) (owner, repo string, err error) {
	// Normalise SCP-style SSH URLs (git@github.com:owner/repo.git) to https form
	// so net/url can parse them.
	normalized := rawURL
	if strings.Contains(rawURL, "@") && !strings.Contains(rawURL, "://") {
		// git@host:path → https://host/path
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) == 2 {
			host := strings.TrimPrefix(parts[0], "git@")
			normalized = "https://" + host + "/" + parts[1]
		}
	}
	u, parseErr := url.Parse(normalized)
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid URL: %w", parseErr)
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 {
		return "", "", fmt.Errorf("expected at least owner/repo in URL path, got %q", u.Path)
	}
	owner = segments[len(segments)-2]
	repo = strings.TrimSuffix(segments[len(segments)-1], ".git")
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("could not extract owner/repo from URL path %q", u.Path)
	}
	return owner, repo, nil
}

// IsCloned reports whether a cache repo directory exists and looks like a git repo.
func IsCloned(cachePath string) bool {
	info, err := os.Stat(filepath.Join(cachePath, ".git"))
	return err == nil && info.IsDir()
}

// Clone clones repoURL into cachePath using sparse-checkout init.
// The repo is cloned with --no-checkout so we can configure sparse-checkout first.
func Clone(repoURL, cachePath string) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("cannot create cache parent dir: %w", err)
	}
	cmd := exec.Command("git", "clone", "--filter=blob:none", "--no-checkout", repoURL, cachePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed for %s: %w", repoURL, err)
	}
	return nil
}

// EnableSparseCheckout configures sparse-checkout cone mode for the cache repo
// and sets it to include only subdir.
func EnableSparseCheckout(cachePath, subdir string) error {
	// Enable sparse-checkout in cone mode.
	init_ := exec.Command("git", "-C", cachePath, "sparse-checkout", "init", "--cone")
	init_.Stdout = os.Stdout
	init_.Stderr = os.Stderr
	if err := init_.Run(); err != nil {
		return fmt.Errorf("git sparse-checkout init failed: %w", err)
	}

	// Set the cone pattern to the requested subdir.
	set := exec.Command("git", "-C", cachePath, "sparse-checkout", "set", subdir)
	set.Stdout = os.Stdout
	set.Stderr = os.Stderr
	if err := set.Run(); err != nil {
		return fmt.Errorf("git sparse-checkout set %s failed: %w", subdir, err)
	}
	return nil
}

// Fetch fetches all refs from the remote for an already-cloned cache repo.
func Fetch(cachePath string) error {
	cmd := exec.Command("git", "-C", cachePath, "fetch", "--tags", "--prune", "origin")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git fetch failed in %s: %w", cachePath, err)
	}
	return nil
}

// Checkout checks out the given ref (branch, tag, or commit SHA) in the cache repo.
// For branch names it uses origin/<ref> to follow the remote branch; for tags and
// SHAs the ref is used directly.
func Checkout(cachePath, ref string) error {
	// Attempt FETCH_HEAD-style: try "origin/<ref>" for branches first.
	remoteRef := "origin/" + ref
	cmd := exec.Command("git", "-C", cachePath, "checkout", "--detach", remoteRef)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Fallback: try the ref directly (tag or commit SHA).
	cmd2 := exec.Command("git", "-C", cachePath, "checkout", "--detach", ref)
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	if err := cmd2.Run(); err != nil {
		return fmt.Errorf("git checkout %s failed in %s: %w", ref, cachePath, err)
	}
	return nil
}

// SourcePath returns the absolute path to subdir inside cachePath and verifies it exists.
func SourcePath(cachePath, subdir string) (string, error) {
	src := filepath.Join(cachePath, filepath.FromSlash(subdir))
	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("subdir %q not found in cache after checkout: %w", subdir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("subdir %q is not a directory in the checked-out tree", subdir)
	}
	return src, nil
}

// ReadVendorSHA returns the commit SHA that was last successfully mirrored for
// the named vendor entry. Returns ("", nil) when no state file exists yet
// (first run or cache wiped).
func ReadVendorSHA(name string) (string, error) {
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(root, name+".sha"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading vendor SHA for %q: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteVendorSHA persists sha as the last-mirrored commit for the named vendor.
// The file is written as <name>.sha directly under the cache root
// (~/.axon/cache/vendors/<name>.sha), alongside the per-repo subdirectories.
func WriteVendorSHA(name, sha string) error {
	root, err := CacheRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("creating cache root: %w", err)
	}
	return os.WriteFile(filepath.Join(root, name+".sha"), []byte(sha+"\n"), 0o644)
}

// AddSparseCheckoutDir adds subdir to the existing sparse-checkout cone for the
// cache repo at cachePath. Idempotent — adding an already-included path is a no-op.
// Use this when a second vendor entry from the same repo needs a different subdir
// than the one configured during the initial clone.
func AddSparseCheckoutDir(cachePath, subdir string) error {
	cmd := exec.Command("git", "-C", cachePath, "sparse-checkout", "add", subdir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git sparse-checkout add %s failed: %w", subdir, err)
	}
	return nil
}

// SubdirLatestSHA returns the latest commit SHA that touched subdir under the
// given gitRef inside cachePath.  gitRef may be any expression git log accepts
// (HEAD, origin/main, a tag, or a commit SHA).
//
// Returns ("", nil) when git log produces no output — this happens on a fresh
// clone where no checkout has been performed yet, or when subdir has never
// been committed.  The caller should treat an empty SHA as "unknown" and
// proceed with the sync rather than skipping it.
func SubdirLatestSHA(cachePath, gitRef, subdir string) (string, error) {
	cmd := exec.Command("git", "-C", cachePath, "log", "-1", "--format=%H", gitRef, "--", subdir)
	out, err := cmd.Output()
	if err != nil {
		// git log may exit non-zero when gitRef doesn't exist yet (fresh clone,
		// tag not yet fetched, etc.). Treat as unknown rather than hard error.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}
