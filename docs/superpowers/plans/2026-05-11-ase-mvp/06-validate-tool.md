# 任务 6：验证工具（Docker + Agent 模拟）

**目标：** 实现两种验证方式：Docker 执行结果验证和 Agent 模拟测试验证。

**文件：**
- 创建：`internal/tools/validate_tool.go`（Docker 结果验证）
- 创建：`internal/tools/validate_tool_test.go`
- 创建：`internal/tools/agent_sim_tool.go`（Agent 模拟测试）
- 创建：`internal/tools/agent_sim_tool_test.go`
- 创建：`internal/tools/checkpoint_tool.go`（检查点验证）
- 创建：`internal/tools/checkpoint_tool_test.go`

---

## 设计说明

验证分为两类：

| 类型 | 适用场景 | 验证方式 |
|---|---|---|
| Docker 执行验证 | 可执行型 skill | 检查退出码和输出 |
| Agent 模拟测试 | 指令型/混合型 skill | LLM 生成检查点，对比 agent 行为 |

---

## 步骤

- [ ] **步骤 1：实现 Docker 结果验证**

创建 `internal/tools/validate_tool.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"fmt"
	"strings"
)

// ValidationResult 持有步骤验证结果。
type ValidationResult struct {
	Passed bool
	Diff   string
}

// ValidateTool 检查 ExecResult 是否符合 Step 的预期。
type ValidateTool struct{}

// NewValidateTool 创建 ValidateTool。
func NewValidateTool() *ValidateTool {
	return &ValidateTool{}
}

// Validate 检查执行结果是否符合步骤预期。
func (v *ValidateTool) Validate(ctx context.Context, result *skill.ExecResult, step *skill.Step) (*ValidationResult, error) {
	vr := &ValidationResult{}

	// 检查退出码
	if result.ExitCode != 0 {
		vr.Passed = false
		vr.Diff = fmt.Sprintf("步骤 %q 失败，退出码 %d。\nStdout: %s\nStderr: %s",
			step.Name, result.ExitCode, result.Stdout, result.Stderr)
		return vr, nil
	}

	// 如果指定了预期输出，检查关键词
	if step.Expected != "" {
		expectedLower := strings.ToLower(step.Expected)
		stdoutLower := strings.ToLower(result.Stdout)

		keywords := extractKeywords(expectedLower)
		if len(keywords) > 0 {
			for _, kw := range keywords {
				if !strings.Contains(stdoutLower, kw) {
					vr.Passed = false
					vr.Diff = fmt.Sprintf("步骤 %q: 预期输出包含 %q，实际: %s",
						step.Name, kw, truncate(result.Stdout, 200))
					return vr, nil
				}
			}
		}
	}

	vr.Passed = true
	return vr, nil
}

func extractKeywords(expected string) []string {
	var keywords []string
	words := strings.FieldsFunc(expected, func(r rune) bool {
		return r == ',' || r == ';' || r == '.' || r == ' '
	})

	skipWords := map[string]bool{
		"all": true, "the": true, "is": true, "a": true,
		"and": true, "or": true, "exit": true, "code": true,
		"outputs": true, "generates": true, "creates": true,
		"pass": true, "passes": true, "successful": true,
		"with": true, "for": true, "to": true, "of": true,
		"expected": true, "should": true, "must": true,
	}

	for _, w := range words {
		if !skipWords[w] && len(w) > 2 {
			keywords = append(keywords, w)
		}
	}

	return keywords
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
```

- [ ] **步骤 2：实现 Agent 模拟测试**

创建 `internal/tools/agent_sim_tool.go`：

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

// AgentSimTool 执行 Agent 模拟测试。
type AgentSimTool struct {
	llm model.ChatModel
}

// NewAgentSimTool 创建 AgentSimTool。
func NewAgentSimTool(llm model.ChatModel) *AgentSimTool {
	return &AgentSimTool{llm: llm}
}

// SimulateResult 模拟测试结果
type SimulateResult struct {
	Pass        bool                      `json:"pass"`
	Score       float64                   `json:"score"`
	Checkpoints []CheckpointResult        `json:"checkpoints"`
	Trace       string                    `json:"trace"`
}

