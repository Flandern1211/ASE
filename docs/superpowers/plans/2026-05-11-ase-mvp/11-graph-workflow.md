# 任务 11：Graph 工作流

**目标：** 实现整体验证流水线：Parse → Classify → Execute/ Simulate → Validate → Analyze → Improve，支持重试。

**文件：**
- 创建：`internal/graph/workflow.go`
- 创建：`internal/graph/workflow_test.go`

**依赖：** Task 6（验证工具）、Task 8（分析工具）、Task 9（改进工具）

---

## 设计说明

### 旧方案 vs 新方案

| | 旧方案（逐步骤） | 新方案（整体验证） |
|---|---|---|
| 执行单位 | 单个 Step | 整个 Skill |
| 循环逻辑 | 遍历 Steps[]，逐步骤执行/验证 | 根据 skill_type 选择执行路径 |
| 失败处理 | 单步骤失败 → 分析 → 改进 → 重新执行 | 整体失败 → 分析 → 改进 → 重新解析/执行 |
| 重试粒度 | 重试失败的步骤 | 重新执行整个 skill |

### 流水线

```
Parse（元数据提取）
    ↓
skill_type?
┌────┴────┐
↓         ↓
executable  instructional/mixed
↓         ↓
生成脚本    生成场景
↓         ↓
Docker执行  Agent模拟
↓         ↓
输出验证    检查点验证
↓         ↓
  ┌───┴───┐
  ↓       ↓
通过      失败
  ↓       ↓
完成    Analyze
          ↓
        Improve
          ↓
        重新 Parse → 重新执行（重试，最多 N 次）
```

---

## 步骤

- [ ] **步骤 1：编写失败测试**

创建 `internal/graph/workflow_test.go`：

