package cmd

import (
	"context"
	"fmt"
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

type swapFlags struct {
	pid      int
	current  string
	newPath  string
	backup   string
	expected string
	timeout  time.Duration
}

var selfUpdateSwapCmd = &cobra.Command{
	Use:    "__selfupdate-swap",
	Short:  "(internal) swap axon binary after parent exit",
	Hidden: true,
	RunE:   runSelfUpdateSwap,
}

func init() {
	var f swapFlags
	selfUpdateSwapCmd.Flags().IntVar(&f.pid, "pid", 0, "Parent process ID")
	selfUpdateSwapCmd.Flags().StringVar(&f.current, "current", "", "Current binary path")
	selfUpdateSwapCmd.Flags().StringVar(&f.newPath, "new", "", "New binary path")
	selfUpdateSwapCmd.Flags().StringVar(&f.backup, "backup", "", "Backup binary path")
	selfUpdateSwapCmd.Flags().StringVar(&f.expected, "expected", "", "Expected version")
	selfUpdateSwapCmd.Flags().DurationVar(&f.timeout, "timeout", 30*time.Second, "Timeout")
	selfUpdateSwapCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(context.WithValue(cmd.Context(), swapFlagsKey{}, f))
		return nil
	}
	rootCmd.AddCommand(selfUpdateSwapCmd)
}

type swapFlagsKey struct{}

func runSelfUpdateSwap(cmd *cobra.Command, _ []string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("__selfupdate-swap is only supported on windows")
	}
	f, ok := cmd.Context().Value(swapFlagsKey{}).(swapFlags)
	if !ok {
		return fmt.Errorf("internal error: swap flags missing")
	}
	if f.pid <= 0 || f.current == "" || f.newPath == "" || f.backup == "" || f.expected == "" {
		return fmt.Errorf("invalid arguments")
	}

	lockPath, err := updateLockPath()
	if err != nil {
		return err
	}
	l := flock.New(lockPath)
	locked, err := l.TryLock()
	if err != nil {
		return fmt.Errorf("cannot acquire update lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("another update is in progress (lock: %s)", lockPath)
	}
	defer func() { _ = l.Unlock() }()

	deadline := time.Now().Add(f.timeout)
	for {
		alive := windowsPIDAlive(f.pid)
		if !alive {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for parent pid %d", f.pid)
		}
		time.Sleep(250 * time.Millisecond)
	}

	_ = cleanupBackup(f.backup)
	if err := os.Rename(f.current, f.backup); err != nil {
		return fmt.Errorf("cannot backup current binary: %w", err)
	}
	if err := os.Rename(f.newPath, f.current); err != nil {
		_ = os.Rename(f.backup, f.current)
		return fmt.Errorf("cannot replace binary: %w", err)
	}

	if err := verifyBinaryVersion(f.current, f.expected); err != nil {
		_ = os.Rename(f.current, filepath.Join(filepath.Dir(f.current), "axon.failed.exe"))
		_ = os.Rename(f.backup, f.current)
		return err
	}

	if err := cleanupBackup(f.backup); err != nil {
		printWarn("", fmt.Sprintf("cannot remove backup: %v", err))
	}
	printOK("", "Update applied successfully.")
	return nil
}

func windowsPIDAlive(pid int) bool {
	// Best-effort: use tasklist output to check whether PID is still present.
	out, err := execTasklist(pid)
	if err != nil {
		return false
	}
	return strings.Contains(out, strconv.Itoa(pid))
}

func execTasklist(pid int) (string, error) {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
