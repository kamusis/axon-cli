# Axon Search Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `axon search` (keyword + semantic vector search with offline fallback) and `axon search --index` (build user semantic index) as described in `docs/axon-search-design.md`.

**Architecture:** Add a new Cobra command `search` that can (1) do keyword search by scanning skill metadata and (2) do semantic search by loading a local index (`skills.jsonl` + `vectors.f32` + `index_manifest.json`) and ranking by cosine similarity. Add an index builder for `--index` that reads skills from the synced repo, generates embeddings via a configurable provider, and writes a local user index atomically.

**Tech Stack:** Go, Cobra CLI, standard library (encoding/json, os, filepath, io), existing axon config paths (`~/.axon`), HTTP client for embeddings provider.

---

## Milestone 0: Baseline discovery (paths, repo layout)

### Task 0.1: Locate existing config/repo path helpers

**Files:**
- Inspect: `src/internal/config/*`
- Inspect: `src/cmd/sync.go` (or equivalent)

**Step 1: Search for repo/config path code**

Run: `rg -n "\.axon" src/internal src/cmd src/main.go`

Expected: matches for config dir and synced repo dir.

**Step 2: Write down canonical helpers**

- Identify functions that return:
  - Axon config dir (e.g., `~/.axon`)
  - Synced repo dir (e.g., `~/.axon/repo`)

**Step 3: Commit (docs-only note in progress)**

No code changes yet.

---

## Milestone 0.5: Dotenv-based embeddings configuration

### Task 0.5.1: Generate `~/.axon/.env` template during `axon init`

**Files:**
- Modify: `src/cmd/init.go`
- Modify/Inspect: `src/internal/config/*` (path helpers)
- Test: `src/cmd/init_test.go`

**Step 1: Write the failing test**

