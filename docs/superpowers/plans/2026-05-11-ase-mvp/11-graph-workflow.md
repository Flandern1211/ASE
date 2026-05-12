# 任务 11：Graph 工作流

**目标：** 实现 Parse → Execute → Validate → Analyze → Improve 流水线，支持重试。

**文件：**
- 创建：`internal/graph/workflow.go`
- 创建：`internal/graph/workflow_test.go`

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

type mockDockerExecutor struct {
	results []*skill.ExecResult
	idx     int
}

func (m *mockDockerExecutor) Execute(ctx context.Context, step *skill.Step, cfg interface{}) (*skill.ExecResult, error) {
	if m.idx >= len(m.results) {
		return &skill.ExecResult{ExitCode: 0, Success: true}, nil
	}
	r := m.results[m.idx]
	m.idx++
	return r, nil
}

type mockValidator struct {
	pass bool
}

func (m *mockValidator) Validate(ctx context.Context, result *skill.ExecResult, step *skill.Step) (bool, string) {
	if m.pass {
		return true, ""
	}
	return false, "validation failed"
}

type mockAnalyzer struct {
	analysis *skill.Analysis
}

func (m *mockAnalyzer) Analyze(ctx context.Context, result *skill.ExecResult, sk *skill.Skill) (*skill.Analysis, error) {
	return m.analysis, nil
}

type mockImprover struct {
	newContent string
}

func (m *mockImprover) Improve(ctx context.Context, sk *skill.Skill, analysis *skill.Analysis) (string, error) {
	return m.newContent, nil
}

func TestWorkflowAllStepsPass(t *testing.T) {
	sk := &skill.Skill{
		Name: "test",
		Steps: []skill.Step{
			{Name: "step1", Command: "echo ok", Order: 1},
		},
	}

	executor := &mockDockerExecutor{
		results: []*skill.ExecResult{
			{ExitCode: 0, Success: true, Stdout: "ok"},
		},
	}
	validator := &mockValidator{pass: true}

	wf := NewWorkflow(executor, validator, nil, nil)
	result := wf.Run(context.Background(), sk, nil)

	if !result.Success {
		t.Error("expected workflow to succeed")
	}
	if result.Retries != 0 {
		t.Errorf("expected 0 retries, got %d", result.Retries)
	}
}

func TestWorkflowRetryAndRecover(t *testing.T) {
	sk := &skill.Skill{
		Name: "test",
		Steps: []skill.Step{
			{Name: "step1", Command: "echo ok", Order: 1},
		},
	}

	executor := &mockDockerExecutor{
		results: []*skill.ExecResult{
			{ExitCode: 1, Success: false, Stderr: "error"},
			{ExitCode: 0, Success: true, Stdout: "ok"},
		},
	}
	validator := &mockValidator{pass: false}
	analyzer := &mockAnalyzer{
		analysis: &skill.Analysis{Reason: "bad", Suggest: "fix it"},
	}
	improver := &mockImprover{newContent: "improved"}

	wf := NewWorkflow(executor, validator, analyzer, improver)
	result := wf.Run(context.Background(), sk, &Config{MaxRetries: 3})

	if !result.Success {
		t.Error("expected workflow to succeed after retry")
	}
	if result.Retries != 1 {
		t.Errorf("expected 1 retry, got %d", result.Retries)
	}
}

func TestWorkflowMaxRetriesExhausted(t *testing.T) {
	sk := &skill.Skill{
		Name: "test",
		Steps: []skill.Step{
			{Name: "step1", Command: "fail", Order: 1},
		},
	}

	executor := &mockDockerExecutor{
		results: []*skill.ExecResult{
			{ExitCode: 1, Success: false, Stderr: "fail"},
			{ExitCode: 1, Success: false, Stderr: "fail"},
		},
	}
	validator := &mockValidator{pass: false}
	analyzer := &mockAnalyzer{
		analysis: &skill.Analysis{Reason: "bad", Suggest: "fix"},
	}
	improver := &mockImprover{newContent: "still bad"}

	wf := NewWorkflow(executor, validator, analyzer, improver)
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
	Success     bool
	Retries     int
	TotalSteps  int
	StepsPassed int
	Results     []*skill.ExecResult
	Analyses    []*skill.Analysis
	Duration    time.Duration
}

