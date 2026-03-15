package audit

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamusis/axon-cli/internal/config"
)

// FileInfo stores metadata about a scanned file for cache validation.
type FileInfo struct {
	Path  string `json:"path"`
	MTime int64  `json:"mtime"` // Unix timestamp
	Hash  string `json:"hash"`  // SHA256 of content
}

// AuditCache represents cached audit results.
type AuditCache struct {
	Target      string     `json:"target"`
	Timestamp   time.Time  `json:"timestamp"`
	LLMProvider string     `json:"llm_provider"`
	LLMModel    string     `json:"llm_model"`
	Files       []FileInfo `json:"files"`
	Findings    []Finding  `json:"findings"`
}

// SaveAuditResults saves audit results to cache.
func SaveAuditResults(target string, files []string, findings []Finding) error {
	// Get cache directory
	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Build file info list
	fileInfos := make([]FileInfo, 0, len(files))
	for _, f := range files {
		info, err := buildFileInfo(f)
		if err != nil {
			// Skip files that can't be read
			continue
		}
		fileInfos = append(fileInfos, info)
	}

	// Get LLM config
	provider, _ := config.GetConfigValue("AXON_AUDIT_PROVIDER")
	model, _ := config.GetConfigValue("AXON_AUDIT_MODEL")

	// Build cache object
	cache := AuditCache{
		Target:      target,
		Timestamp:   time.Now(),
		LLMProvider: provider,
		LLMModel:    model,
		Files:       fileInfos,
		Findings:    findings,
	}

	// Generate cache key
	cacheKey := generateCacheKey(target, files)
	cachePath := filepath.Join(cacheDir, cacheKey+".json")

	// Write to file
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// LoadAuditResults loads cached audit results if available and valid.
// Returns nil if cache doesn't exist or is invalid.
func LoadAuditResults(target string, files []string) (*AuditCache, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return nil, err
	}

	cacheKey := generateCacheKey(target, files)
	cachePath := filepath.Join(cacheDir, cacheKey+".json")

	// Check if cache file exists
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	// Parse cache
	var cache AuditCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}

	return &cache, nil
}

// ValidateCache checks if cached results are still valid.
func ValidateCache(cache *AuditCache, files []string) bool {
	if cache == nil {
		return false
	}

	// Build map of cached file info
	cachedFiles := make(map[string]FileInfo)
	for _, f := range cache.Files {
		cachedFiles[f.Path] = f
	}

	// Check if all current files match cache
	for _, path := range files {
		cached, exists := cachedFiles[path]
		if !exists {
			return false // New file not in cache
		}

		// Check if file changed
		current, err := buildFileInfo(path)
		if err != nil {
			return false // Can't read file
		}

		// Compare mtime and hash
		if current.MTime != cached.MTime || current.Hash != cached.Hash {
			return false // File changed
		}
	}

	// Check if cache has extra files (files were deleted)
	if len(cache.Files) != len(files) {
		return false
	}

	return true
}

// getCacheDir returns the audit cache directory path.
func getCacheDir() (string, error) {
	axonDir, err := config.AxonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(axonDir, "audit-results"), nil
}

// generateCacheKey generates a cache key from target and file list.
func generateCacheKey(target string, files []string) string {
	// Use MD5 of target + sorted file list
	h := md5.New()
	h.Write([]byte(target))
	for _, f := range files {
		h.Write([]byte(f))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// buildFileInfo creates FileInfo for a file.
func buildFileInfo(path string) (FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}

	// Read file content for hash
	content, err := os.ReadFile(path)
	if err != nil {
		return FileInfo{}, err
	}

	hash := sha256.Sum256(content)

	return FileInfo{
		Path:  path,
		MTime: info.ModTime().Unix(),
		Hash:  fmt.Sprintf("%x", hash),
	}, nil
}