```go
func TestInitGeneratesDotEnvTemplate(t *testing.T) {
    // Run axon init in a temp home/config dir sandbox
    // Assert ~/.axon/.env exists
    // Assert it contains the three keys with empty values
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL.

**Step 3: Implement minimal template generation**

Write `~/.axon/.env` with:

```text
AXON_EMBEDDINGS_PROVIDER=
AXON_EMBEDDINGS_MODEL=
AXON_EMBEDDINGS_API_KEY=
```

Rules:

- Do not overwrite an existing `.env`.
- If the file already exists, leave it unchanged.

**Step 4: Run tests**

Run: `go test ./...`
Expected: PASS.

**Step 5: Commit**

```bash
git add src/cmd/init.go src/cmd/init_test.go
git commit -m "feat: generate ~/.axon/.env template on axon init"
```

### Task 0.5.2: Load `~/.axon/.env` (with env var override)

**Files:**
- Create: `src/internal/config/dotenv.go`
- Modify: `src/internal/embeddings/provider.go`
- Test: `src/internal/config/dotenv_test.go`

**Step 1: Write the failing test**

```go
func TestDotEnvLoadAndOverride(t *testing.T) {
    // Create a temp .env with values
    // Set a real env var override
    // Expect final config uses env var when set
}
```

**Step 2: Implement dotenv loader**

Option A (preferred): use a small pure-Go dependency such as `github.com/joho/godotenv`.
Option B: implement a minimal parser for `KEY=VALUE`.

**Step 3: Wire config resolution**

- When building embeddings provider config:
  - Load `~/.axon/.env`
  - Read final values with environment variables taking precedence

**Step 4: Run tests**

Run: `go test ./...`
Expected: PASS.

**Step 5: Commit**

```bash
git add src/internal/config/dotenv.go src/internal/config/dotenv_test.go src/internal/embeddings/provider.go
git commit -m "feat: load ~/.axon/.env for embeddings config with env override"
```

---

## Milestone 1: Keyword search (Phase 1)

### Task 1.1: Add `axon search` command skeleton

**Files:**
- Create: `src/cmd/search.go`
- Modify: `src/cmd/root.go` (or wherever commands are registered)

**Step 1: Write the failing test**

**Files:**
- Create: `src/cmd/search_test.go`

Add a test that runs the command with a temporary fake repo containing `skills/*/SKILL.md` and asserts output contains matched skills.

```go
func TestSearchKeywordBasic(t *testing.T) {
    // Setup temp axon repo path with skills
    // Run `axon search "database"`
    // Assert output lists matching skill IDs
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./...`

Expected: FAIL (missing command / missing behavior).

**Step 3: Implement minimal command**

- Add Cobra command `search` with args `<query>`.
- For now, only parse args and print placeholder.

**Step 4: Run tests**

Run: `go test ./...`

Expected: still FAIL (placeholder).

**Step 5: Commit**

```bash
git add src/cmd/search.go src/cmd/search_test.go src/cmd/root.go
git commit -m "feat: add axon search command scaffold"
```

### Task 1.2: Implement skill discovery + metadata parsing

**Files:**
- Modify: `src/cmd/search.go`
- Create: `src/internal/search/skills.go`
- Test: `src/internal/search/skills_test.go`

**Step 1: Write failing tests for discovery**

```go
func TestDiscoverSkillsFindsSkillMD(t *testing.T) {
    // create repo/skills/foo/SKILL.md
    // expect DiscoverSkills returns foo
}
```

**Step 2: Run tests**

Run: `go test ./...`

Expected: FAIL.

**Step 3: Implement discovery**

- `DiscoverSkills(repoRoot string) ([]SkillDoc, error)`
- Read `SKILL.md` minimal fields: `name`, `description`.
  - If SKILL.md uses frontmatter, parse it; otherwise fallback to best-effort extraction.

**Step 4: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/internal/search/skills.go src/internal/search/skills_test.go src/cmd/search.go
git commit -m "feat: discover skills and parse minimal metadata"
```

### Task 1.3: Implement keyword matching + result rendering

**Files:**
- Modify: `src/internal/search/keyword.go`
- Modify: `src/cmd/search.go`
- Test: `src/internal/search/keyword_test.go`

**Step 1: Write failing tests**

```go
func TestKeywordSearchCaseInsensitiveMultiKeyword(t *testing.T) {
    // query: "database perf"
    // expect match when name/description contains both terms
}
```

**Step 2: Implement keyword matcher**

- Split query into tokens (space-separated)
- Case-insensitive
- Require all tokens (AND semantics)

**Step 3: Implement rendering**

- Print top K (default 5)
- Show `id`, `path`, `description` (or snippet)

**Step 4: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/internal/search/keyword.go src/internal/search/keyword_test.go src/cmd/search.go
git commit -m "feat: implement keyword search and output formatting"
```

---

## Milestone 2: Semantic search (Phase 2)

### Task 2.1: Define index file formats + loaders

**Files:**
- Create: `src/internal/search/index/types.go`
- Create: `src/internal/search/index/load.go`
- Test: `src/internal/search/index/load_test.go`

**Step 1: Write failing tests**

- Create temp directory with:
  - `index_manifest.json`
  - `skills.jsonl`
  - `vectors.f32`
- Verify loader returns correct `[]SkillEntry` and vectors shape.

**Step 2: Implement types**

- `Manifest` with fields from design doc.
- `SkillEntry` matching JSONL.

**Step 3: Implement loader**

- Load manifest JSON
- Load skills JSONL
- Load vectors as `[]float32` and validate length:
  - `len(vectors) == len(skills) * dim`

**Step 4: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/internal/search/index
git commit -m "feat: add semantic index manifest/skills/vectors loader"
```

### Task 2.2: Implement cosine similarity ranking

**Files:**
- Create: `src/internal/search/semantic.go`
- Test: `src/internal/search/semantic_test.go`

**Step 1: Write failing tests**

- Given 3 vectors and a query vector, expect ordering.

**Step 2: Implement**

- `CosineSim(a, b []float32) float32`
- Optional normalization if `manifest.normalize`.

**Step 3: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 4: Commit**

```bash
git add src/internal/search/semantic.go src/internal/search/semantic_test.go
git commit -m "feat: implement cosine similarity ranking"
```

### Task 2.3: Add embeddings provider interface (query-time)

**Files:**
- Create: `src/internal/embeddings/provider.go`
- Create: `src/internal/embeddings/openai.go` (initial provider)
- Test: `src/internal/embeddings/provider_test.go`

**Step 1: Write failing tests**

- Provider parses env vars into config.
- Provider returns error if missing key.

**Step 2: Implement provider interface**

```go
type Provider interface {
    ModelID() string
    Embed(ctx context.Context, text string) ([]float32, error)
}
```

**Step 3: Implement one provider**

- Implement OpenAI-compatible embeddings endpoint.
- Parse `AXON_EMBEDDINGS_PROVIDER=openai`, `AXON_EMBEDDINGS_MODEL`, `AXON_EMBEDDINGS_API_KEY`.

**Step 4: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/internal/embeddings
git commit -m "feat: add embeddings provider interface and openai provider"
```

### Task 2.4: Wire semantic search into `axon search`

**Files:**
- Modify: `src/cmd/search.go`
- Modify: `src/internal/search/search.go` (or create)
- Test: `src/cmd/search_test.go`

**Step 1: Write failing integration-style test**

- Provide a fake semantic index on disk.
- Stub embeddings provider to return deterministic query vector.
- Expect semantic results order.

**Step 2: Implement index selection**

- Implement selection precedence:
  - user index `~/.axon/search/`
  - repo index `~/.axon/repo/search/`
- Enforce `model_id` match between provider and selected index.

**Step 3: Implement flags**

- `--semantic`, `--no-fallback`, `--keyword`, `--k`, `--debug`

**Step 4: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/cmd/search.go src/cmd/search_test.go src/internal/search
git commit -m "feat: add semantic search with fallback and flags"
```

---

## Milestone 3: `axon search --index` (user index builder)

### Task 3.1: Implement canonical embedding text generation

**Files:**
- Create: `src/internal/search/index/text.go`
- Test: `src/internal/search/index/text_test.go`

**Step 1: Write failing tests**

- Given `SkillDoc{name, description}` produce stable canonical text.
- Hash is stable.

**Step 2: Implement**

- `CanonicalText(skill SkillDoc) string`
- `TextHash(text string) string` (sha256)

**Step 3: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 4: Commit**

```bash
git add src/internal/search/index/text.go src/internal/search/index/text_test.go
git commit -m "feat: add canonical embedding text and text hash"
```

### Task 3.2: Implement index writer (JSONL + vectors.f32 + manifest)

**Files:**
- Create: `src/internal/search/index/write.go`
- Test: `src/internal/search/index/write_test.go`

**Step 1: Write failing tests**

- Write index to temp dir.
- Re-load it with loader and compare entries.

**Step 2: Implement writer**

- Write `skills.jsonl` in deterministic order.
- Write `vectors.f32` row-major.
- Write `index_manifest.json` with:
  - `hub_revision` (current repo revision if available; otherwise a file hash)
  - `created_at`
  - `model_id`, `dim`, `normalize`

**Step 3: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 4: Commit**

```bash
git add src/internal/search/index/write.go src/internal/search/index/write_test.go
git commit -m "feat: write semantic index artifacts"
```

### Task 3.3: Implement incremental rebuild logic

**Files:**
- Create: `src/internal/search/index/build.go`
- Test: `src/internal/search/index/build_test.go`

**Step 1: Write failing tests**

- Start with existing user index.
- Change one skill’s description.
- Run build, ensure only one embedding call happened.

**Step 2: Implement build**

- Load existing user index if present.
- Map `skill.id -> {text_hash, vector}`.
- For each current skill:
  - if unchanged and same `hub_revision` and no `--force`, reuse.
  - else call provider `Embed`.

**Step 3: Atomic swap**

- Build to temp dir under `~/.axon/`.
- Rename swap into `~/.axon/search/`.

**Step 4: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/internal/search/index/build.go src/internal/search/index/build_test.go
git commit -m "feat: build user semantic index incrementally with atomic swap"
```

### Task 3.4: Wire `axon search --index` command

**Files:**
- Modify: `src/cmd/search.go`
- Test: `src/cmd/search_test.go`

**Step 1: Write failing test**

- Run `axon search --index` with stub provider.
- Assert files exist under `~/.axon/search/` in temp sandbox.

**Step 2: Implement**

- Enforce embeddings config presence.
- Support `--force`.
- Print summary output (count indexed, reused, embedded).

**Step 3: Run tests**

Run: `go test ./...`

Expected: PASS.

**Step 4: Commit**

```bash
git add src/cmd/search.go src/cmd/search_test.go
git commit -m "feat: implement axon search --index"
```

---

## Milestone 4: Polish + docs + manual validation

### Task 4.1: Add `--debug` diagnostics + error messages

**Files:**
- Modify: `src/cmd/search.go`
- Modify: `src/internal/search/*`

**Step 1: Add debug output**

- Which index selected (user vs repo)
- Why semantic was skipped (missing key, model mismatch, missing index)

**Step 2: Tests**

- Update tests to assert debug info when flag set.

**Step 3: Commit**

```bash
git add src/cmd/search.go src/internal/search
git commit -m "feat: improve search diagnostics and debug output"
```

### Task 4.2: Manual test script

**Files:**
- Modify: `README.md` (optional) or `docs/NEW_FEATURES.md`

**Step 1: Document usage**

- `axon search "..."`
- `axon search --index`
- env vars required

**Step 2: Commit**

```bash
git add README.md docs/NEW_FEATURES.md
git commit -m "docs: document axon search and indexing"
```

---

## Execution options

Plan complete and saved to `docs/plans/2026-02-28-axon-search.md`. Two execution options:

1. **Subagent-Driven (this session)** - I dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?