// Simulate 执行 Agent 模拟测试。
func (a *AgentSimTool) Simulate(ctx context.Context, sk *skill.Skill, scenario *skill.TestScenario) (*SimulateResult, error) {
	// 1. 如果有现有测试场景，使用它；否则让 LLM 生成
	if scenario == nil {
		var err error
		scenario, err = a.generateScenario(ctx, sk)
		if err != nil {
			return nil, fmt.Errorf("生成测试场景失败: %w", err)
		}
	}

	// 2. 让 LLM 模拟 agent 行为
	trace, err := a.simulateAgent(ctx, sk, scenario)
	if err != nil {
		return nil, fmt.Errorf("模拟 agent 失败: %w", err)
	}

	// 3. 验证检查点
	result := a.evaluateCheckpoints(scenario.Checkpoints, trace)

	return result, nil
}

// generateScenario 让 LLM 根据 skill 内容生成测试场景
func (a *AgentSimTool) generateScenario(ctx context.Context, sk *skill.Skill) (*skill.TestScenario, error) {
	prompt := fmt.Sprintf(`根据以下 skill 内容，生成一个测试场景来验证 skill 的有效性。

## Skill: %s
%s

## 要求
1. 创建一个能触发 skill 使用的场景
2. 定义 3-5 个行为检查点
3. 每个检查点说明类型（must_do/must_not/order/output）

返回 JSON 格式：
{
  "name": "场景名称",
  "description": "场景描述",
  "steps": ["步骤1", "步骤2"],
  "checkpoints": [
    {"description": "检查点描述", "type": "must_do", "required": true}
  ]
}`, sk.Name, truncateStr(sk.RawContent, 1000))

	resp, err := a.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: "你是一个测试场景生成器。根据 skill 内容生成测试场景。"},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, err
	}

	return parseTestScenario(resp.Content)
}

// simulateAgent 让 LLM 模拟 agent 带着 skill 执行场景
func (a *AgentSimTool) simulateAgent(ctx context.Context, sk *skill.Skill, scenario *skill.TestScenario) (string, error) {
	prompt := fmt.Sprintf(`你是一个 AI agent。现在你需要根据以下 skill 指导来执行任务。

## Skill: %s
%s

## 测试场景
%s

## 执行步骤
%s

请描述你将如何执行这个场景，包括你会采取的具体行动。`,
		sk.Name,
		truncateStr(sk.RawContent, 2000),
		scenario.Description,
		strings.Join(scenario.Steps, "\n"))

	resp, err := a.llm.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// evaluateCheckpoints 评估检查点是否满足
func (a *AgentSimTool) evaluateCheckpoints(checkpoints []skill.Checkpoint, trace string) *SimulateResult {
	result := &SimulateResult{
		Checkpoints: make([]CheckpointResult, len(checkpoints)),
		Trace:       trace,
	}

	passed := 0
	for i, cp := range checkpoints {
		cr := CheckpointResult{
			Checkpoint: cp,
			Met:        false,
			Evidence:   "",
		}

		// 简单的关键词匹配（后续可以用 LLM 做更精确的判断）
		traceLower := strings.ToLower(trace)
		descLower := strings.ToLower(cp.Description)

		// 提取关键词
		keywords := extractKeywords(descLower)
		if len(keywords) > 0 {
			allFound := true
			for _, kw := range keywords {
				if !strings.Contains(traceLower, kw) {
					allFound = false
					break
				}
			}
			cr.Met = allFound
		}

		if cr.Met {
			passed++
		}

		result.Checkpoints[i] = cr
	}

	result.Score = float64(passed) / float64(len(checkpoints))
	result.Pass = result.Score >= 0.8 // 80% 检查点通过即为通过

	return result
}

func parseTestScenario(content string) (*skill.TestScenario, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("无法从 LLM 响应中提取 JSON")
	}

	jsonStr := content[start : end+1]
	var scenario skill.TestScenario
	if err := json.Unmarshal([]byte(jsonStr), &scenario); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	return &scenario, nil
}
```

- [ ] **步骤 3：实现检查点验证**

创建 `internal/tools/checkpoint_tool.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// CheckpointTool 使用 LLM 精确验证检查点。
type CheckpointTool struct {
	llm model.ChatModel
}

// NewCheckpointTool 创建 CheckpointTool。
func NewCheckpointTool(llm model.ChatModel) *CheckpointTool {
	return &CheckpointTool{llm: llm}
}