```go
package graph

import (
	"Agent/internal/skill"
	"context"
	"testing"
	"time"
)

// --- Mock 工具 ---

type mockScriptGenerator struct {
	script string
}

func (m *mockScriptGenerator) GenerateScript(ctx context.Context, sk *skill.Skill) (string, error) {
	return m.script, nil
}

type mockDockerRunner struct {
	results []execResult
	idx     int
}

type execResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func (m *mockDockerRunner) Run(ctx context.Context, script string) (string, string, int, error) {
	if m.idx >= len(m.results) {
		return "", "", 0, nil
	}
	r := m.results[m.idx]
	m.idx++
	return r.stdout, r.stderr, r.exitCode, nil
}

type mockOutputValidator struct {
	pass bool
}

func (m *mockOutputValidator) ValidateOutput(ctx context.Context, sk *skill.Skill, stdout, stderr string, exitCode int) (*ValidationResult, error) {
	return &ValidationResult{Passed: m.pass, Summary: "mock"}, nil
}

type mockScenarioGenerator struct {
	scenario *skill.TestScenario
}

func (m *mockScenarioGenerator) GenerateScenario(ctx context.Context, sk *skill.Skill) (*skill.TestScenario, error) {
	return m.scenario, nil
}

type mockAgentSimulator struct {
	result *SimulateResult
}

func (m *mockAgentSimulator) Simulate(ctx context.Context, sk *skill.Skill, scenario *skill.TestScenario) (*SimulateResult, error) {
	return m.result, nil
}

type mockAnalyzer struct {
	analysis *skill.Analysis
}

func (m *mockAnalyzer) Analyze(ctx context.Context, result *ExecFailure, sk *skill.Skill) (*skill.Analysis, error) {
	return m.analysis, nil
}

type mockImprover struct {
	newContent string
}

func (m *mockImprover) Improve(ctx context.Context, sk *skill.Skill, analysis *skill.Analysis) (string, error) {
	return m.newContent, nil
}

type mockReparsedSkill struct {
	skill *skill.Skill
}

func (m *mockReparsedSkill) Reparse(ctx context.Context, rawContent string, path string) (*skill.Skill, error) {
	return m.skill, nil
}

// --- 测试 ---

func TestWorkflowExecutablePass(t *testing.T) {
	wf := NewWorkflow(&WorkflowDeps{
		ScriptGen:     &mockScriptGenerator{script: "echo ok"},
		DockerRunner:  &mockDockerRunner{results: []execResult{{stdout: "ok", exitCode: 0}}},
		OutputValid:   &mockOutputValidator{pass: true},
	})

	sk := &skill.Skill{
		Name:           "test",
		SkillType:      "executable",
		HasExecutable:  true,
		RawContent:     "# Test\nRun echo ok",
	}

	result := wf.Run(context.Background(), sk, &Config{MaxRetries: 3})

	if !result.Success {
		t.Error("expected workflow to succeed")
	}
	if result.Retries != 0 {
		t.Errorf("expected 0 retries, got %d", result.Retries)
	}
}

func TestWorkflowInstructionalPass(t *testing.T) {
	wf := NewWorkflow(&WorkflowDeps{
		ScenarioGen: &mockScenarioGenerator{
			scenario: &skill.TestScenario{
				Name:  "test",
				Steps: []string{"do something"},
				Checkpoints: []skill.Checkpoint{
					{Description: "does something", Type: "must_do", Required: true},
				},
			},
		},
		AgentSim: &mockAgentSimulator{
			result: &SimulateResult{
				Pass:  true,
				Score: 1.0,
				Checkpoints: []CheckpointResult{
					{Met: true, Evidence: "done"},
				},
			},
		},
	})

	sk := &skill.Skill{
		Name:          "test",
		SkillType:     "instructional",
		HasGuidance:   true,
		RawContent:    "# TDD\nWrite tests first.",
	}

	result := wf.Run(context.Background(), sk, &Config{MaxRetries: 3})

	if !result.Success {
		t.Error("expected workflow to succeed")
	}
}

func TestWorkflowRetryAndRecover(t *testing.T) {
	wf := NewWorkflow(&WorkflowDeps{
		ScriptGen:    &mockScriptGenerator{script: "go build ."},
		DockerRunner: &mockDockerRunner{
			results: []execResult{
				{stderr: "error", exitCode: 1},
				{stdout: "ok", exitCode: 0},
			},
		},
		OutputValid: &mockOutputValidator{pass: false},
		Analyzer:    &mockAnalyzer{analysis: &skill.Analysis{Reason: "bad", Suggest: "fix it", FixType: "command"}},
		Improver:    &mockImprover{newContent: "---\nname: test\ndesc: fixed\n---\n# Fixed"},
		Reparsed:    &mockReparsedSkill{skill: &skill.Skill{Name: "test", SkillType: "executable", RawContent: "fixed"}},
	})

	sk := &skill.Skill{
		Name:           "test",
		SkillType:      "executable",
		HasExecutable:  true,
		RawContent:     "# Test\nBuild.",
	}

	result := wf.Run(context.Background(), sk, &Config{MaxRetries: 3})

	if !result.Success {
		t.Error("expected workflow to succeed after retry")
	}
	if result.Retries != 1 {
		t.Errorf("expected 1 retry, got %d", result.Retries)
	}
}

func TestWorkflowMaxRetriesExhausted(t *testing.T) {
	wf := NewWorkflow(&WorkflowDeps{
		ScriptGen:    &mockScriptGenerator{script: "fail"},
		DockerRunner: &mockDockerRunner{
			results: []execResult{
				{stderr: "fail", exitCode: 1},
				{stderr: "fail", exitCode: 1},
			},
		},
		OutputValid: &mockOutputValidator{pass: false},
		Analyzer:    &mockAnalyzer{analysis: &skill.Analysis{Reason: "bad", Suggest: "fix", FixType: "command"}},
		Improver:    &mockImprover{newContent: "still bad"},
		Reparsed:    &mockReparsedSkill{skill: &skill.Skill{Name: "test", SkillType: "executable", RawContent: "still bad"}},
	})

	sk := &skill.Skill{
		Name:           "test",
		SkillType:      "executable",
		HasExecutable:  true,
		RawContent:     "# Test\nFail.",
	}

	result := wf.Run(context.Background(), sk, &Config{MaxRetries: 1})

	if result.Success {
		t.Error("expected workflow to fail after max retries")
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/graph/ -v
```
预期：FAIL

- [ ] **步骤 3：实现工作流**

创建 `internal/graph/workflow.go`：

