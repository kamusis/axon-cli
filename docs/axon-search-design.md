# Design Doc: `axon search` (Local Keyword + Vector Semantic Search)

## Context / Problem

As users accumulate more skills in the Axon Hub, browsing manually becomes inefficient. Issue #2 proposes a new `axon search` command to find the right skill using natural language.

This document defines a practical phased implementation:

- Phase 1: local keyword search (zero external dependencies)
- Phase 2: vector semantic search using precomputed embeddings synced from the Hub, plus query-time embedding generation (API) with a robust offline fallback to Phase 1

## Goals

- Provide a single command to discover skills by intent:
  - `axon search "how do I analyze database perf?"`
  - `axon search "我想制作一个ppt，哪些skill可用"`
- Keep the client lightweight and portable:
  - **No CGO requirement** for `axon-cli` builds.
- Make indexing efficient and mostly centralized:
  - Hub periodically precomputes embeddings for all skills and publishes index artifacts.
  - Client receives those artifacts via `axon sync`.
- Ensure good UX offline:
  - If query-time embeddings cannot be generated (no network/no key), search still works via keyword/BM25 fallback.

## Non-Goals

- Running a full vector database server locally.
- Advanced ANN indexing (HNSW/IVF). With a few hundred skills, exact similarity over all vectors is fast enough.
- Perfect multilingual semantic retrieval without any network usage (offline embeddings can be considered later).

## Key Design Decisions

### 1) Exact vector search (not ANN)

Given expected scale (typically **hundreds** of skills), Phase 2 will compute similarity by scanning all vectors and ranking by cosine similarity. This is simpler, deterministic, and avoids ANN complexity.

### 2) Split responsibilities: Hub precomputes skill embeddings, client embeds query at runtime

- Skills embeddings are precomputed centrally and distributed via Hub index artifacts.
- Query embedding is computed at search time:
  - Primary path: call an embeddings API once per query.
  - Fallback: keyword/BM25 search.

This minimizes API calls (only the query) and ensures the client remains small.

### 3) Model consistency is mandatory

Skill vectors and query vectors must be produced by the **same embedding model** (same dimension and normalization conventions), or similarity comparisons become meaningless.

The synced index artifacts must include:

- `model_id`
- `dim`
- `normalize` (boolean)
- `index_version`

## CLI / UX

### Commands

- `axon search <query>`
  - Runs semantic search if query embeddings can be produced.
  - Otherwise falls back to keyword search.

- `axon search --index`
  - Builds a **local user semantic index** under `~/.axon/search/` using the user-configured embeddings provider/model.
  - Never writes into the synced Hub repo.
  - If the embeddings provider is not configured, `--index` returns an actionable error.

### Flags (proposed)

- `--k <int>`: number of results to show (default `5`)
- `--semantic`: force semantic search; error if semantic search cannot be performed
- `--keyword`: force keyword/BM25 only
- `--hybrid`: combine semantic + keyword scores (optional later)
- `--debug`: print extra diagnostics (provider used, model id, fallback reason)
- `--force`: rebuild index even if `hub_revision` + `text_hash` indicates no changes

### Output format

Example:

```
axon search "database performance"

Results (3 found):
  1. db-advisor        skills/db-advisor/        — Analyze slow queries and index usage
  2. sql-explain       skills/sql-explain/       — Generate EXPLAIN plans for queries
  3. pg-tuning         skills/pg-tuning/         — PostgreSQL performance tuning guide
```

## Data Sources

### Skill content

Indexing uses the Hub repository content:

- `skills/*/SKILL.md`

For each skill, the indexed text should minimally include:

- Name
- Description

Optionally include:

- A short set of keywords/tags
- A small excerpt of usage/examples

These fields should be concatenated into a single “embedding text” with stable formatting.

## Index Artifacts (synced from Hub)

The semantic index artifacts are shipped with the Hub repo and stored under the synced repo directory.

- Hub repo root (after `axon sync`): `~/.axon/repo/`
- Semantic index directory (in-repo): `~/.axon/repo/search/`

