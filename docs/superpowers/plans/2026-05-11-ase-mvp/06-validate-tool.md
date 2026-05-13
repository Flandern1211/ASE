# 任务 6：验证工具（整体验证）

**目标：** 实现两种验证方式：Docker 整体执行验证和 Agent 模拟测试验证。不再逐步骤验证，而是对 skill 整体进行验证。

**文件：**
- 创建：`internal/tools/validate_tool.go`
- 创建：`internal/tools/validate_tool_test.go`
- 创建：`internal/tools/agent_sim_tool.go`
- 创建：`internal/tools/agent_sim_tool_test.go`
- 创建：`internal/tools/checkpoint_tool.go`
- 创建：`internal/tools/checkpoint_tool_test.go`

**依赖：** Task 2（核心类型）

---

## 设计说明

### 旧方案 vs 新方案

| | 旧方案（逐步骤） | 新方案（整体验证） |
|---|---|---|
| 验证单位 | 单个 Step | 整个 Skill |
| 数据来源 | 预解析的 Steps[] | LLM 读取 RawContent |
| Docker 执行 | 逐步骤提取命令执行 | LLM 生成完整执行脚本 |
| Agent 模拟 | 用 Steps 生成场景 | 用 RawContent 生成场景 |
| 信息完整性 | 丢失 Red Flags 等 | 完整保留 |

### 为什么整体验证更好

1. **零信息丢失** — LLM 读全文，能遵循 Red Flags、Iron Law 等纪律约束
2. **更简单** — 不需要从 markdown 中提取命令（格式千差万别）
3. **更准确** — LLM 理解 skill 意图后生成的执行脚本比正则提取更可靠
4. **兼容所有 skill 类型** — executable、instructional、mixed 都走同一路径

---

## 验证流程

```
Skill{RawContent, Metadata}
        ↓
   skill_type?
   ┌────┴────┐
   ↓         ↓
executable  instructional/mixed
   ↓         ↓
Docker 执行  Agent 模拟
   ↓         ↓
整体验证     检查点验证
   ↓         ↓
   └────┬────┘
      通过/失败
```

---

## 步骤

- [ ] **步骤 1：实现 Docker 整体执行验证**

创建 `internal/tools/validate_tool.go`：

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

// ValidationResult 持有验证结果。
type ValidationResult struct {
	Passed  bool   `json:"passed"`
	Summary string `json:"summary"`
	Details string `json:"details"`
}

// ValidateTool 使用 LLM 读取完整 SKILL.md 并生成执行脚本，在 Docker 中执行后验证。
type ValidateTool struct {
	llm model.ChatModel
}

// NewValidateTool 创建 ValidateTool。
func NewValidateTool(llm model.ChatModel) *ValidateTool {
	return &ValidateTool{llm: llm}
}

// GenerateScript 让 LLM 根据完整 SKILL.md 生成可执行的 shell 脚本。
func (v *ValidateTool) GenerateScript(ctx context.Context, sk *skill.Skill) (string, error) {
	prompt := buildGenerateScriptPrompt(sk)

	resp, err := v.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: generateScriptPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("LLM 调用失败: %w", err)
	}

	return strings.TrimSpace(resp.Content), nil
}

// ValidateOutput 验证执行输出是否符合 skill 预期。
func (v *ValidateTool) ValidateOutput(ctx context.Context, sk *skill.Skill, stdout, stderr string, exitCode int) (*ValidationResult, error) {
	prompt := buildValidateOutputPrompt(sk, stdout, stderr, exitCode)

	resp, err := v.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: validateOutputPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parseValidationResult(resp.Content)
}

const generateScriptPrompt = `你是一个 Skill 执行器。阅读完整的 SKILL.md 内容，生成一个可执行的 shell 脚本。

规则：
1. 提取所有可执行的 bash 命令，按正确顺序组织
2. 如果 skill 是指导型（instructional），将指导转化为具体命令
3. 脚本应该可独立运行，不需要人工交互
4. 在关键步骤后添加 echo 输出进度
5. 任何步骤失败时立即退出（set -e）
6. 只输出脚本内容，不要解释`

func buildGenerateScriptPrompt(sk *skill.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Skill: %s\n", sk.Name)
	fmt.Fprintf(&b, "## 描述: %s\n\n", sk.Description)
	fmt.Fprintf(&b, "## SKILL.md 完整内容\n\n%s\n\n", sk.RawContent)

	if len(sk.SupportFiles) > 0 {
		fmt.Fprintf(&b, "## 辅助文件\n\n")
		for path, content := range sk.SupportFiles {
			truncated := content
			if len(truncated) > 500 {
				truncated = truncated[:500] + "..."
			}
			fmt.Fprintf(&b, "### %s\n` + "```" + `\n%s\n` + "```" + `\n\n", path, truncated)
		}
	}

	fmt.Fprintf(&b, "\n请根据以上内容生成可执行脚本。\n")
	return b.String()
}

