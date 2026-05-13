# 任务 8：分析工具

**目标：** 使用 LLM 分析验证失败原因，支持 Docker 执行失败和 Agent 模拟失败两种类型。LLM 基于完整 SKILL.md 内容进行分析。

**文件：**
- 创建：`internal/tools/analyze_tool.go`
- 创建：`internal/tools/analyze_tool_test.go`

---

## 设计说明

分析工具接收验证结果（Docker 执行输出或 Agent 模拟结果）+ 完整 Skill 元数据，让 LLM 诊断失败原因并建议修复方案。

**关键变化：** 分析 prompt 中传入 `sk.RawContent`（完整 SKILL.md）而非 `sk.Steps`，让 LLM 能看到 skill 的完整上下文（包括 Red Flags、约束条件等），给出更准确的诊断。

---

## 步骤

- [ ] **步骤 1：编写失败测试**

创建 `internal/tools/analyze_tool_test.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type analyzeMockModel struct{}

func (m *analyzeMockModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{
		Role: schema.Assistant,
		Content: `{
			"reason": "missing dependency: go mod tidy not run before build",
			"suggest": "add 'go mod tidy' step before 'go build'",
			"fix_type": "command"
		}`,
	}, nil
}

func TestAnalyzeDockerFailure(t *testing.T) {
	at := NewAnalyzeTool(&analyzeMockModel{})

	result := &skill.ExecResult{
		StepName: "build",
		ExitCode: 1,
		Stdout:   "",
		Stderr:   "missing go.mod",
		Success:  false,
	}

	sk := &skill.Skill{
		Name:        "test",
		Description: "Build Go project",
		RawContent:  "# Build\nRun go build.",
	}

	analysis, err := at.AnalyzeDocker(context.Background(), result, sk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Reason == "" {
		t.Error("expected non-empty reason")
	}
	if analysis.Suggest == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestAnalyzeAgentSimFailure(t *testing.T) {
	at := NewAnalyzeTool(&analyzeMockModel{})

	simResult := &SimulateResult{
		Pass:  false,
		Score: 0.4,
		Checkpoints: []CheckpointResult{
			{
				Checkpoint: skill.Checkpoint{Description: "writes test first", Type: "must_do", Required: true},
				Met:        false,
				Evidence:   "agent started coding without tests",
			},
		},
	}

	sk := &skill.Skill{
		Name:        "test",
		Description: "TDD skill",
		RawContent:  "# TDD\nAlways write tests first.",
	}

	analysis, err := at.AnalyzeAgentSim(context.Background(), simResult, sk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Reason == "" {
		t.Error("expected non-empty reason")
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/tools/ -v -run TestAnalyze
```
预期：FAIL

- [ ] **步骤 3：实现 AnalyzeTool**

创建 `internal/tools/analyze_tool.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// AnalyzeTool 使用 LLM 诊断验证失败原因。
type AnalyzeTool struct {
	llm model.ChatModel
}

// NewAnalyzeTool 创建 AnalyzeTool。
func NewAnalyzeTool(llm model.ChatModel) *AnalyzeTool {
	return &AnalyzeTool{llm: llm}
}

// AnalyzeDocker 诊断 Docker 执行失败。
func (a *AnalyzeTool) AnalyzeDocker(ctx context.Context, result *skill.ExecResult, sk *skill.Skill) (*skill.Analysis, error) {
	prompt := buildDockerAnalyzePrompt(result, sk)

	resp, err := a.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: analyzeSystemPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parseAnalysis(resp.Content, result.StepName, "docker")
}

// AnalyzeAgentSim 诊断 Agent 模拟测试失败。
func (a *AnalyzeTool) AnalyzeAgentSim(ctx context.Context, simResult *SimulateResult, sk *skill.Skill) (*skill.Analysis, error) {
	prompt := buildAgentSimAnalyzePrompt(simResult, sk)

	resp, err := a.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: analyzeSystemPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parseAnalysis(resp.Content, "agent_simulation", "instruction")
}

const analyzeSystemPrompt = `你是一位 Skill 质量专家。分析验证失败原因并建议修复方案。

输出必须是 JSON 格式：
{"reason": "失败原因", "suggest": "修复建议", "fix_type": "command/instruction/dependency"}

fix_type 说明：
- command: 需要修改命令或脚本
- instruction: 需要修改指导文本（使指导更清晰、更具体）
- dependency: 需要添加或修改依赖`

func buildDockerAnalyzePrompt(result *skill.ExecResult, sk *skill.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Skill: %s\n", sk.Name)
	fmt.Fprintf(&b, "## 描述: %s\n\n", sk.Description)
	fmt.Fprintf(&b, "## SKILL.md 完整内容\n\n%s\n\n", truncateStr(sk.RawContent, 3000))
	fmt.Fprintf(&b, "## 失败步骤: %s\n", result.StepName)
	fmt.Fprintf(&b, "退出码: %d\n", result.ExitCode)
	if result.Stderr != "" {
		fmt.Fprintf(&b, "Stderr: %s\n", truncateStr(result.Stderr, 500))
	}
	if result.Stdout != "" {
		fmt.Fprintf(&b, "Stdout: %s\n", truncateStr(result.Stdout, 500))
	}
	fmt.Fprintf(&b, "\n请分析此 skill 执行失败的原因。返回 JSON。\n")
	return b.String()
}

func buildAgentSimAnalyzePrompt(simResult *SimulateResult, sk *skill.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Skill: %s\n", sk.Name)
	fmt.Fprintf(&b, "## 描述: %s\n\n", sk.Description)
	fmt.Fprintf(&b, "## SKILL.md 完整内容\n\n%s\n\n", truncateStr(sk.RawContent, 3000))
	fmt.Fprintf(&b, "## Agent 模拟测试结果\n")
	fmt.Fprintf(&b, "得分: %.2f\n", simResult.Score)
	fmt.Fprintf(&b, "通过: %v\n\n", simResult.Pass)

	fmt.Fprintf(&b, "## 检查点详情\n")
	for i, cp := range simResult.Checkpoints {
		status := "pass"
		if !cp.Met {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "%d. %s [%s]\n", i+1, cp.Checkpoint.Description, status)
		if !cp.Met {
			fmt.Fprintf(&b, "   证据: %s\n", cp.Evidence)
		}
	}

	fmt.Fprintf(&b, "\n## Agent 行为记录\n%s\n", truncateStr(simResult.Trace, 1000))
	fmt.Fprintf(&b, "\n请分析为什么 agent 没有遵循 skill 的指导。考虑 skill 中的约束和 Red Flags。返回 JSON。\n")
	return b.String()
}

func parseAnalysis(content, stepName, fixType string) (*skill.Analysis, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		jsonStr := content[start : end+1]
		var result struct {
			Reason  string `json:"reason"`
			Suggest string `json:"suggest"`
			FixType string `json:"fix_type"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
			if result.FixType == "" {
				result.FixType = fixType
			}
			return &skill.Analysis{
				StepName: stepName,
				Reason:   result.Reason,
				Suggest:  result.Suggest,
				FixType:  result.FixType,
			}, nil
		}
	}

	return &skill.Analysis{
		StepName: stepName,
		Reason:   content,
		Suggest:  "需要人工检查",
		FixType:  fixType,
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
go test ./internal/tools/ -v -run TestAnalyze
```
预期：PASS

- [ ] **步骤 5：提交**

```bash
git add internal/tools/analyze_tool.go internal/tools/analyze_tool_test.go
git commit -m "feat: 添加分析工具 — 基于完整 SKILL.md 诊断验证失败"
```
