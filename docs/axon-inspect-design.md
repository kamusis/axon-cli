# Implementation Plan - Polymorphic Axon Inspect

Enhance `axon inspect` to support all categories in the Hub, including directory-based skills and single-file workflows/commands/rules.

## Proposed Changes

### [Component Name] Command Layer (`src/cmd`)

#### [MODIFY] [inspect.go](file:///home/kamus/CascadeProjects/axon-cli/src/cmd/inspect.go)

- **Discovery Logic**: Rename `resolveSkillDirs` to `resolveInspectPaths`.
  - If argument contains `.md`, skip scanning the `skills/` directory completely.
  - Support finding both files and directories.
  - No automatic extension completion (e.g., must type `.md` for files) to remain consistent with `axon status`.
- **UI Logic**: Update `printInspect` to handle different item types (Universal Logic).
  - **Parsing**: Leverage the existing `parseSkillMeta` function which is already frontmatter-compatible. It will be used to read `SKILL.md` (for directories) or the file itself (for standalone items).
  - **Category Identification**: Dynamically determine the item type (Label) by taking the parent directory name and formatting it (e.g., `workflows` -> `Workflow`).
  - **Directory**: Always lookup for `SKILL.md` for metadata, regardless of category.
  - **File**: Always parse frontmatter from the file itself, regardless of category.
  - **Monochrome Icon Matrix**:
    - `skills` -> `⧉`
    - `workflows` -> `≡`
    - `commands` -> `$`
    - `rules` -> `‡`
  - **Custom Folder** -> `◇`
  - **Custom File** -> `⬦`
  - **Labels**: Dynamically determine label (e.g., "Workflow", "Command") from the parent directory name.
  - **Graceful Fallback**: If no frontmatter is found in a file, display `(no metadata found)`. If `description` is missing, skip the `Summary` field.
  - **Context Awareness**: Hide `Files:` and `Scripts:` sections for single-file items.
- **Documentation**: Update `inspectCmd` help strings and examples to reflect support for workflows and rules.

## Output Examples

### Skill Package (Directory)

```text
⧉ Skill Folder: humanizer
Version:  1.0.0
Summary:  Refine AI text to sound more natural

Triggers:
  - content contains "draft"

Path: /home/kamus/.axon/repo/skills/humanizer
```

### Workflow (File)

```text
≡ Workflow: git-pr-comments-analysis
Summary:  Workflow to analyze PR comments and provide structured recommendations

Path: /home/kamus/.axon/repo/workflows/git-pr-comments-analysis.md
```

### Custom Category (Dynamic)

**Case 1: Custom Folder (using `◇` icon)**

```text
◇ My-category: some-tool
Summary:  A custom tool package managed by user

Triggers:
  - keyword "custom"

Path: /home/kamus/.axon/repo/my-category/some-tool
```

**Case 2: Custom File (using `⬦` icon)**

```text
⬦ My-category: some-config
Summary:  Standalone configuration workflow

Path: /home/kamus/.axon/repo/my-category/some-config.md
```

## Verification Plan

### Automated Tests

- Run existing tests: `cd src && go test ./...`
- Manual build check: `cd src && go build ./...`

### Manual Verification

1. **Skill Inspection**: Run `axon inspect humanizer` and verify it shows `⧉ Skill Folder: humanizer`.
2. **Workflow Inspection**: Run `axon inspect git-pr-comments-analysis.md` and verify it shows `≡ Workflow: git-pr-comments-analysis`.
3. **Custom Category**: Create a custom file in a new category and verify it uses the `⬦` icon.
4. **Missing Metadata**: Run inspect on a file without frontmatter and verify the fallback message.
5. **Efficiency Check**: Run `axon inspect non-existent.md` and verify it doesn't waste time scanning `skills/`.