const validateOutputPrompt = `你是一个 Skill 验证器。判断执行结果是否符合 SKILL.md 的预期。

输出必须是 JSON 格式：
{"passed": true/false, "summary": "一句话总结", "details": "详细说明"}

判断标准：
1. 脚本是否执行了 skill 描述的核心操作？
2. 输出中是否有错误或异常？
3. 是否达到了 skill 的预期目标？`

func buildValidateOutputPrompt(sk *skill.Skill, stdout, stderr string, exitCode int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Skill: %s\n", sk.Name)
	fmt.Fprintf(&b, "## 描述: %s\n\n", sk.Description)
	fmt.Fprintf(&b, "## 执行结果\n")
	fmt.Fprintf(&b, "退出码: %d\n", exitCode)
	if stdout != "" {
		fmt.Fprintf(&b, "Stdout:\n```\n%s\n```\n", truncateStr(stdout, 2000))
	}
	if stderr != "" {
		fmt.Fprintf(&b, "Stderr:\n```\n%s\n```\n", truncateStr(stderr, 2000))
	}
	fmt.Fprintf(&b, "\n请判断执行结果是否符合 skill 预期。返回 JSON。\n")
	return b.String()
}

func parseValidationResult(content string) (*ValidationResult, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return &ValidationResult{
			Passed:  false,
			Summary: "无法解析验证结果",
			Details: content,
		}, nil
	}

	jsonStr := content[start : end+1]
	var result struct {
		Passed  bool   `json:"passed"`
		Summary string `json:"summary"`
		Details string `json:"details"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return &ValidationResult{
			Passed:  false,
			Summary: "JSON 解析失败",
			Details: content,
		}, nil
	}

	return &ValidationResult{
		Passed:  result.Passed,
		Summary: result.Summary,
		Details: result.Details,
	}, nil
}

func truncateStr(s string, max int) string {
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
	Pass        bool               `json:"pass"`
	Score       float64            `json:"score"`
	Checkpoints []CheckpointResult `json:"checkpoints"`
	Trace       string             `json:"trace"`
}

// Simulate 执行 Agent 模拟测试。
func (a *AgentSimTool) Simulate(ctx context.Context, sk *skill.Skill, scenario *skill.TestScenario) (*SimulateResult, error) {
	// 1. 如果没有测试场景，让 LLM 根据 RawContent 生成
	if scenario == nil {
		var err error
		scenario, err = a.generateScenario(ctx, sk)
		if err != nil {
			return nil, fmt.Errorf("生成测试场景失败: %w", err)
		}
	}

	// 2. 让 LLM 模拟 agent 带着 skill 执行场景
	trace, err := a.simulateAgent(ctx, sk, scenario)
	if err != nil {
		return nil, fmt.Errorf("模拟 agent 失败: %w", err)
	}

	// 3. 验证检查点
	result := a.evaluateCheckpoints(scenario.Checkpoints, trace)

	return result, nil
}

// generateScenario 让 LLM 根据完整 SKILL.md 内容生成测试场景
func (a *AgentSimTool) generateScenario(ctx context.Context, sk *skill.Skill) (*skill.TestScenario, error) {
	prompt := fmt.Sprintf(`根据以下 skill 的完整内容，生成一个测试场景来验证 skill 的有效性。

## Skill: %s
## 描述: %s

## SKILL.md 完整内容

%s

## 要求
1. 创建一个能触发 skill 使用的场景
2. 定义 3-5 个行为检查点（覆盖 skill 的核心要求和约束）
3. 每个检查点说明类型（must_do/must_not/order/output）

返回 JSON 格式：
{
  "name": "场景名称",
  "description": "场景描述",
  "steps": ["步骤1", "步骤2"],
  "checkpoints": [
    {"description": "检查点描述", "type": "must_do", "required": true}
  ]
}`, sk.Name, sk.Description, truncateStr(sk.RawContent, 3000))

	resp, err := a.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: "你是一个测试场景生成器。根据 skill 的完整内容（包括指导原则和约束）生成测试场景。"},
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
## 描述: %s

## SKILL.md 完整内容

%s

## 测试场景
%s

## 执行步骤
%s

请描述你将如何执行这个场景，包括你会采取的具体行动。
重要：你必须严格遵循 skill 中的所有约束和指导原则。`,
		sk.Name,
		sk.Description,
		truncateStr(sk.RawContent, 3000),
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

		traceLower := strings.ToLower(trace)
		descLower := strings.ToLower(cp.Description)

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
	result.Pass = result.Score >= 0.8

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

func extractKeywords(expected string) []string {
	var keywords []string
	words := strings.FieldsFunc(expected, func(r rune) bool {
		return r == ',' || r == ';' || r == '.' || r == ' '
	})

	skipWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"and": true, "or": true, "but": true, "in": true, "on": true,
		"at": true, "to": true, "for": true, "of": true, "with": true,
		"that": true, "this": true, "it": true, "be": true, "was": true,
		"should": true, "must": true, "can": true, "will": true,
	}

	for _, w := range words {
		if !skipWords[w] && len(w) > 2 {
			keywords = append(keywords, w)
		}
	}

	return keywords
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

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
```

- [ ] **步骤 4：编写测试**

创建 `internal/tools/validate_tool_test.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type validateMockLLM struct {
	response string
}

func (m *validateMockLLM) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{
		Role:    schema.Assistant,
		Content: m.response,
	}, nil
}