### Recommended layout (applies to both Hub and user indexes)

Both Hub-shipped and user-generated indexes use the same structure:

- `search/index_manifest.json`
- `search/skills.jsonl`
- `search/vectors.f32`

**Hub index location**: `~/.axon/repo/search/` (synced, read-only)
**User index location**: `~/.axon/search/` (generated via `axon search --index`)

The user index MUST NOT write into the synced Hub repo (`~/.axon/repo/`).

Notes:

- The user index manifest must record the same fields as the Hub manifest (`model_id`, `dim`, `normalize`, `hub_revision`, `created_at`).
- `axon sync` updates the repo at `~/.axon/repo/` but does not touch the user index at `~/.axon/search/`.

Precedence rule at query time:

1. If a valid user index exists under `~/.axon/search/` and matches the current Hub `hub_revision` and embedding `model_id`, use it.
2. Otherwise, fall back to the Hub-shipped index under `~/.axon/repo/search/`.

Validity rules:

- A semantic index is valid if:
  - `index_manifest.json` exists and parses
  - `skills.jsonl` and `vectors.f32` exist
  - `dim` is consistent with the vector file length and the number of skills

Model matching rules:

- The configured query-time embeddings (`AXON_EMBEDDINGS_PROVIDER` + `AXON_EMBEDDINGS_MODEL`) MUST match the `model_id` of the selected semantic index.
- If they do not match:
  - Default mode (no flags): do not use semantic search; fall back to keyword search.
  - With `--semantic`: return an error describing the mismatch.

### `index_manifest.json` (example)

```json
{
  "index_version": 1,
  "created_at": "2026-02-28T00:00:00Z",
  "hub_revision": "<git-sha-or-tag>",
  "model_id": "<provider>:<model>",
  "dim": 1024,
  "normalize": true,
  "vector_file": "vectors.f32",
  "skills_file": "skills.jsonl"
}
```

### `skills.jsonl`

Each line is a JSON object:

```json
{
  "id": "db-advisor",
  "path": "skills/db-advisor",
  "name": "db-advisor",
  "description": "Analyze slow queries and index usage",
  "text_hash": "<sha256-of-embedding-text>",
  "updated_at": "2026-02-28T00:00:00Z"
}
```

### `vectors.f32`

Binary file storing `float32` vectors packed in row-major order:

- record i occupies `dim * 4` bytes
- skill i corresponds to the i-th line in `skills.jsonl`

This format is trivial to load in pure Go (no CGO) and efficient.

## Search Execution Flow

### Phase 1: Keyword search

Default implementation options:

- Minimal: scan the parsed `name` and `description` for case-insensitive matches.
- Improved: use BM25 via a pure-Go library, or embed Bleve (pure Go) for richer analyzers.

Because Phase 1 is also the offline fallback, it should remain reliable and fast.

### Phase 2: Semantic vector search

Semantic execution:

1. Select a semantic index using the precedence/validity rules defined in the “Index Artifacts” section.
   - If no valid semantic index is found, fall back to Phase 1 keyword search unless `--semantic` is set.
2. Ensure the selected semantic index is compatible with the configured `model_id`.
2. Generate query embedding:
   - If API key is configured and network is available, call embeddings API.
   - Otherwise, trigger fallback to Phase 1 unless `--semantic` is set.
3. Normalize query vector if `manifest.normalize == true`.
4. Compute cosine similarity between query vector and each skill vector.
5. Rank by similarity desc and return TopK.

### Fallback rules

- If semantic search is not possible (missing key, network error, provider error, model mismatch, missing index):
  - Default mode (no flags): print a debug reason when `--debug` is set and run keyword/BM25 search.
  - With `--semantic`: return an error (do not fall back).

## `axon search --index` (User Index Build)

### Purpose

Build a local semantic index using the user-configured embeddings provider/model. This enables semantic search even when the Hub index is absent, outdated, or generated using a different model.

### Inputs