```go
package graph

import (
	"Agent/internal/skill"
	"context"
	"fmt"
	"time"
)

// Config 持有工作流执行配置。
type Config struct {
	MaxRetries    int
	SandboxConfig interface{}
}

// WorkflowResult 持有完整工作流运行结果。
type WorkflowResult struct {
	Success    bool
	Retries    int
	Analyses   []*skill.Analysis
	Duration   time.Duration
	FinalSkill *skill.Skill
}

// ExecFailure 封装执行失败信息，供分析工具使用。
type ExecFailure struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// --- 工具接口 ---

// ScriptGenerator 从 SKILL.md 生成可执行脚本。
type ScriptGenerator interface {
	GenerateScript(ctx context.Context, sk *skill.Skill) (string, error)
}

// DockerRunner 在 Docker 中执行脚本。
type DockerRunner interface {
	Run(ctx context.Context, script string) (stdout, stderr string, exitCode int, err error)
}

// OutputValid 验证执行输出是否符合 skill 预期。
type OutputValid interface {
	ValidateOutput(ctx context.Context, sk *skill.Skill, stdout, stderr string, exitCode int) (*ValidationResult, error)
}

// ScenarioGen 从 SKILL.md 生成测试场景。
type ScenarioGen interface {
	GenerateScenario(ctx context.Context, sk *skill.Skill) (*skill.TestScenario, error)
}

// AgentSim 执行 Agent 模拟测试。
type AgentSim interface {
	Simulate(ctx context.Context, sk *skill.Skill, scenario *skill.TestScenario) (*SimulateResult, error)
}

// Analyzer 诊断验证失败原因。
type Analyzer interface {
	Analyze(ctx context.Context, failure *ExecFailure, sk *skill.Skill) (*skill.Analysis, error)
}

// Improver 根据分析修改 skill 文件。
type Improver interface {
	Improve(ctx context.Context, sk *skill.Skill, analysis *skill.Analysis) (string, error)
}

// Reparser 重新解析改进后的 skill。
type Reparser interface {
	Reparse(ctx context.Context, rawContent string, path string) (*skill.Skill, error)
}

// ValidationResult 验证结果
type ValidationResult struct {
	Passed  bool
	Summary string
}

// WorkflowDeps 持有工作流所需的全部工具。
type WorkflowDeps struct {
	ScriptGen    ScriptGen
	DockerRunner DockerRunner
	OutputValid  OutputValid
	ScenarioGen  ScenarioGen
	AgentSim     AgentSim
	Analyzer     Analyzer
	Improver     Improver
	Reparsed     Reparser
}

// Workflow 编排验证流水线。
type Workflow struct {
	deps *WorkflowDeps
}

// NewWorkflow 创建工作流。
func NewWorkflow(deps *WorkflowDeps) *Workflow {
	return &Workflow{deps: deps}
}

// Run 执行完整流水线：验证 → 失败分析 → 改进 → 重试。
func (w *Workflow) Run(ctx context.Context, sk *skill.Skill, cfg *Config) *WorkflowResult {
	start := time.Now()
	maxRetries := 3
	if cfg != nil && cfg.MaxRetries > 0 {
		maxRetries = cfg.MaxRetries
	}

	result := &WorkflowResult{FinalSkill: sk}

	for retries := 0; retries <= maxRetries; retries++ {
		var passed bool
		var failure *ExecFailure

		if sk.HasExecutable && w.deps.ScriptGen != nil && w.deps.DockerRunner != nil {
			// 可执行型：生成脚本 → Docker 执行 → 输出验证
			passed, failure = w.executeAndValidate(ctx, sk)
		} else if w.deps.AgentSim != nil {
			// 指令型/混合型：Agent 模拟 → 检查点验证
			passed, failure = w.simulateAndValidate(ctx, sk)
		} else {
			// 无验证工具，视为通过
			passed = true
		}

		if passed {
			result.Success = true
			result.Retries = retries
			result.Duration = time.Since(start)
			return result
		}

		// 重试次数用尽
		if retries >= maxRetries {
			break
		}

		// 分析 + 改进
		if w.deps.Analyzer != nil && w.deps.Improver != nil {
			fmt.Printf("[Analyze] 分析失败原因...\n")
			analysis, err := w.deps.Analyzer.Analyze(ctx, failure, sk)
			if err != nil {
				fmt.Printf("[Analyze] 错误: %v\n", err)
				break
			}
			result.Analyses = append(result.Analyses, analysis)
			fmt.Printf("  原因: %s\n  建议: %s\n", analysis.Reason, analysis.Suggest)

			fmt.Println("[Improve] 改进 skill 文件...")
			newContent, err := w.deps.Improver.Improve(ctx, sk, analysis)
			if err != nil {
				fmt.Printf("[Improve] 错误: %v\n", err)
				break
			}

			// 重新解析
			if w.deps.Reparsed != nil {
				reparsed, err := w.deps.Reparsed.Reparse(ctx, newContent, sk.Path)
				if err != nil {
					fmt.Printf("[Reparse] 错误: %v\n", err)
					break
				}
				sk = reparsed
				fmt.Printf("  → Skill 已更新\n")
			}
		}
	}

	result.Success = false
	result.Retries = maxRetries
	result.Duration = time.Since(start)
	return result
}

// executeAndValidate 执行可执行型 skill 并验证输出。
func (w *Workflow) executeAndValidate(ctx context.Context, sk *skill.Skill) (bool, *ExecFailure) {
	fmt.Printf("[Execute] 生成执行脚本...\n")
	script, err := w.deps.ScriptGen.GenerateScript(ctx, sk)
	if err != nil {
		fmt.Printf("[Execute] 脚本生成失败: %v\n", err)
		return false, &ExecFailure{Stderr: err.Error(), ExitCode: -1}
	}

	fmt.Printf("[Execute] 在 Docker 中执行...\n")
	stdout, stderr, exitCode, err := w.deps.DockerRunner.Run(ctx, script)
	if err != nil {
		fmt.Printf("[Execute] 执行失败: %v\n", err)
		return false, &ExecFailure{Stderr: err.Error(), ExitCode: -1}
	}

	fmt.Printf("[Validate] 验证输出...\n")
	if w.deps.OutputValid != nil {
		vr, err := w.deps.OutputValid.ValidateOutput(ctx, sk, stdout, stderr, exitCode)
		if err != nil {
			fmt.Printf("[Validate] 验证错误: %v\n", err)
			return false, &ExecFailure{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}
		}
		if vr.Passed {
			fmt.Printf("  → 通过: %s\n", vr.Summary)
			return true, nil
		}
		fmt.Printf("  → 失败: %s\n", vr.Summary)
		return false, &ExecFailure{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}
	}

	// 无验证工具，仅检查退出码
	if exitCode == 0 {
		return true, nil
	}
	return false, &ExecFailure{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}
}

// simulateAndValidate 模拟 Agent 执行并验证检查点。
func (w *Workflow) simulateAndValidate(ctx context.Context, sk *skill.Skill) (bool, *ExecFailure) {
	var scenario *skill.TestScenario
	if w.deps.ScenarioGen != nil {
		fmt.Printf("[Scenario] 生成测试场景...\n")
		var err error
		scenario, err = w.deps.ScenarioGen.GenerateScenario(ctx, sk)
		if err != nil {
			fmt.Printf("[Scenario] 生成失败: %v\n", err)
			return false, &ExecFailure{Stderr: err.Error()}
		}
	}

	fmt.Printf("[Simulate] 模拟 Agent 执行...\n")
	result, err := w.deps.AgentSim.Simulate(ctx, sk, scenario)
	if err != nil {
		fmt.Printf("[Simulate] 模拟失败: %v\n", err)
		return false, &ExecFailure{Stderr: err.Error()}
	}

	if result.Pass {
		fmt.Printf("  → 通过 (score: %.2f)\n", result.Score)
		return true, nil
	}

	fmt.Printf("  → 失败 (score: %.2f)\n", result.Score)
	for i, cp := range result.Checkpoints {
		if !cp.Met {
			fmt.Printf("    FAIL #%d: %s — %s\n", i+1, cp.Checkpoint.Description, cp.Evidence)
		}
	}

	return false, &ExecFailure{
		Stderr: fmt.Sprintf("Agent simulation failed with score %.2f", result.Score),
	}
}
```

- [ ] **步骤 4：运行测试确认通过**

```bash
go test ./internal/graph/ -v
```
预期：全部 PASS

- [ ] **步骤 5：提交**

```bash
git add internal/graph/
git commit -m "feat: 实现整体验证工作流 — 支持可执行型和指令型 skill"
```