func TestGenerateScript(t *testing.T) {
	llm := &validateMockLLM{
		response: "#!/bin/bash\necho 'running tests'\ngo test ./... -v",
	}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{
		Name:        "test-skill",
		Description: "Run Go tests",
		RawContent:  "# Test\nRun `go test`",
	}

	script, err := vt.GenerateScript(context.Background(), sk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if script == "" {
		t.Error("expected non-empty script")
	}
}

func TestValidateOutputPass(t *testing.T) {
	resultJSON := `{"passed": true, "summary": "all tests passed", "details": "exit code 0"}`
	llm := &validateMockLLM{response: resultJSON}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{Name: "test-skill", Description: "Run tests"}
	vr, err := vt.ValidateOutput(context.Background(), sk, "ok", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vr.Passed {
		t.Error("expected validation to pass")
	}
}

func TestValidateOutputFail(t *testing.T) {
	resultJSON := `{"passed": false, "summary": "tests failed", "details": "compilation error"}`
	llm := &validateMockLLM{response: resultJSON}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{Name: "test-skill", Description: "Run tests"}
	vr, err := vt.ValidateOutput(context.Background(), sk, "", "error", 1)
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
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type simMockLLM struct {
	responses []string
	idx       int
}

func (m *simMockLLM) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	resp := m.responses[m.idx]
	m.idx++
	return &schema.Message{
		Role:    schema.Assistant,
		Content: resp,
	}, nil
}

func TestSimulateGeneratesScenario(t *testing.T) {
	scenarioJSON, _ := json.Marshal(map[string]interface{}{
		"name":        "test scenario",
		"description": "Test if agent follows the skill",
		"steps":       []string{"Step 1", "Step 2"},
		"checkpoints": []map[string]interface{}{
			{"description": "follows instructions", "type": "must_do", "required": true},
		},
	})

	llm := &simMockLLM{
		responses: []string{
			string(scenarioJSON),
			"I followed the instructions carefully and completed all steps.",
		},
	}
	at := NewAgentSimTool(llm)

	sk := &skill.Skill{
		Name:        "test-skill",
		Description: "Use when testing",
		RawContent:  "# Test\nFollow these instructions.",
	}

	result, err := at.Simulate(context.Background(), sk, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Score == 0 {
		t.Error("expected non-zero score")
	}
}

func TestSimulateWithProvidedScenario(t *testing.T) {
	llm := &simMockLLM{
		responses: []string{
			"I did exactly what was asked.",
		},
	}
	at := NewAgentSimTool(llm)

	sk := &skill.Skill{
		Name:       "test-skill",
		RawContent: "# Test",
	}
	scenario := &skill.TestScenario{
		Name:  "manual scenario",
		Steps: []string{"Do something"},
		Checkpoints: []skill.Checkpoint{
			{Description: "does something", Type: "must_do", Required: true},
		},
	}

	result, err := at.Simulate(context.Background(), sk, scenario)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Checkpoints) != 1 {
		t.Errorf("expected 1 checkpoint, got %d", len(result.Checkpoints))
	}
}
```

创建 `internal/tools/checkpoint_tool_test.go`：

```go
package tools

import (
	"Agent/internal/skill"
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type cpMockLLM struct {
	response string
}

func (m *cpMockLLM) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{
		Role:    schema.Assistant,
		Content: m.response,
	}, nil
}

func TestCheckpointVerifyMet(t *testing.T) {
	llm := &cpMockLLM{response: `{"met": true, "evidence": "agent wrote test first"}`}
	ct := NewCheckpointTool(llm)

	cp := skill.Checkpoint{Description: "writes test first", Type: "must_do", Required: true}
	result, err := ct.Verify(context.Background(), cp, "I wrote a failing test before writing code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Met {
		t.Error("expected checkpoint to be met")
	}
}

func TestCheckpointVerifyNotMet(t *testing.T) {
	llm := &cpMockLLM{response: `{"met": false, "evidence": "agent started coding directly"}`}
	ct := NewCheckpointTool(llm)

	cp := skill.Checkpoint{Description: "writes test first", Type: "must_do", Required: true}
	result, err := ct.Verify(context.Background(), cp, "I started coding right away")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Met {
		t.Error("expected checkpoint to not be met")
	}
}
```

- [ ] **步骤 5：运行测试确认通过**

```bash
go test ./internal/tools/ -v -run TestGenerateScript
go test ./internal/tools/ -v -run TestValidate
go test ./internal/tools/ -v -run TestSimulate
go test ./internal/tools/ -v -run TestCheckpoint
```
预期：全部 PASS

- [ ] **步骤 6：提交**

```bash
git add internal/tools/validate_tool.go internal/tools/validate_tool_test.go
git add internal/tools/agent_sim_tool.go internal/tools/agent_sim_tool_test.go
git add internal/tools/checkpoint_tool.go internal/tools/checkpoint_tool_test.go
git commit -m "feat: 实现整体验证工具 — Docker 执行 + Agent 模拟 + 检查点验证"
```
