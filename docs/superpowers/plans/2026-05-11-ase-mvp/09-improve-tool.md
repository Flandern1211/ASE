# 任务 9：改进工具

**目标：** 使用 LLM 根据分析结果修改 skill 文件。

**文件：**
- 创建：`internal/tools/improve_tool.go`
- 创建：`internal/tools/improve_tool_test.go`

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
version: 2
expected_env:
  - bash: true
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
	skillPath := filepath.Join(tmp, "skill.md")
	originalContent := `---
name: test-skill
description: test
version: 1
expected_env:
  - bash: true
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
		Name:    "test-skill",
		Version: 1,
		Path:    skillPath,
	}

	analysis := &skill.Analysis{
		StepName: "Build",
		Reason:   "missing go mod tidy",
		Suggest:  "add setup step",
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
	content, err := os.ReadFile(sk.Path)
	if err != nil {
		return "", fmt.Errorf("读取 skill 文件失败: %w", err)
	}

	prompt := buildImprovePrompt(string(content), sk, analysis)

	resp, err := im.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: "你是一位 DevOps 专家。根据失败分析改进 skill 文件。只输出完整的改进后 markdown 文件内容，不要解释。"},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("LLM 调用失败: %w", err)
	}

	newContent := strings.TrimSpace(resp.Content)

	// 验证改进后的内容可解析
	if _, err := skill.ParseContent(newContent, sk.Path); err != nil {
		return "", fmt.Errorf("LLM 生成了无效的 skill 内容: %w", err)
	}

	return newContent, nil
}

func buildImprovePrompt(currentContent string, sk *skill.Skill, analysis *skill.Analysis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "当前 skill 文件:\n\n%s\n\n", currentContent)
	fmt.Fprintf(&b, "失败分析:\n")
	fmt.Fprintf(&b, "  步骤: %s\n", analysis.StepName)
	fmt.Fprintf(&b, "  原因: %s\n", analysis.Reason)
	fmt.Fprintf(&b, "  建议: %s\n\n", analysis.Suggest)
	fmt.Fprintf(&b, "改进 skill 文件以修复此失败。保持相同结构，按需添加/修改步骤。\n")
	fmt.Fprintf(&b, "递增版本号。输出完整的改进后 markdown 文件。\n")
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
git commit -m "feat: 添加 LLM skill 改进工具"
```
