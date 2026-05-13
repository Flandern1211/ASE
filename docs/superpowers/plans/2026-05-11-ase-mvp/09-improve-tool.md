# 任务 9：改进工具

**目标：** 使用 LLM 根据分析结果修改 skill 文件。LLM 基于完整 SKILL.md 内容进行改进，保持 skill 的结构和约束。

**文件：**
- 创建：`internal/tools/improve_tool.go`
- 创建：`internal/tools/improve_tool_test.go`

---

## 设计说明

改进工具接收分析结果 + 完整 Skill，让 LLM 生成改进后的 SKILL.md。

**关键点：**
- 传入 `sk.RawContent`（完整原文）而非 Steps，让 LLM 能看到完整上下文
- 改进时保持 skill 的核心意图和约束，只修复导致失败的部分
- 改进后的内容直接写回文件

---

## 步骤

- [ ] **步骤 1：编写失败测试**

创建 `internal/tools/improve_tool_test.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type improveMockModel struct{}

func (m *improveMockModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{
		Role: schema.Assistant,
		Content: `---
name: improved-skill
description: improved
---

# Improved

## Steps

### Step 1: Setup
` + "```bash" + `
go mod tidy
` + "```" + `
**Expected**: dependencies resolved

### Step 2: Build
` + "```bash" + `
go build .
` + "```" + `
**Expected**: builds successfully
`,
	}, nil
}

func TestImproveSkill(t *testing.T) {
	it := NewImproveTool(&improveMockModel{})

	tmp := t.TempDir()
	skillPath := filepath.Join(tmp, "SKILL.md")
	originalContent := `---
name: test-skill
description: test
---

# Test

## Steps

### Step 1: Build
` + "```bash" + `
go build .
` + "```" + `
**Expected**: builds
`

	if err := os.WriteFile(skillPath, []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	sk := &skill.Skill{
		Name:       "test-skill",
		Path:       skillPath,
		RawContent: originalContent,
	}

	analysis := &skill.Analysis{
		StepName: "Build",
		Reason:   "missing go mod tidy",
		Suggest:  "add setup step before build",
		FixType:  "command",
	}

	newContent, err := it.Improve(context.Background(), sk, analysis)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newContent == "" {
		t.Error("expected non-empty improved content")
	}
	if newContent == originalContent {
		t.Error("expected content to change")
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/tools/ -v -run TestImprove
```
预期：FAIL

- [ ] **步骤 3：实现 ImproveTool**

创建 `internal/tools/improve_tool.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ImproveTool 使用 LLM 根据分析结果修改 skill 文件。
type ImproveTool struct {
	llm model.ChatModel
}

// NewImproveTool 创建 ImproveTool。
func NewImproveTool(llm model.ChatModel) *ImproveTool {
	return &ImproveTool{llm: llm}
}

// Improve 修改 skill 文件，返回新内容。
func (im *ImproveTool) Improve(ctx context.Context, sk *skill.Skill, analysis *skill.Analysis) (string, error) {
	prompt := buildImprovePrompt(sk, analysis)

	resp, err := im.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: improveSystemPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("LLM 调用失败: %w", err)
	}

	newContent := strings.TrimSpace(resp.Content)

	// 验证改进后的内容有 frontmatter
	if !strings.HasPrefix(newContent, "---") {
		return "", fmt.Errorf("LLM 生成的内容缺少 YAML frontmatter")
	}

	// 写入文件
	if err := os.WriteFile(sk.Path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("写入 skill 文件失败: %w", err)
	}

	return newContent, nil
}

const improveSystemPrompt = `你是一位 Skill 优化专家。根据失败分析改进 SKILL.md 文件。

规则：
1. 保持 skill 的核心意图和结构
2. 只修改导致失败的部分，不要大幅重写
3. 保持 frontmatter 中的 name 和 description
4. 改进指导文本使其更清晰、更具体
5. 如果是命令失败，修复命令或添加前置步骤
6. 只输出完整的改进后 markdown 文件内容，不要解释`

func buildImprovePrompt(sk *skill.Skill, analysis *skill.Analysis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "当前 SKILL.md 完整内容:\n\n%s\n\n", sk.RawContent)
	fmt.Fprintf(&b, "失败分析:\n")
	fmt.Fprintf(&b, "  失败点: %s\n", analysis.StepName)
	fmt.Fprintf(&b, "  原因: %s\n", analysis.Reason)
	fmt.Fprintf(&b, "  建议: %s\n", analysis.Suggest)
	fmt.Fprintf(&b, "  修复类型: %s\n\n", analysis.FixType)
	fmt.Fprintf(&b, "请改进此 skill 文件以修复此失败。保持相同的整体结构，按需修改。\n")
	fmt.Fprintf(&b, "输出完整的改进后 markdown 文件。\n")
	return b.String()
}
```

- [ ] **步骤 4：运行测试确认通过**

```bash
go test ./internal/tools/ -v -run TestImprove
```
预期：PASS

- [ ] **步骤 5：提交**

```bash
git add internal/tools/improve_tool.go internal/tools/improve_tool_test.go
git commit -m "feat: 添加改进工具 — 基于完整 SKILL.md 修复验证失败"
```