// Executor 执行单个步骤。
type Executor interface {
	Execute(ctx context.Context, step *skill.Step, cfg interface{}) (*skill.ExecResult, error)
}

// Validator 检查步骤结果是否符合预期。
type Validator interface {
	Validate(ctx context.Context, result *skill.ExecResult, step *skill.Step) (bool, string)
}

// Analyzer 诊断步骤失败原因。
type Analyzer interface {
	Analyze(ctx context.Context, result *skill.ExecResult, sk *skill.Skill) (*skill.Analysis, error)
}

// Improver 根据分析修改 skill 文件。
type Improver interface {
	Improve(ctx context.Context, sk *skill.Skill, analysis *skill.Analysis) (string, error)
}

// Workflow 编排：Parse → Execute → Validate → Analyze → Improve
type Workflow struct {
	executor  Executor
	validator Validator
	analyzer  Analyzer
	improver  Improver
}

// NewWorkflow 创建工作流。
func NewWorkflow(e Executor, v Validator, a Analyzer, imp Improver) *Workflow {
	return &Workflow{
		executor:  e,
		validator: v,
		analyzer:  a,
		improver:  imp,
	}
}

// Run 执行完整流水线：所有步骤，失败时重试。
func (w *Workflow) Run(ctx context.Context, sk *skill.Skill, cfg *Config) *WorkflowResult {
	start := time.Now()
	maxRetries := 3
	if cfg != nil && cfg.MaxRetries > 0 {
		maxRetries = cfg.MaxRetries
	}

	result := &WorkflowResult{
		TotalSteps: len(sk.Steps),
	}

	retries := 0
	allPassed := false

	for retries <= maxRetries {
		stepResults := make([]*skill.ExecResult, 0, len(sk.Steps))
		passed := 0
		failed := false

		for i := range sk.Steps {
			step := &sk.Steps[i]

			fmt.Printf("[Execute]  Step %d: %s... ", step.Order, step.Name)

			execResult, err := w.executor.Execute(ctx, step, cfg)
			if err != nil {
				fmt.Printf("ERROR (%v)\n", err)
				execResult = &skill.ExecResult{
					StepName: step.Name,
					ExitCode: -1,
					Stderr:   err.Error(),
					Success:  false,
				}
			}

			stepResults = append(stepResults, execResult)

			if w.validator != nil {
				ok, diff := w.validator.Validate(ctx, execResult, step)
				if ok {
					fmt.Println("OK")
					passed++
				} else {
					fmt.Printf("FAILED (%s)\n", diff)
					failed = true
					break
				}
			} else {
				if execResult.Success {
					fmt.Println("OK")
					passed++
				} else {
					fmt.Printf("FAILED (exit %d)\n", execResult.ExitCode)
					failed = true
					break
				}
			}
		}

		result.Results = stepResults
		result.StepsPassed = passed

		if !failed {
			allPassed = true
			break
		}

		if retries >= maxRetries {
			break
		}

		if w.analyzer != nil && w.improver != nil {
			lastFailed := stepResults[len(stepResults)-1]

			fmt.Println("[Analyze]  分析失败原因...")
			analysis, err := w.analyzer.Analyze(ctx, lastFailed, sk)
			if err != nil {
				fmt.Printf("[Analyze]  错误: %v\n", err)
				break
			}
			result.Analyses = append(result.Analyses, analysis)
			fmt.Printf("  原因: %s\n  建议: %s\n", analysis.Reason, analysis.Suggest)

			fmt.Println("[Improve]  修改 skill 文件...")
			newContent, err := w.improver.Improve(ctx, sk, analysis)
			if err != nil {
				fmt.Printf("[Improve]  错误: %v\n", err)
				break
			}

			// 重新解析改进后的内容
			reparsed, err := skill.ParseContent(newContent, sk.Path)
			if err != nil {
				fmt.Printf("[Improve]  重新解析失败: %v\n", err)
				break
			}
			*sk = *reparsed
			fmt.Printf("  → Skill 已更新到版本 %d\n", sk.Version)
		}

		retries++
	}

	result.Success = allPassed
	result.Retries = retries
	result.Duration = time.Since(start)

	return result
}
```

- [ ] **步骤 4：运行测试确认通过**

```bash
go test ./internal/graph/ -v
```
预期：PASS

- [ ] **步骤 5：提交**

```bash
git add internal/graph/
git commit -m "feat: 实现 eino Graph 工作流流水线"
```
