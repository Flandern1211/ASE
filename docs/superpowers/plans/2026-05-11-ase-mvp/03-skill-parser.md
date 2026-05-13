# 任务 3：Skill 解析器（元数据提取）

**目标：** 实现基于 LLM 的 Skill 解析器，从 SKILL.md 提取元数据（name, description, deps 等），保留完整原文供执行时使用。

**文件：**
- 创建：`internal/skill/parser.go`
- 创建：`internal/skill/parser_test.go`

**依赖：** Task 2（核心类型）、Task 5（模型 Provider）

---

## 设计说明

### 解析流程

```
SKILL.md + 辅助文件
    ↓
读取原始内容 + 扫描目录
    ↓
LLM 提取元数据（name, description, deps, use_cases, purpose, skill_type）
    ↓
Skill{元数据 + RawContent + SupportFiles}
```

### parser 只做元数据提取，不做步骤解析

原因：
1. 步骤解析信息损失大（Red Flags、Iron Law、Rationalizations 丢失）
2. 不同 skill 的步骤格式差异巨大（编号列表 vs 标题 vs 流程图 vs 纯文本）
3. 执行时 LLM 需要读全文才能正确遵循 skill，预解析的步骤反而可能误导

### system prompt 大幅简化

旧 prompt 需要教 LLM 识别三种 markdown 格式、提取 steps、分类 skill_type（~30 行）。
新 prompt 只提取 ~8 个元数据字段（~15 行），解析更稳定、token 消耗更低。

---

## 步骤

- [ ] **步骤 1：编写失败测试**

创建 `internal/skill/parser_test.go`：

```go
package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// mockLLM 返回预定义的元数据提取结果
type mockLLM struct {
	response string
}

func (m *mockLLM) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{
		Role:    schema.Assistant,
		Content: m.response,
	}, nil
}

// buildMockMetadata 构建标准的元数据 mock 响应
func buildMockMetadata(name, desc string) string {
	result := map[string]interface{}{
		"name":        name,
		"description": desc,
		"frontmatter": map[string]interface{}{"name": name, "description": desc},
		"deps":        []map[string]string{},
		"purpose":     "test purpose",
		"use_cases":   []string{"testing"},
		"core_principles": "test accurately",
		"skill_type":  "mixed",
		"has_executable": true,
		"has_guidance":   true,
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func TestParseSkillFile(t *testing.T) {
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

	mockResp := buildMockMetadata("go-test-build", "Use when running Go tests and building the project")
	skill, err := Parse(tmp, &mockLLM{response: mockResp})
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
	os.MkdirAll(filepath.Join(skillDir, "scripts"), 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)
	os.WriteFile(filepath.Join(skillDir, "scripts", "setup.sh"), []byte(scriptContent), 0755)

	mockResp := buildMockMetadata("with-scripts", "Skill with helper scripts")
	skill, err := Parse(skillDir, &mockLLM{response: mockResp})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if _, ok := skill.SupportFiles["scripts/setup.sh"]; !ok {
		t.Error("expected support file scripts/setup.sh")
	}
	if skill.Dir != skillDir {
		t.Errorf("expected Dir %q, got %q", skillDir, skill.Dir)
	}
}

func TestParseFileNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/skill", &mockLLM{})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseMissingFrontmatter(t *testing.T) {
	content := `# No Frontmatter

Just some text.
`

	tmp := t.TempDir()
	skillPath := filepath.Join(tmp, "SKILL.md")
	os.WriteFile(skillPath, []byte(content), 0644)

	_, err := Parse(tmp, &mockLLM{})
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParseRejectsEmptyName(t *testing.T) {
	content := `---
name: ""
description: "test"
---
# Test
`
	tmp := t.TempDir()
	skillPath := filepath.Join(tmp, "SKILL.md")
	os.WriteFile(skillPath, []byte(content), 0644)

	mockResp := `{"name": "", "description": "test"}`
	_, err := Parse(tmp, &mockLLM{response: mockResp})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestParseExtractsMetadata(t *testing.T) {
	content := `---
name: "my-skill"
description: "A test skill"
---
# My Skill
`
	tmp := t.TempDir()
	skillPath := filepath.Join(tmp, "SKILL.md")
	os.WriteFile(skillPath, []byte(content), 0644)

	mockResp := `{
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
	}`
	skill, err := Parse(tmp, &mockLLM{response: mockResp})
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
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/skill/ -v -run TestParse
```
预期：FAIL（函数签名或字段不匹配）

- [ ] **步骤 3：实现简化版解析器**

创建 `internal/skill/parser.go`：

