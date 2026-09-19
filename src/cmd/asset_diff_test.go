package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestClassifyAssetPath(t *testing.T) {
	tests := []struct {
		path         string
		wantCategory string
		wantName     string
	}{
		{"skills/hexo-blog-publish/SKILL.md", "Skills", "hexo-blog-publish"},
		{"skills/hexo-blog-publish/scripts/deploy.sh", "Skills", "hexo-blog-publish"},
		{"skills/agent-reach", "Skills", "agent-reach"},
		{"rules/claude-code-rules", "Rules", "claude-code-rules"},
		{"rules/global.md", "Rules", "global.md"},
		{"workflows/deploy.md", "Workflows", "deploy"},
		{"workflows/git-release/workflow.yaml", "Workflows", "git-release"},
		{"commands/test.md", "Commands", "test"},
		{"axon.yaml", "Config", "axon.yaml"},
		{".gitignore", "Config", ".gitignore"},
		{"README.md", "Other", "README.md"},
		{"custom/nested/file.txt", "Other", "custom/nested/file.txt"},
	}

	for _, tt := range tests {
		cat, name := classifyAssetPath(tt.path)
		if cat != tt.wantCategory || name != tt.wantName {
			t.Errorf("classifyAssetPath(%q) = (%q, %q), want (%q, %q)",
				tt.path, cat, name, tt.wantCategory, tt.wantName)
		}
	}
}

func TestDetermineAssetChangeType(t *testing.T) {
	// All added
	diffsNew := []FileDiff{
		{Status: "A", Path: "skills/foo/SKILL.md"},
		{Status: "A", Path: "skills/foo/run.sh"},
	}
	if got := determineAssetChangeType(diffsNew); got != ChangeNew {
		t.Errorf("got %v, want %v", got, ChangeNew)
	}

	// All deleted
	diffsDel := []FileDiff{
		{Status: "D", Path: "skills/foo/SKILL.md"},
	}
	if got := determineAssetChangeType(diffsDel); got != ChangeDeleted {
		t.Errorf("got %v, want %v", got, ChangeDeleted)
	}

	// Mixed / Modified
	diffsMod := []FileDiff{
		{Status: "M", Path: "skills/foo/SKILL.md"},
		{Status: "A", Path: "skills/foo/extra.txt"},
	}
	if got := determineAssetChangeType(diffsMod); got != ChangeUpdated {
		t.Errorf("got %v, want %v", got, ChangeUpdated)
	}
}

func TestParseDiffNameStatus(t *testing.T) {
	raw := "A\tskills/hexo-blog-publish/SKILL.md\n" +
		"A\tskills/hexo-blog-publish/deploy.sh\n" +
		"M\tskills/agent-reach/SKILL.md\n" +
		"D\trules/old-rules\n" +
		"M\tworkflows/deploy.md\n"

	groups := parseDiffNameStatus(raw)
	if len(groups) != 3 {
		t.Fatalf("expected 3 category groups (Skills, Rules, Workflows), got %d", len(groups))
	}

	if groups[0].Category != "Skills" {
		t.Errorf("expected first category to be Skills, got %s", groups[0].Category)
	}
	if len(groups[0].Assets) != 2 {
		t.Fatalf("expected 2 Skills assets, got %d", len(groups[0].Assets))
	}

	// Skills assets should be sorted alphabetically: agent-reach, hexo-blog-publish
	agentReach := groups[0].Assets[0]
	if agentReach.Name != "agent-reach" || agentReach.Type != ChangeUpdated {
		t.Errorf("agent-reach asset mismatch: %+v", agentReach)
	}

	hexo := groups[0].Assets[1]
	if hexo.Name != "hexo-blog-publish" || hexo.Type != ChangeNew || len(hexo.Files) != 2 {
		t.Errorf("hexo-blog-publish asset mismatch: %+v", hexo)
	}

	if groups[1].Category != "Rules" {
		t.Errorf("expected second category to be Rules, got %s", groups[1].Category)
	}
	if groups[1].Assets[0].Type != ChangeDeleted {
		t.Errorf("expected Rules asset to be deleted, got %v", groups[1].Assets[0].Type)
	}
}

func TestParsePorcelainStatus(t *testing.T) {
	raw := "?? skills/hexo-blog-publish/\n" +
		" M workflows/deploy.md\n" +
		"D  rules/stale-rule\n"

	groups := parsePorcelainStatus(raw)
	if countAssets(groups) != 3 {
		t.Fatalf("expected 3 assets across groups, got %d", countAssets(groups))
	}

	foundSkills := false
	for _, g := range groups {
		if g.Category == "Skills" {
			foundSkills = true
			if len(g.Assets) != 1 || g.Assets[0].Name != "hexo-blog-publish" {
				t.Errorf("unexpected skills asset: %+v", g.Assets)
			}
			if g.Assets[0].Type != ChangeNew {
				t.Errorf("untracked skill should be ChangeNew, got %v", g.Assets[0].Type)
			}
		}
	}
	if !foundSkills {
		t.Error("expected Skills group in porcelain output")
	}
}

func TestPrintAssetCategoryGroups(t *testing.T) {
	groups := []CategoryGroup{
		{
			Category: "Skills",
			Assets: []AssetChange{
				{
					Category: "Skills",
					Name:     "hexo-blog-publish",
					Type:     ChangeNew,
					Files: []FileDiff{
						{Status: "A", Path: "skills/hexo-blog-publish/SKILL.md"},
						{Status: "A", Path: "skills/hexo-blog-publish/run.sh"},
					},
				},
				{
					Category: "Skills",
					Name:     "agent-reach",
					Type:     ChangeUpdated,
					Files: []FileDiff{
						{Status: "M", Path: "skills/agent-reach/SKILL.md"},
					},
				},
			},
		},
	}

	// Test normal mode
	var bufNormal bytes.Buffer
	printAssetCategoryGroups(&bufNormal, groups, false)
	outNormal := bufNormal.String()

	if !strings.Contains(outNormal, "● Skills:") {
		t.Errorf("missing ● Skills: in normal output:\n%s", outNormal)
	}
	if !strings.Contains(outNormal, "+  hexo-blog-publish (new, 2 files)") {
		t.Errorf("missing hexo-blog-publish (new, 2 files) in normal output:\n%s", outNormal)
	}
	if !strings.Contains(outNormal, "~  agent-reach (updated)") {
		t.Errorf("missing agent-reach (updated) in normal output:\n%s", outNormal)
	}
	// Normal mode should NOT print individual file paths
	if strings.Contains(outNormal, "skills/hexo-blog-publish/SKILL.md") {
		t.Errorf("normal mode should not print individual file paths:\n%s", outNormal)
	}

	// Test verbose mode
	var bufVerbose bytes.Buffer
	printAssetCategoryGroups(&bufVerbose, groups, true)
	outVerbose := bufVerbose.String()

	if !strings.Contains(outVerbose, "skills/hexo-blog-publish/SKILL.md") {
		t.Errorf("verbose mode should print file path, got:\n%s", outVerbose)
	}
}
