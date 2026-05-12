# 任务 8：分析工具

**目标：** 使用 LLM 分析验证失败原因，支持 Docker 执行失败和 Agent 模拟失败两种类型。

**文件：**
- 创建：`internal/tools/analyze_tool.go`
- 创建：`internal/tools/analyze_tool_test.go`

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

type mockChatModel struct{}

func (m *mockChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{
		Role:    schema.Assistant,
		Content: `{"reason": "missing dependency", "suggest": "add go mod tidy before build", "fix_type": "command"}`,
	}, nil
}

func TestAnalyzeDockerFailure(t *testing.T) {
	at := NewAnalyzeTool(&mockChatModel{})

	result := &skill.ExecResult{
		StepName: "build",
		ExitCode: 1,
		Stdout:   "",
		Stderr:   "missing go.mod",
		Success:  false,
	}

	analysis, err := at.AnalyzeDocker(context.Background(), result, &skill.Skill{Name: "test"})
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
	at := NewAnalyzeTool(&mockChatModel{})

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

	analysis, err := at.AnalyzeAgentSim(context.Background(), simResult, &skill.Skill{Name: "test"})
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
- instruction: 需要修改指导文本
- dependency: 需要添加或修改依赖`

func buildDockerAnalyzePrompt(result *skill.ExecResult, sk *skill.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Skill: %s\n", sk.Name)
	fmt.Fprintf(&b, "## 描述: %s\n\n", sk.Description)
	fmt.Fprintf(&b, "## 失败步骤: %s\n", result.StepName)
	fmt.Fprintf(&b, "退出码: %d\n", result.ExitCode)
	if result.Stderr != "" {
		fmt.Fprintf(&b, "Stderr: %s\n", truncateStr(result.Stderr, 500))
	}
	if result.Stdout != "" {
		fmt.Fprintf(&b, "Stdout: %s\n", truncateStr(result.Stdout, 500))
	}
	fmt.Fprintf(&b, "\n分析此步骤失败的原因并建议修复方案。\n")
	return b.String()
}

func buildAgentSimAnalyzePrompt(simResult *SimulateResult, sk *skill.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Skill: %s\n", sk.Name)
	fmt.Fprintf(&b, "## 描述: %s\n\n", sk.Description)
	fmt.Fprintf(&b, "## Agent 模拟测试结果\n")
	fmt.Fprintf(&b, "得分: %.2f\n", simResult.Score)
	fmt.Fprintf(&b, "通过: %v\n\n", simResult.Pass)

	fmt.Fprintf(&b, "## 检查点详情\n")
	for i, cp := range simResult.Checkpoints {
		status := "✓"
		if !cp.Met {
			status = "✗"
		}
		fmt.Fprintf(&b, "%d. %s [%s]\n", i+1, cp.Checkpoint.Description, status)
		if !cp.Met {
			fmt.Fprintf(&b, "   证据: %s\n", cp.Evidence)
		}
	}

	fmt.Fprintf(&b, "\n## Agent 行为记录\n%s\n", truncateStr(simResult.Trace, 1000))
	fmt.Fprintf(&b, "\n分析为什么 agent 没有遵循 skill 的指导，并建议如何改进 skill 的描述。\n")
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
git commit -m "feat: 添加支持 Docker 和 Agent 模拟的分析工具"
```
