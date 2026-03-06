# Axon Rollback Implementation Plan

Implement a new `axon rollback` command (issue #11) that lets users revert a skill or the entire Hub to a previous Git state without needing Git knowledge.

Based on design review: **history/inspection belongs in `axon status`**, not in `rollback`. The rollback command is purely for rollback. `axon status` is extended to accept an optional `[skill-name]` argument that shows skill-level info including recent commit history.

## Proposed Changes

### `cmd` package

#### [NEW] [rollback.go](src/cmd/rollback.go)

A single new file- **[NEW]** `cmd/rollback.go` — implements `rollback` and shared Git/Path helpers.

- **[MODIFY]** `cmd/status.go` — extends `status` to support skill-level inspection.
- **[MODIFY]** `cmd/rollback_test.go` — verification for both commands and path resolution.

**Cobra command structure:**

```bash
axon rollback <skill-name>                       # roll back one skill one commit
axon rollback <skill> --revision <sha-or-tag>    # roll back skill to exact revision
axon rollback --all                              # roll back entire Hub one commit
axon rollback --all --revision <sha-or-tag>      # reset entire Hub to exact revision
```

### `cmd/rollback.go`

- `rollbackCmd`: Definition with `--all` and `--revision` flags.
- `runRollback`: Main entry point with dirty-check and resolution logic.
- `rollbackSkill(repoPath, skillName, revision)`: Reverts a skill by checking out target SHA and committing.
- `rollbackHubAll(repoPath, revision)`: Reverts the Hub using `git revert` (forward revert commits; no history rewrite).
- **`resolveSkillPath(repoPath, name)`**: Checks if `name` exists at root, or in `skills/`, `workflows/`, or `commands/`. Returns the relative path.
- `gitCommitInfo`, `gitLogEntries`: Helpers for formatted Git output.

Flags registered (all on the `rollback` sub-command):
| Flag | Type | Description |
|---|---|---|
| `--all` | bool | Roll back entire Hub (not a single skill) |
| `--revision` | string | Target Git SHA, tag, or branch |

**`runRollback` — argument / flag validation:**

- `--all` and a skill name are mutually exclusive.
- `--revision` is valid with either a skill name **or** `--all`.
- No skill name + no `--all` → error with usage hint.

**`rollbackSkill(repo, skillPath, targetSHA)` helper (core action):**

1. **Safety check**: call `gitIsDirty(repo)` (already in `sync.go`); if dirty, **hard-fail** with an error — `"uncommitted changes in Hub, please commit or stash first"`. No prompt.
2. Find `targetSHA` if not provided:
   - If `--revision` given: use as-is.
   - Default: `git log --skip=1 -1 --format=%H -- <path>` (the commit before HEAD).
3. If no `targetSHA` found → error "no previous version found for skill".
4. Show summary block (matching issue example output exactly):

   ```
   [ Rollback ]
     Skill: <name>
     Current: <current-commit-subject> (<YYYY-MM-DD HH:MM>)
     Target:  <target-commit-subject>  (<YYYY-MM-DD HH:MM>)

     Reverting skills/<name>... DONE
     ✓ Skill '<name>' rolled back one version. Run 'axon sync' to propagate.
   ```

   Timestamps are fetched via `git log --format=%cd --date=format:'%Y-%m-%d %H:%M'`.

5. `git checkout <targetSHA> -- <skillPath>` to restore the tree.
6. `git add <skillPath>` then `git commit -m "axon: rollback <skill> to <sha-short>"`.

**`rollbackAll(repo, targetSHA string)` helper:**

1. **Safety check**: same hard-fail on dirty repo as `rollbackSkill`.
2. `git log --oneline -2` to show current + target.
3. If last commit is NOT an `axon:` commit, warn the user.
4. Create a **forward revert commit** (never rewrite history — sync-safe):
   - Without `--revision`: `git revert HEAD --no-edit` — creates one new commit that undoes HEAD.
   - With `--revision <sha>`: `git revert --no-commit <sha>..HEAD` then
     `git commit -m "axon: rollback hub to <sha-short>"` — stages all intermediate reversals
     as a single squashed commit, restoring the tree to the state at `<sha>`.
5. Print confirmation.

> **Design rationale:** `git reset --hard` is intentionally avoided. It rewrites local history,
> which causes `axon sync` (which runs `git pull --rebase`) to silently re-apply the removed
> commits from origin, undoing the rollback. Using `git revert` keeps history linear and
> forward-only, so sync can push the revert commit to origin and propagate it to other machines.
> Running `rollback --all` twice will cancel out (same as `rollback <skill>` twice); users who
> need to go back N steps should use `--revision <sha>` to target a specific commit.

**Shared helpers (new, private to rollback.go):**

- `gitLogPath(repo, path, skip, n int) ([]logEntry, error)` — parses `git log --format=%H|%ar|%s`.
- `gitCurrentSHA(repo) (string, error)` — `git rev-parse --short HEAD`.

**Output conventions** — reuse existing helpers from `output.go`: `printSection`, `printOK`, `printWarn`, `printInfo`, `printErr`.

---

#### [MODIFY] [status.go](src/cmd/status.go)

Extend `runStatus` to accept an optional positional argument `[skill-name]`.

- **No argument** (existing behaviour unchanged): symlink health + Hub git status.
- **With `skill-name`**: show a focused skill info block. `--fetch` is already a flag on `statusCmd` and applies here too:

  ```
  === Skill: humanizer ===
    Path:    /path/to/hub/skills/humanizer
    Linked:  ✓  (or ✗ not linked)

  ● Recent commits:
    #1  abc1234  2026-03-05 14:20   axon: sync from mac-mini
    #2  def5678  2026-03-04 10:15   axon: sync from vps-1
  ```

  With `--fetch`: after the commit list, add a remote comparison line (reusing the same `git rev-list --left-right --count` logic already in `runStatus`), scoped to commits touching `-- <skillPath>`:

  ```
    Remote: origin/master  (skill ahead 0 / behind 1)
  ```

Implementation: add `Args: cobra.MaximumNArgs(1)` to `statusCmd` and branch at the top of `runStatus` on `len(args) == 1` to call a new private `showSkillStatus(cfg, skillName string, fetchFirst bool) error` helper.

---

#### [NEW] [rollback_test.go](src/cmd/rollback_test.go)

New test file in `cmd/` using the same `initTestRepo(t)` helper from `sync_test.go`.

| Test                                | What it covers                                                                                                   |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `TestRollbackSkill_PreviousVersion` | After two commits to `skills/foo`, `rollbackSkill` restores the first version's content and creates a new commit |
| `TestRollbackSkill_NoHistory`       | Rolling back a skill never touched before returns an error                                                       |
| `TestRollbackAll`                   | After two commits, `rollbackAll` creates a new revert commit; HEAD moves forward by 1 and file content matches the pre-rollback-target state |
| `TestRollbackSkill_WithRevision`    | `--revision HEAD~1` rolls back to the specified SHA                                                              |
| `TestShowSkillStatus`               | After two commits touching a skill path, `showSkillStatus` output contains the skill path and ≥1 commit entry    |

---

## Verification Plan

### Automated Tests

```bash
cd src
go test ./cmd/... -v -run TestRollback
go test ./cmd/... -v -run TestShowSkillStatus
go test ./...
go build ./...
```

### Manual Smoke Test

1. `axon status humanizer` — should show the skill path, link state, and a numbered recent-commit table.
2. `axon rollback humanizer` — should print the `[ Rollback ]` summary block, then `✓ ... rolled back`.
3. `git log --oneline -3` in the Hub repo — should show a new `axon: rollback humanizer to <sha>` commit on top.
4. `axon rollback --all` — should create a new revert commit on top of HEAD; `git log --oneline -3` should show the new `axon: rollback hub to <sha>` commit, and file content should reflect the previous version.
5. `axon rollback humanizer --revision HEAD~2` — should roll back to a specific SHA.
