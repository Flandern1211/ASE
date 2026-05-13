package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mockModel "ASE/internal/mock/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/golang/mock/gomock"
)

// buildMockMetadata 构建标准的元数据 mock 响应
func buildMockMetadata(name, desc string) string {
	result := map[string]interface{}{
		"name":            name,
		"description":     desc,
		"frontmatter":     map[string]interface{}{"name": name, "description": desc},
		"deps":            []map[string]string{},
		"purpose":         "test purpose",
		"use_cases":       []string{"testing"},
		"core_principles": "test accurately",
		"skill_type":      "mixed",
		"has_executable":  true,
		"has_guidance":    true,
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func TestParseSkillFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChatModel := mockModel.NewMockBaseChatModel(ctrl)

	mockChatModel.EXPECT().Generate(
		gomock.Any(),
		gomock.Any(),
	).Return(&schema.Message{
		Content: buildMockMetadata("go-test-build", "Use when running Go tests and building the project"),
	}, nil)

	content := `---
name: "go-test-build"
description: "Use when running Go tests and building the project"
---

# Go Test & Build

## Overview
Run tests and build the Go project.

## Steps

### Step 1: Run tests
` + "```bash" + `
go test ./... -v
` + "```" + `
**Expected**: All tests pass

### Step 2: Build project
` + "```bash" + `
go build -o ./bin/app .
` + "```" + `
**Expected**: Generates ./bin/app file
`

	tmp := t.TempDir()
	skillPath := filepath.Join(tmp, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skill, err := Parse(tmp, mockChatModel)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if skill.Name != "go-test-build" {
		t.Errorf("expected name 'go-test-build', got %q", skill.Name)
	}
	if skill.Description != "Use when running Go tests and building the project" {
		t.Errorf("unexpected description: %q", skill.Description)
	}
	if skill.RawContent == "" {
		t.Error("expected non-empty RawContent")
	}
	if !strings.Contains(skill.RawContent, "go test") {
		t.Error("RawContent should contain original SKILL.md text")
	}
	if skill.Path == "" {
		t.Error("expected non-empty Path")
	}
}

func TestParseSkillWithSupportFiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChatModel := mockModel.NewMockBaseChatModel(ctrl)

	mockChatModel.EXPECT().Generate(
		gomock.Any(),
		gomock.Any(),
	).Return(&schema.Message{
		Content: buildMockMetadata("with-scripts", "Skill with helper scripts"),
	}, nil)

	skillContent := `---
name: "with-scripts"
description: "Skill with helper scripts"
---

# With Scripts
`
	scriptContent := `#!/bin/bash
echo "setting up..."`

	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "my-skill")
	scriptsDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "setup.sh"), []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	skill, err := Parse(skillDir, mockChatModel)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// 验证 SupportFiles 存储的是文件路径
	path, ok := skill.SupportFiles["scripts/setup.sh"]
	if !ok {
		// Windows 上路径分隔符是反斜杠
		path, ok = skill.SupportFiles[`scripts\setup.sh`]
		if !ok {
			t.Errorf("expected support file scripts/setup.sh, got keys: %v", getKeys(skill.SupportFiles))
			return
		}
	}
	if !strings.Contains(path, "setup.sh") {
		t.Errorf("expected path containing setup.sh, got: %s", path)
	}
	if skill.Dir != skillDir {
		t.Errorf("expected Dir %q, got %q", skillDir, skill.Dir)
	}
}

func TestParseFileNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChatModel := mockModel.NewMockBaseChatModel(ctrl)

	_, err := Parse("/nonexistent/skill", mockChatModel)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseMissingFrontmatter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChatModel := mockModel.NewMockBaseChatModel(ctrl)

	content := `# No Frontmatter

Just some text.
`

	tmp := t.TempDir()
	skillPath := filepath.Join(tmp, "SKILL.md")
	os.WriteFile(skillPath, []byte(content), 0644)

	_, err := Parse(tmp, mockChatModel)
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParseRejectsEmptyName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChatModel := mockModel.NewMockBaseChatModel(ctrl)

	mockChatModel.EXPECT().Generate(
		gomock.Any(),
		gomock.Any(),
	).Return(&schema.Message{
		Content: `{"name": "", "description": "test"}`,
	}, nil)

	content := `---
name: ""
description: "test"
---
# Test
`
	tmp := t.TempDir()
	skillPath := filepath.Join(tmp, "SKILL.md")
	os.WriteFile(skillPath, []byte(content), 0644)

	_, err := Parse(tmp, mockChatModel)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestParseExtractsMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChatModel := mockModel.NewMockBaseChatModel(ctrl)

	mockChatModel.EXPECT().Generate(
		gomock.Any(),
		gomock.Any(),
	).Return(&schema.Message{
		Content: `{
			"name": "my-skill",
			"description": "A test skill",
			"frontmatter": {"name": "my-skill"},
			"deps": [{"name": "docker", "version": ">=20.0", "type": "tool"}],
			"purpose": "test something",
			"use_cases": ["testing", "debugging"],
			"core_principles": "be thorough",
			"skill_type": "executable",
			"has_executable": true,
			"has_guidance": false
		}`,
	}, nil)

	content := `---
name: "my-skill"
description: "A test skill"
---
# My Skill
`
	tmp := t.TempDir()
	skillPath := filepath.Join(tmp, "SKILL.md")
	os.WriteFile(skillPath, []byte(content), 0644)

	skill, err := Parse(tmp, mockChatModel)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(skill.Deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(skill.Deps))
	}
	if skill.Deps[0].Name != "docker" {
		t.Errorf("expected dep name 'docker', got %q", skill.Deps[0].Name)
	}
	if skill.SkillType != "executable" {
		t.Errorf("expected skill_type 'executable', got %q", skill.SkillType)
	}
	if len(skill.UseCases) != 2 {
		t.Errorf("expected 2 use_cases, got %d", len(skill.UseCases))
	}
	if skill.CorePrinciples != "be thorough" {
		t.Errorf("expected core_principles 'be thorough', got %q", skill.CorePrinciples)
	}
}

func TestParseRootDirectoryFiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockChatModel := mockModel.NewMockBaseChatModel(ctrl)

	mockChatModel.EXPECT().Generate(
		gomock.Any(),
		gomock.Any(),
	).Return(&schema.Message{
		Content: buildMockMetadata("with-root-files", "Skill with root directory support files"),
	}, nil)

	skillContent := `---
name: "with-root-files"
description: "Skill with root directory support files"
---

# With Root Files

See ` + "`" + `helper.md` + "`" + ` for details.
`
	helperContent := "# Helper\n\nThis is a helper file."

	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.md"), []byte(helperContent), 0644); err != nil {
		t.Fatal(err)
	}

	skill, err := Parse(skillDir, mockChatModel)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// 验证根目录下的辅助文件被扫描到，且值是路径
	path, ok := skill.SupportFiles["helper.md"]
	if !ok {
		t.Errorf("expected support file helper.md, got keys: %v", getKeys(skill.SupportFiles))
		return
	}
	if !strings.HasSuffix(path, "helper.md") {
		t.Errorf("expected path ending with helper.md, got: %s", path)
	}
}

func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
