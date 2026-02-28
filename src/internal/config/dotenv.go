package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DotEnvPath returns the absolute path to Axon's dotenv file (~/.axon/.env).
func DotEnvPath() (string, error) {
	axonDir, err := AxonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(axonDir, ".env"), nil
}

// LoadDotEnv reads ~/.axon/.env and returns key/value pairs.
//
// Parsing rules:
// - Lines starting with '#' are ignored.
// - Empty lines are ignored.
// - Lines must be of form KEY=VALUE.
// - Whitespace around KEY is trimmed.
// - VALUE is taken as-is (no quote parsing).
func LoadDotEnv() (map[string]string, error) {
	p, err := DotEnvPath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("cannot open dotenv file %s: %w", p, err)
	}
	defer f.Close()

	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := line[i+1:]
		if k == "" {
			continue
		}
		out[k] = v
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot read dotenv file %s: %w", p, err)
	}
	return out, nil
}

// GetConfigValue returns the effective value for key, using process environment variables
// first and falling back to ~/.axon/.env.
func GetConfigValue(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	dotenv, err := LoadDotEnv()
	if err != nil {
		return "", err
	}
	return dotenv[key], nil
}

// EnsureDotEnvTemplate creates ~/.axon/.env if it does not already exist.
//
// The template contains configuration keys with empty values so users can fill
// them in when they want to use embeddings-powered features.
func EnsureDotEnvTemplate() error {
	p, err := DotEnvPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(p); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot stat dotenv file %s: %w", p, err)
	}

	body := "" +
		"AXON_EMBEDDINGS_PROVIDER=\n" +
		"AXON_EMBEDDINGS_MODEL=\n" +
		"AXON_EMBEDDINGS_API_KEY=\n"

	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		return fmt.Errorf("cannot write dotenv template %s: %w", p, err)
	}
	return nil
}
