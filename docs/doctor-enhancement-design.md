# Enhanced Axon Doctor Design

## 1. Overview

This design document details how the current `axon doctor` command will be enhanced into a comprehensive diagnostic and self-healing suite, according to [Issue #1](https://github.com/kamusis/axon-cli/issues/1). The aim is to surface potential configuration issues proactively and provide seamless remediation logic.

## 2. Current Status Analysis

Based on `src/cmd/doctor.go`, here is our current implementation breakdown regarding Issue #1 items:

### 2.1 Already Implemented

- **Windows Symlink Permissions**: `checkWindowsSymlinkPermission()` probes whether the current process has the required Administrator rights.
- **Auto-remediation of unresolved imports**: `axon doctor --fix` automatically deletes `.conflict-*` files.

### 2.2 Needs Improvement

- **Symlink Integrity Audit**: Currently checks whether the destination exists, is a correct symlink, or is a real directory (hijack check). Needs standardized actionable remediation outputs (`axon link <tool>` to repair).
- **Git Health**: Currently only verifies the existence of `.git`. We need to add checks for blocking problems: detached HEAD states and divergent branches (which prevent pulling/syncing). **Note: General uncommitted changes should NOT be checked.** Information about uncommitted changes belongs in `axon status` because they are a normal part of the development lifecycle, not a "problem" that needs fixing.
- **Permission Sentinel**: The current global Windows check works for basic usage, but we lack specific write/symlink verification at the target link level for both Unix and Windows.
- **`axon doctor --fix` Extension**: Currently only resolves conflicts. It must be extended to auto-remediate other safe fixes, like symlink repairs.
- **Actionable Remediation**: Refactor inline instructions into a structured, copy-pasteable format for every diagnostic module.

### 2.3 Completely Newly Add

- **Binary Dependency Check**: Parse `metadata.requires.bins` in `SKILL.md` of linked skills and cross-check against `$PATH`.

### 2.4 Rejected Features

- **Uncommitted Changes Check**: This was initially proposed under Git Health. However, since developers frequently have uncommitted work inside the Hub during development, complaining about them in `axon doctor` creates noise. This state is strictly rotational and should continue to be exclusively surfaced via `axon status`.

## 3. Proposed Solution & Architecture Options

### 3.1 Refactoring Doctor Diagnostics into Modules

Instead of a single heavy function, `axon doctor` will execute independent diagnostic modules. Each module returns a structured outcome:

```go
type DiagnosticResult struct {
	Name        string
	Passed      bool
	ErrorMsg    string
	Remediation string
	CanFix      bool
	FixAction   func() error
}
```

### 3.2 Extending `axon doctor --fix`

By capturing `CanFix` and `FixAction`, running with the `--fix` flag will iterate through failing modules whose fixes are flagged as safe/auto-remediable, and execute `FixAction()`.

### 3.3 Adding the Git Checks

Using `os/exec` to execute:

- `git symbolic-ref -q HEAD` or `git branch --show-current` to check for detached HEAD state.
- Check upstream tracking status for diverged branches (e.g., `git rev-list @..@{u}` and `git rev-list @{u}..@`).

### 3.4 Adding Binary Dependency Parsing

Parse `SKILL.md` from the Hub recursively across linked targets. Extract the YAML frontmatter, decode `requires.bins` arrays, and iterate calling `exec.LookPath(binName)`.

## 4. Implementation Steps

1. Refactor `runDoctor` interface to aggregate individual diagnostic modules (Symlink, Git, Windows Permissions, Binary Dependencies).
2. Write functions for `checkGitHealth(repoPath string)`.
3. Add a YAML unmarshaller for `SKILL.md` frontmatter and `checkSkillDependencies(repoPath, target config.Target)`.
4. Define standard output remediation messages for broken targets.
5. Extend `axon doctor --fix` to invoke `fixSymlink(target)` if symlink audit fails.