- Source of skills: the synced Hub repo at `~/.axon/repo/` (read-only)
- Skill documents: `~/.axon/repo/skills/*/SKILL.md`
- Embeddings provider configuration:
  - `AXON_EMBEDDINGS_PROVIDER`
  - `AXON_EMBEDDINGS_MODEL`
  - `AXON_EMBEDDINGS_API_KEY`

### Outputs

Write the user index under `~/.axon/search/`:

- `index_manifest.json`
- `skills.jsonl`
- `vectors.f32`

### Invariants

- The `model_id` recorded into the user manifest MUST equal the configured provider+model.

### Incremental behavior

Default behavior is incremental:

- For each skill, compute `text_hash` from the canonical embedding text.
- If an existing user index is present and:
  - the repo `hub_revision` is unchanged, and
  - the skill’s `text_hash` is unchanged,
  then reuse the existing vector without re-embedding.

Rebuild behavior:

- With `--force`, recompute embeddings for all skills.

### Error handling

- If embeddings config is missing, return an error explaining which variables are required.
- If embeddings calls fail mid-run:
  - Default: fail the indexing command with a clear error.
  - The existing user index should remain intact until a full new index is successfully written.

### Atomic write strategy

- Build new artifacts into a temp directory under `~/.axon/`.
- After successful completion, atomically replace `~/.axon/search/` (rename swap).

## Embeddings Provider (Query-time)

Query-time embeddings should be pluggable via configuration.

### Configuration

The embeddings provider is configured using the following keys:

- `AXON_EMBEDDINGS_PROVIDER` (e.g. `openai`, `azure-openai`, `voyage`, etc.)
- `AXON_EMBEDDINGS_MODEL`
- `AXON_EMBEDDINGS_API_KEY`

Configuration sources and precedence:

1. Process environment variables (highest priority)
2. `~/.axon/.env` (loaded by Axon)

`axon init` should generate `~/.axon/.env` as a template with the keys present but empty values, so users can fill them in when they want to use semantic search.

### Multilingual requirement

Because users may query in English or Chinese, the embedding model must be multilingual.

## Hub-side Index Generation (out of scope for axon-cli implementation, but required contract)

This section describes a possible way to ship a precomputed semantic index with the Hub repo, but it is not required to design or implement `axon search` / `axon search --index`.

If a Hub index is provided, it must satisfy:

- Stable ordering between `skills.jsonl` and `vectors.f32`.
- Include `hub_revision` and `created_at`.
- If the embedding model changes, update `model_id` and ideally bump `index_version`.

## Security / Privacy Considerations

- Query text will be sent to the embeddings provider when semantic search is used.
- Skills embeddings are generated on the Hub side; clients do not send skill text to the API.
- API keys must be read from environment or config at runtime and never stored in index artifacts.

## Error Handling

- If index artifacts are missing:
  - Print guidance: run `axon sync`.
- If model mismatch or embeddings call fails:
  - See Fallback rules section above.

## Testing Strategy

- Unit tests:
  - Parsing manifest/skills.jsonl
  - Loading vectors.f32 and mapping to skills
  - Cosine similarity correctness and ranking stability
  - Fallback behavior and error messaging

- Manual tests:
  - With network + API key: semantic search returns relevant results
  - Without network/key: keyword fallback still returns results

## Acceptance Criteria Mapping (Issue #2)

- Phase 1:
  - `axon search <query>` searches `name` + `description` across all skills.
  - Case-insensitive, supports multiple keywords.
  - Results show skill name, path, and matched description snippet.

- Phase 2:
  - Semantic queries return ranked results by relevance.
  - Semantic index is portable across syncs when shipped with the Hub repo.
  - `axon search --index` builds a local user semantic index and does not overwrite the synced repo.

## Open Questions

1. Which embeddings provider/model should be the default for the Hub index?
2. Should `axon-cli` support a local offline embeddings option in the future?
3. Should we introduce a hybrid scoring mode (semantic + BM25) for improved precision?