// CheckpointResult 单个检查点的验证结果
type CheckpointResult struct {
	Checkpoint skill.Checkpoint `json:"checkpoint"`
	Met        bool             `json:"met"`
	Evidence   string           `json:"evidence"`
}

// Verify 使用 LLM 验证检查点是否满足
func (c *CheckpointTool) Verify(ctx context.Context, checkpoint skill.Checkpoint, trace string) (*CheckpointResult, error) {
	prompt := fmt.Sprintf(`判断以下 agent 行为是否满足检查点要求。

## 检查点
描述: %s
类型: %s
是否必须: %v

## Agent 行为记录
%s

请判断检查点是否满足，并提供证据。返回 JSON：
{"met": true/false, "evidence": "具体证据"}`,
		checkpoint.Description,
		checkpoint.Type,
		checkpoint.Required,
		truncateStr(trace, 2000))

	resp, err := c.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: "你是一个行为验证器。判断 agent 行为是否满足检查点要求。"},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, err
	}

	return parseCheckpointResult(resp.Content, checkpoint)
}

func parseCheckpointResult(content string, checkpoint skill.Checkpoint) (*CheckpointResult, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("无法从 LLM 响应中提取 JSON")
	}

	jsonStr := content[start : end+1]
	var result struct {
		Met      bool   `json:"met"`
		Evidence string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	return &CheckpointResult{
		Checkpoint: checkpoint,
		Met:        result.Met,
		Evidence:   result.Evidence,
	}, nil
}
```

- [ ] **步骤 4：编写测试**

创建 `internal/tools/validate_tool_test.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"testing"
	"time"
)

func TestValidateSuccess(t *testing.T) {
	vt := NewValidateTool()
	result := &skill.ExecResult{
		StepName: "test",
		ExitCode: 0,
		Stdout:   "ok",
		Success:  true,
		Duration: time.Second,
	}
	step := &skill.Step{
		Name:     "test",
		Command:  "echo ok",
		Expected: "outputs ok",
	}

	vr, err := vt.Validate(context.Background(), result, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vr.Passed {
		t.Error("expected validation to pass")
	}
}

func TestValidateFailure(t *testing.T) {
	vt := NewValidateTool()
	result := &skill.ExecResult{
		StepName: "test",
		ExitCode: 1,
		Stdout:   "",
		Stderr:   "error occurred",
		Success:  false,
		Duration: time.Second,
	}
	step := &skill.Step{
		Name:     "test",
		Command:  "false",
		Expected: "exit code 0",
	}

	vr, err := vt.Validate(context.Background(), result, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.Passed {
		t.Error("expected validation to fail")
	}
}
```

创建 `internal/tools/agent_sim_tool_test.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"testing"
)

func TestSimulateInstructionalSkill(t *testing.T) {
	llm := &mockLLMForSim{
		response: `{
			"name": "test scenario",
			"description": "Test if agent follows TDD",
			"steps": ["Write a failing test", "Run test to see it fail", "Write minimal code"],
			"checkpoints": [
				{"description": "writes test first", "type": "must_do", "required": true},
				{"description": "runs test before code", "type": "order", "required": true}
			]
		}`,
	}

	at := NewAgentSimTool(llm)
	sk := &skill.Skill{
		Name:        "test-driven-development",
		Description: "Use when implementing features",
		RawContent:  "# TDD\nWrite test first.",
		SkillType:   "instructional",
	}

	result, err := at.Simulate(context.Background(), sk, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Score == 0 {
		t.Error("expected non-zero score")
	}
}

type mockLLMForSim struct {
	response string
}

func (m *mockLLMForSim) Generate(ctx interface{}, messages interface{}, opts ...interface{}) (interface{}, error) {
	return &schema.Message{
		Role:    schema.Assistant,
		Content: m.response,
	}, nil
}
```

- [ ] **步骤 5：运行测试确认通过**

```bash
go test ./internal/tools/ -v -run TestValidate
go test ./internal/tools/ -v -run TestSimulate
```
预期：PASS

- [ ] **步骤 6：提交**

```bash
git add internal/tools/validate_tool.go internal/tools/validate_tool_test.go
git add internal/tools/agent_sim_tool.go internal/tools/agent_sim_tool_test.go
git add internal/tools/checkpoint_tool.go internal/tools/checkpoint_tool_test.go
git commit -m "feat: 添加 Docker 验证和 Agent 模拟测试工具"
```