```go
package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// llmMetaResult 是 LLM 元数据提取的 JSON 结构
type llmMetaResult struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Frontmatter     map[string]interface{} `json:"frontmatter"`
	Deps            []Dependency           `json:"deps"`
	Purpose         string                 `json:"purpose"`
	UseCases        []string               `json:"use_cases"`
	CorePrinciples  string                 `json:"core_principles"`
	SkillType       string                 `json:"skill_type"`
	HasExecutable   bool                   `json:"has_executable"`
	HasGuidance     bool                   `json:"has_guidance"`
}

// Parse 从目录读取 skill，使用 LLM 提取元数据。
// 完整 SKILL.md 文本保存在 RawContent 中，供执行时使用。
func Parse(path string, llm model.ChatModel) (*Skill, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	var skillDir, skillFile string
	if info.IsDir() {
		skillDir = path
		skillFile = filepath.Join(path, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			return nil, fmt.Errorf("在 %s 中未找到 SKILL.md", path)
		}
	} else {
		skillFile = path
		skillDir = filepath.Dir(path)
	}

	// 读取 SKILL.md 原始内容
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("读取 skill 文件失败: %w", err)
	}
	rawContent := string(data)

	// 验证 frontmatter 存在
	if !strings.HasPrefix(strings.TrimSpace(rawContent), "---") {
		return nil, fmt.Errorf("SKILL.md 缺少 YAML frontmatter（应以 --- 开头）")
	}

	// 扫描辅助文件
	supportFiles, err := scanSupportFiles(skillDir)
	if err != nil {
		return nil, fmt.Errorf("扫描辅助文件失败: %w", err)
	}

	// 使用 LLM 提取元数据
	skill, err := extractMetadata(context.Background(), llm, rawContent)
	if err != nil {
		return nil, fmt.Errorf("LLM 元数据提取失败: %w", err)
	}

	// 填充运行时字段
	skill.Path = skillFile
	skill.Dir = skillDir
	skill.RawContent = rawContent
	skill.SupportFiles = supportFiles

	return skill, nil
}

// scanSupportFiles 扫描 skill 目录中的辅助文件
func scanSupportFiles(skillDir string) (map[string]string, error) {
	files := make(map[string]string)

	supportDirs := []string{"scripts", "tests", "config"}
	for _, dir := range supportDirs {
		dirPath := filepath.Join(skillDir, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			relPath, _ := filepath.Rel(skillDir, path)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[relPath] = string(data)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return files, nil
}

// extractMetadata 使用 LLM 从 SKILL.md 内容中提取元数据
func extractMetadata(ctx context.Context, llm model.ChatModel, content string) (*Skill, error) {
	prompt := fmt.Sprintf("## SKILL.md 内容\n\n%s\n\n请分析以上内容，提取元数据。返回 JSON 格式。", content)

	resp, err := llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: metaExtractPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parseMetaResponse(resp.Content)
}

const metaExtractPrompt = `你是一个 Skill 元数据提取器。分析 SKILL.md 内容，提取元数据信息。

输出必须是 JSON 格式，包含以下字段：
- name: skill 名称（从 frontmatter 提取）
- description: skill 描述（从 frontmatter 提取）
- frontmatter: 原始 frontmatter 对象（保留所有字段）
- deps: 环境依赖数组，每个包含 name, version, type（如无法确定，返回空数组）
- purpose: skill 的核心目的（一句话概括）
- use_cases: 适用场景列表
- core_principles: 核心原则或约束
- skill_type: "executable"（只有命令）, "instructional"（只有指导）, 或 "mixed"（混合）
- has_executable: 是否包含可执行命令
- has_guidance: 是否包含指导原则/约束规则

注意：不要解析具体步骤，只提取元数据。`

func parseMetaResponse(content string) (*Skill, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("无法从 LLM 响应中提取 JSON")
	}

	jsonStr := content[start : end+1]
	var result llmMetaResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	if result.Name == "" {
		return nil, fmt.Errorf("LLM 未提取到 skill 名称")
	}
	if result.Description == "" {
		return nil, fmt.Errorf("LLM 未提取到 skill 描述")
	}

	return &Skill{
		Name:            result.Name,
		Description:     result.Description,
		Frontmatter:     result.Frontmatter,
		Deps:            result.Deps,
		Purpose:         result.Purpose,
		UseCases:        result.UseCases,
		CorePrinciples:  result.CorePrinciples,
		SkillType:       result.SkillType,
		HasExecutable:   result.HasExecutable,
		HasGuidance:     result.HasGuidance,
	}, nil
}
```

- [ ] **步骤 4：运行测试确认通过**

```bash
go test ./internal/skill/ -v -run TestParse
```
预期：全部 PASS

- [ ] **步骤 5：提交**

```bash
git add internal/skill/parser.go internal/skill/parser_test.go
git commit -m "feat: 简化 skill 解析器 — 只提取元数据，保留全文供执行"
```
