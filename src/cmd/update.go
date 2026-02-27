package cmd

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
)

// updateFlags holds flag values for the `axon update` command.
type updateFlags struct {
	check      bool
	dryRun     bool
	repo       string
	prerelease bool
	force      bool
	timeout    time.Duration
	verbose    bool
}

// githubRelease models the subset of GitHub Releases API fields used by axon update.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Draft   bool          `json:"draft"`
	Pre     bool          `json:"prerelease"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset models the subset of GitHub Release Asset fields used by axon update.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the Axon CLI to the latest release",
	RunE:  runUpdate,
}

func init() {
	var f updateFlags
	updateCmd.Flags().BoolVar(&f.check, "check", false, "Check for updates but do not download or install")
	updateCmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Resolve update details but do not download or install")
	updateCmd.Flags().StringVar(&f.repo, "repo", "kamusis/axon-cli", "GitHub repo in owner/name format")
	updateCmd.Flags().BoolVar(&f.prerelease, "prerelease", false, "Allow updating to a prerelease")
	updateCmd.Flags().BoolVar(&f.force, "force", false, "Reinstall even if already on the latest version")
	updateCmd.Flags().DurationVar(&f.timeout, "timeout", 30*time.Second, "Overall timeout for network operations")
	updateCmd.Flags().BoolVar(&f.verbose, "verbose", false, "Verbose output")
	updateCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(context.WithValue(cmd.Context(), updateFlagsKey{}, f))
		return nil
	}
	rootCmd.AddCommand(updateCmd)
}

type updateFlagsKey struct{}

// runUpdate implements the `axon update` command.
func runUpdate(cmd *cobra.Command, _ []string) error {
	f, ok := cmd.Context().Value(updateFlagsKey{}).(updateFlags)
	if !ok {
		return fmt.Errorf("internal error: update flags missing")
	}

	_, unlock, err := acquireUpdateLock(f.timeout)
	if err != nil {
		return err
	}
	defer unlock()

	owner, repo, err := splitRepo(f.repo)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), f.timeout)
	defer cancel()

	rel, err := fetchRelease(ctx, owner, repo, f.prerelease)
	if err != nil {
		return err
	}
	latestTag := strings.TrimSpace(rel.TagName)
	if latestTag == "" {
		return fmt.Errorf("invalid release: empty tag_name")
	}
	latestVersion := normalizeReleaseVersion(latestTag)

	if !f.force && version == latestVersion {
		printOK("", fmt.Sprintf("Axon is up to date: %s", version))
		return nil
	}

	asset, err := selectReleaseAsset(rel, latestTag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	if f.check {
		printInfo("", fmt.Sprintf("Update available: %s -> %s", version, latestTag))
		printInfo("", fmt.Sprintf("Asset: %s", asset.Name))
		return nil
	}
	if f.dryRun {
		printInfo("", fmt.Sprintf("Would update: %s -> %s", version, latestTag))
		printInfo("", fmt.Sprintf("Would download: %s", asset.BrowserDownloadURL))
		return nil
	}

	printInfo("", fmt.Sprintf("Updating: %s -> %s", version, latestTag))

	baseTempDir, err := chooseWritableTempBase()
	if err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(baseTempDir, "axon-update-*")
	if err != nil {
		return fmt.Errorf("cannot create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, asset.Name)
	if err := downloadWithProgress(ctx, asset.BrowserDownloadURL, archivePath, f.verbose); err != nil {
		return err
	}

	checksumAsset, checksumAssetFound := findChecksumAsset(rel)
	if checksumAssetFound {
		expected, expErr := fetchExpectedSHA256(ctx, checksumAsset.BrowserDownloadURL, asset.Name)
		if expErr != nil {
			return expErr
		}
		actual, actErr := fileSHA256Hex(archivePath)
		if actErr != nil {
			return actErr
		}
		if !strings.EqualFold(expected, actual) {
			return fmt.Errorf("checksum mismatch for %s\nexpected: %s\nactual:   %s", asset.Name, expected, actual)
		}
		printOK("", "Checksum verified.")
	} else {
		printWarn("", "checksums.txt not found in release; skipping checksum verification")
	}

	newBinPath := filepath.Join(tmpDir, "axon.new")
	if runtime.GOOS == "windows" {
		newBinPath = filepath.Join(tmpDir, "axon.new.exe")
	}

	if err := extractBinaryFromArchive(archivePath, newBinPath); err != nil {
		return err
	}

	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine current executable path: %w", err)
	}
	currentPath, _ = filepath.EvalSymlinks(currentPath)

	if runtime.GOOS == "windows" {
		stagedNew := filepath.Join(filepath.Dir(currentPath), "axon.new.exe")
		if err := copyFile(newBinPath, stagedNew); err != nil {
			return err
		}
		backupPath := currentPath + ".bak"
		if err := spawnWindowsSwapHelper(currentPath, stagedNew, backupPath, latestVersion, f.timeout); err != nil {
			return err
		}
		printOK("", "Update staged; it will complete after this process exits.")
		return nil
	}

	backupPath := currentPath + ".bak"
	if err := installWithRollback(currentPath, newBinPath, backupPath, latestVersion); err != nil {
		return err
	}
	printOK("", fmt.Sprintf("Updated to %s", latestTag))
	return nil
}

// normalizeReleaseVersion converts a GitHub release tag (e.g. "v0.1.9")
// to the version string embedded in binaries and archive names (e.g. "0.1.9").
func normalizeReleaseVersion(tag string) string {
	tag = strings.TrimSpace(tag)
	if strings.HasPrefix(tag, "v") && len(tag) > 1 {
		return strings.TrimPrefix(tag, "v")
	}
	return tag
}

func splitRepo(s string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid --repo %q (expected owner/name)", s)
	}
	return parts[0], parts[1], nil
}

// fetchRelease retrieves release metadata from GitHub.
func fetchRelease(ctx context.Context, owner, repo string, allowPrerelease bool) (*githubRelease, error) {
	client := &http.Client{}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	if allowPrerelease {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "axon-cli")
	if tok := os.Getenv("AXON_GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	} else if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("github api request failed: %s\n%s", resp.Status, strings.TrimSpace(string(body)))
	}

	if !allowPrerelease {
		var rel githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return nil, fmt.Errorf("cannot decode release response: %w", err)
		}
		return &rel, nil
	}

	var rels []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return nil, fmt.Errorf("cannot decode releases response: %w", err)
	}
	for _, r := range rels {
		if r.Draft {
			continue
		}
		return &r, nil
	}
	return nil, fmt.Errorf("no releases found")
}

// selectReleaseAsset chooses the correct release archive for the current platform.
func selectReleaseAsset(rel *githubRelease, versionTag, goos, goarch string) (*githubAsset, error) {
	expected := expectedArchiveName(versionTag, goos, goarch)
	for _, a := range rel.Assets {
		if a.Name == expected {
			return &a, nil
		}
	}
	var names []string
	for _, a := range rel.Assets {
		names = append(names, a.Name)
	}
	return nil, fmt.Errorf("no suitable release asset found for %s/%s (expected %q). Available: %s", goos, goarch, expected, strings.Join(names, ", "))
}

// expectedArchiveName returns the expected GoReleaser archive filename for the given version and platform.
func expectedArchiveName(versionTag, goos, goarch string) string {
	versionTag = normalizeReleaseVersion(versionTag)
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("axon_%s_%s_%s.%s", versionTag, goos, goarch, ext)
}

// chooseWritableTempBase selects a temp base directory that is very likely to be writable.
func chooseWritableTempBase() (string, error) {
	candidates := []string{os.TempDir()}
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		candidates = append(candidates, filepath.Join(cacheDir, "axon", "tmp"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".axon", "tmp"))
	}

	for _, base := range candidates {
		if base == "" {
			continue
		}
		if err := os.MkdirAll(base, 0o755); err != nil {
			continue
		}
		probe := filepath.Join(base, ".axon-probe-tmp")
		if err := os.WriteFile(probe, []byte(""), 0o644); err != nil {
			continue
		}
		_ = os.Remove(probe)
		return base, nil
	}
	return "", fmt.Errorf("no writable temp directory found")
}

// downloadWithProgress downloads a URL to dest while printing a byte-based progress indicator.
func downloadWithProgress(ctx context.Context, url, dest string, verbose bool) error {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "axon-cli")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("download failed: %s\n%s", resp.Status, strings.TrimSpace(string(body)))
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", dest, err)
	}
	defer out.Close()

	total := resp.ContentLength
	var downloaded int64
	lastPrint := time.Now()
	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write failed: %w", werr)
			}
			downloaded += int64(n)
			if time.Since(lastPrint) > 200*time.Millisecond {
				printDownloadProgress(downloaded, total)
				lastPrint = time.Now()
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return fmt.Errorf("download read failed: %w", rerr)
		}
	}
	printDownloadProgress(downloaded, total)
	fmt.Fprintln(os.Stderr)
	if verbose {
		printInfo("", fmt.Sprintf("Downloaded %d bytes to %s", downloaded, dest))
	}
	return nil
}

// printDownloadProgress renders a single-line progress indicator to stderr.
func printDownloadProgress(downloaded, total int64) {
	if total > 0 {
		pct := float64(downloaded) / float64(total) * 100
		fmt.Fprintf(os.Stderr, "\rDownloading... %s / %s (%.1f%%)", humanBytes(downloaded), humanBytes(total), pct)
		return
	}
	fmt.Fprintf(os.Stderr, "\rDownloading... %s", humanBytes(downloaded))
}

// humanBytes formats a byte count in a human-friendly binary unit.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	prefix := "KMGTPE"[exp]
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), prefix)
}

// findChecksumAsset finds checksums.txt (preferred) or other checksum-like assets in a release.
func findChecksumAsset(rel *githubRelease) (*githubAsset, bool) {
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			return &a, true
		}
	}
	for _, a := range rel.Assets {
		if strings.Contains(strings.ToLower(a.Name), "checksum") {
			return &a, true
		}
	}
	return nil, false
}

// fetchExpectedSHA256 downloads a checksum manifest and extracts the SHA256 for the given filename.
func fetchExpectedSHA256(ctx context.Context, checksumURL, filename string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "axon-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("checksum download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", fmt.Errorf("checksum download failed: %s\n%s", resp.Status, strings.TrimSpace(string(body)))
	}

	return parseExpectedSHA256(resp.Body, filename)
}

// parseExpectedSHA256 parses a checksums manifest stream and returns the SHA256 for filename.
//
// The expected format is whitespace-separated fields where the first field is hex SHA256
// and the last field is the filename, e.g.:
//
//	<sha256> <filename>
func parseExpectedSHA256(r io.Reader, filename string) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		h := fields[0]
		name := fields[len(fields)-1]
		name = strings.TrimPrefix(name, "*")
		if name == filename {
			if _, err := hex.DecodeString(h); err != nil {
				return "", fmt.Errorf("invalid checksum hex for %s", filename)
			}
			return strings.ToLower(h), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("checksum parse failed: %w", err)
	}
	return "", fmt.Errorf("checksum for %s not found", filename)
}

// fileSHA256Hex returns the SHA256 checksum of a file as lowercase hex.
func fileSHA256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinaryFromArchive extracts the axon binary from an archive into destPath.
func extractBinaryFromArchive(archivePath, destPath string) error {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return extractFromTarGz(archivePath, destPath)
	case strings.HasSuffix(lower, ".zip"):
		return extractFromZip(archivePath, destPath)
	default:
		return fmt.Errorf("unsupported archive format: %s", archivePath)
	}
}

// extractFromTarGz extracts the axon binary from a .tar.gz archive.
func extractFromTarGz(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	want := "axon"
	if runtime.GOOS == "windows" {
		want = "axon.exe"
	}

	for {
		h, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		name := sanitizeArchivePath(h.Name)
		if name == "" {
			continue
		}
		if filepath.Base(name) != want {
			continue
		}
		if h.FileInfo().Mode().IsDir() {
			continue
		}
		if err := writeFileFromReader(destPath, tr, 0o755); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("binary %s not found in archive", want)
}

// extractFromZip extracts the axon binary from a .zip archive.
func extractFromZip(archivePath, destPath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	want := "axon"
	if runtime.GOOS == "windows" {
		want = "axon.exe"
	}

	for _, f := range r.File {
		name := sanitizeArchivePath(f.Name)
		if name == "" {
			continue
		}
		if filepath.Base(name) != want {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		if err := writeFileFromReader(destPath, rc, 0o755); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("binary %s not found in archive", want)
}

// sanitizeArchivePath rejects absolute paths and traversal sequences in archive entries.
func sanitizeArchivePath(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "/") {
		return ""
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return ""
		}
	}
	clean := filepath.Clean(name)
	if clean == "." {
		return ""
	}
	return clean
}

// writeFileFromReader writes a file by copying from r and setting mode.
func writeFileFromReader(path string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return nil
}

// installWithRollback replaces currentPath with newPath, verifies the new binary, and rolls back on failure.
func installWithRollback(currentPath, newPath, backupPath, expectedVersion string) error {
	_ = os.Remove(backupPath)
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("cannot create backup: %w", err)
	}
	if err := os.Rename(newPath, currentPath); err != nil {
		_ = os.Rename(backupPath, currentPath)
		return fmt.Errorf("cannot replace binary: %w", err)
	}
	if err := verifyBinaryVersion(currentPath, expectedVersion); err != nil {
		_ = os.Rename(currentPath, currentPath+".failed")
		_ = os.Rename(backupPath, currentPath)
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		printWarn("", fmt.Sprintf("cannot remove backup: %v", err))
	}
	return nil
}

// verifyBinaryVersion executes the binary at path with -v and compares it to expected.
func verifyBinaryVersion(path, expected string) error {
	out, err := exec.Command(path, "-v").Output()
	if err != nil {
		return fmt.Errorf("version verification failed: %w", err)
	}
	got := strings.TrimSpace(string(out))
	if got != expected {
		return fmt.Errorf("version verification failed: expected %s, got %s", expected, got)
	}
	return nil
}

// spawnWindowsSwapHelper starts the internal helper command that swaps binaries after the parent exits.
func spawnWindowsSwapHelper(currentPath, newPath, backupPath, expectedVersion string, timeout time.Duration) error {
	pid := os.Getpid()
	args := []string{"__selfupdate-swap",
		"--pid", strconv.Itoa(pid),
		"--current", currentPath,
		"--new", newPath,
		"--backup", backupPath,
		"--expected", expectedVersion,
		"--timeout", timeout.String(),
	}
	c := exec.Command(currentPath, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Start()
}

// acquireUpdateLock obtains the global update lock for the current user.
func acquireUpdateLock(timeout time.Duration) (*flock.Flock, func(), error) {
	lockPath, err := updateLockPath()
	if err != nil {
		return nil, func() {}, err
	}
	l := flock.New(lockPath)
	deadline := time.Now().Add(timeout)
	for {
		locked, err := l.TryLock()
		if err != nil {
			return nil, func() {}, fmt.Errorf("cannot acquire update lock: %w", err)
		}
		if locked {
			return l, func() { _ = l.Unlock() }, nil
		}
		if time.Now().After(deadline) {
			return nil, func() {}, fmt.Errorf("another update is in progress (lock: %s)", lockPath)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// updateLockPath determines the per-user lock path used to prevent concurrent updates.
func updateLockPath() (string, error) {
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		dir := filepath.Join(cacheDir, "axon")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return filepath.Join(dir, "update.lock"), nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dir := filepath.Join(home, ".axon")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return filepath.Join(dir, "update.lock"), nil
		}
	}
	return "", fmt.Errorf("cannot determine writable lock directory")
}

// copyFile copies a file from src to dst, overwriting dst if it exists.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
