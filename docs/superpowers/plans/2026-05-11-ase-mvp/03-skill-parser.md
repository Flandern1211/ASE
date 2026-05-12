# 任务 3：Skill 解析器（LLM 解析）

**目标：** 实现基于 LLM 的 Skill 解析器，读取 SKILL.md 和辅助文件，提取结构化信息。

**文件：**
- 创建：`internal/skill/parser.go`
- 创建：`internal/skill/parser_test.go`

**依赖：** Task 2（核心类型）、Task 5（模型 Provider）

---

## 设计说明

SKILL.md 格式是半结构化的：
- Frontmatter 字段不固定
- Markdown body 完全自由格式
- 可能包含 scripts/、tests/、config/ 等辅助文件

**结论：** 不适合用固定 struct + yaml tag 做硬解析。改用 LLM 做语义解析。

### 解析流程

```
1. 读取 SKILL.md 原始内容
2. 扫描目录，读取辅助文件（scripts/、tests/、config/）
3. 构建 prompt，让 LLM 提取结构化信息
4. 解析 LLM 输出为 Skill struct
```

---

## 步骤

- [ ] **步骤 1：编写失败测试**

创建 `internal/skill/parser_test.go`：

```go
package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillFile(t *testing.T) {
	content := `---
name: "go-test-build"
description: "Use when running Go tests and building the project"
argument-hint: "optional build flags"
compatibility: "Requires Go 1.21+"
metadata:
  author: "test-author"
  source: "test.md"
user-invocable: true
disable-model-invocation: false
---

# Go Test & Build

## Overview
Run tests and build the Go project.

## Steps

### Step 1: Run tests
` + "```bash" + `
go test ./... -v
` + "```" + `
**Expected**: All tests pass, exit code 0

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

	skill, err := Parse(tmp, &mockLLM{})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if skill.Name != "go-test-build" {
		t.Errorf("expected name 'go-test-build', got %q", skill.Name)
	}
	if skill.Description != "Use when running Go tests and building the project" {
		t.Errorf("unexpected description: %q", skill.Description)
	}
	if len(skill.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(skill.Steps))
	}
	if skill.Steps[0].Command != "go test ./... -v" {
		t.Errorf("unexpected step command: %q", skill.Steps[0].Command)
	}
}

func TestParseSkillWithScripts(t *testing.T) {
	skillContent := `---
name: "with-scripts"
description: "Skill with helper scripts"
---

# With Scripts

## Steps

### Step 1: Setup
` + "```bash" + `
bash scripts/setup.sh
` + "```" + `
**Expected**: Setup complete
`
	scriptContent := `#!/bin/bash
echo "setting up..."`

	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "my-skill")
	os.MkdirAll(filepath.Join(skillDir, "scripts"), 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)
	os.WriteFile(filepath.Join(skillDir, "scripts", "setup.sh"), []byte(scriptContent), 0755)

	skill, err := Parse(skillDir, &mockLLM{})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if _, ok := skill.SupportFiles["scripts/setup.sh"]; !ok {
		t.Error("expected support file scripts/setup.sh")
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

// mockLLM 用于测试，返回预定义的解析结果
type mockLLM struct{}

func (m *mockLLM) Generate(ctx interface{}, messages interface{}, opts ...interface{}) (interface{}, error) {
	// 返回预定义的 JSON 响应
	return nil, nil
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/skill/ -v -run TestParse
```
预期：FAIL（Parse 函数签名不匹配）

- [ ] **步骤 3：实现 LLM 解析器**

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

// LLM 提取结果的 JSON 结构
type llmParseResult struct {
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Frontmatter     map[string]interface{} `json:"frontmatter"`
	Steps           []Step            `json:"steps"`
	Deps            []Dependency      `json:"deps"`
	Purpose         string            `json:"purpose"`
	UseCases        []string          `json:"use_cases"`
	CorePrinciples  string            `json:"core_principles"`
	SkillType       string            `json:"skill_type"`
	HasExecutable   bool              `json:"has_executable"`
	HasGuidance     bool              `json:"has_guidance"`
}

// Parse 从目录读取 skill，使用 LLM 解析内容。
// 如果路径是文件，使用其父目录作为 skill 目录。
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

	// 读取 SKILL.md 内容
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("读取 skill 文件失败: %w", err)
	}
	rawContent := string(data)

	// 扫描辅助文件
	supportFiles, err := scanSupportFiles(skillDir)
	if err != nil {
		return nil, fmt.Errorf("扫描辅助文件失败: %w", err)
	}

	// 使用 LLM 解析
	skill, err := parseWithLLM(context.Background(), llm, rawContent, supportFiles)
	if err != nil {
		return nil, fmt.Errorf("LLM 解析失败: %w", err)
	}

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

// parseWithLLM 使用 LLM 从 skill 内容中提取结构化信息
func parseWithLLM(ctx context.Context, llm model.ChatModel, content string, supportFiles map[string]string) (*Skill, error) {
	prompt := buildParsePrompt(content, supportFiles)

	resp, err := llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: parseSystemPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parseLLMResponse(resp.Content)
}

const parseSystemPrompt = `你是一个 Skill 文件解析器。分析给定的 SKILL.md 文件内容，提取结构化信息。

输出必须是 JSON 格式，包含以下字段：
- name: skill 名称（从 frontmatter 提取）
- description: skill 描述（从 frontmatter 提取）
- frontmatter: 原始 frontmatter 对象（保留所有字段）
- steps: 可执行步骤数组，每个包含 name, command, expected, order
- deps: 环境依赖数组，每个包含 name, version, type
- purpose: skill 的核心目的（一句话）
- use_cases: 适用场景列表
- core_principles: 核心原则或约束
- skill_type: "executable"（只有命令）, "instructional"（只有指令）, 或 "mixed"（混合）
- has_executable: 是否包含可执行命令
- has_guidance: 是否包含指导原则

如果某些字段无法确定，使用空值（空字符串、空数组）。`

func buildParsePrompt(content string, supportFiles map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## SKILL.md 内容\n\n%s\n\n", content)

	if len(supportFiles) > 0 {
		fmt.Fprintf(&b, "## 辅助文件\n\n")
		for path, content := range supportFiles {
			fmt.Fprintf(&b, "### %s\n```\n%s\n```\n\n", path, truncateStr(content, 500))
		}
	}

	fmt.Fprintf(&b, "\n请分析以上内容，提取结构化信息。返回 JSON 格式。\n")
	return b.String()
}

func parseLLMResponse(content string) (*Skill, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("无法从 LLM 响应中提取 JSON")
	}

	jsonStr := content[start : end+1]
	var result llmParseResult
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
		Name:              result.Name,
		Description:       result.Description,
		Frontmatter:       result.Frontmatter,
		Steps:             result.Steps,
		Deps:              result.Deps,
		Purpose:           result.Purpose,
		UseCases:          result.UseCases,
		CorePrinciples:    result.CorePrinciples,
		SkillType:         result.SkillType,
		HasExecutable:     result.HasExecutable,
		HasGuidance:       result.HasGuidance,
	}, nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
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
git commit -m "feat: 实现基于 LLM 的 skill 解析器"
```
