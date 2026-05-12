# 任务 2：核心类型

**目标：** 定义 Skill、ExecResult、Analysis 等核心数据结构，作为 LLM 解析 SKILL.md 后的输出容器。

**文件：**
- 创建：`internal/skill/types.go`
- 创建：`internal/skill/types_test.go`

---

## 设计决策：为什么用 LLM 解析

SKILL.md 格式是**半结构化**的：
- Frontmatter 字段不固定（必须有 name/description，其他可选）
- Markdown body 完全自由格式（步骤、指令、流程图、代码示例混杂）
- 目录结构不固定（可能有 scripts/、tests/、config/ 等子目录）

**结论：** 不适合用固定 struct + yaml tag 做硬解析。改用 LLM 做语义解析，struct 仅作为 LLM 输出的容器。

### 解析流程

```
SKILL.md + 辅助文件 → 读取原始内容 → LLM 提取结构化信息 → Skill struct
```

### LLM 需要提取的信息

| 信息类别 | 说明 | 用途 |
|---|---|---|
| 元数据 | name, description, argument-hint 等 | 识别和展示 |
| 执行步骤 | 可执行的 shell 命令、预期结果 | Docker 沙箱执行 |
| 环境依赖 | 需要的工具、语言版本、Docker | 选择基础镜像 |
| 语义理解 | skill 的用途、适用场景、核心原则 | 改进时保持意图 |
| 辅助文件 | scripts/、tests/ 等文件内容 | 完整执行上下文 |

---

## 步骤

- [ ] **步骤 1：创建 types.go**

```go
package skill

import "time"

// Skill 是 LLM 解析 SKILL.md 后的输出容器。
// 字段设计围绕 ASE 需要"对 skill 做什么"，而非 SKILL.md 的格式。
type Skill struct {
	// --- 身份信息 ---
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"-"`        // SKILL.md 文件路径
	Dir         string `json:"-"`        // skill 所在目录
	RawContent  string `json:"-"`        // 原始文件内容

	// --- Frontmatter（原样保留，供 LLM 改进时回写） ---
	Frontmatter map[string]interface{} `json:"frontmatter"`

	// --- 执行信息 ---
	Steps    []Step           `json:"steps"`
	Deps     []Dependency     `json:"deps"`

	// --- 语义理解 ---
	Purpose        string   `json:"purpose"`         // skill 的核心目的
	UseCases       []string `json:"use_cases"`       // 适用场景
	CorePrinciples string   `json:"core_principles"` // 核心原则/约束

	// --- 分类结果 ---
	SkillType     string `json:"skill_type"`      // "executable", "instructional", "mixed"
	HasExecutable bool   `json:"has_executable"`  // 是否包含可执行步骤
	HasGuidance   bool   `json:"has_guidance"`    // 是否包含指导原则

	// --- 辅助文件 ---
	SupportFiles map[string]string `json:"support_files"` // 相对路径 → 文件内容
}

// Step 从 SKILL.md body 中提取的可执行步骤。
type Step struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Expected string `json:"expected"` // 预期输出/结果
	Order    int    `json:"order"`
}

// Dependency skill 运行所需的环境依赖。
type Dependency struct {
	Name    string `json:"name"`    // 工具名，如 "go", "docker"
	Version string `json:"version"` // 版本要求，如 ">=1.21"
	Type    string `json:"type"`    // "tool", "language", "service"
}

// TestScenario 测试场景（用于 Agent 模拟测试）。
type TestScenario struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Steps       []string     `json:"steps"`
	Checkpoints []Checkpoint `json:"checkpoints"`
}

// Checkpoint 行为检查点。
type Checkpoint struct {
	Description string `json:"description"`
	Type        string `json:"type"` // "must_do", "must_not", "order", "output"
	Required    bool   `json:"required"`
}

// ExecResult 保存 skill 单次执行的结果。
type ExecResult struct {
	SkillName string
	StepName  string
	ExitCode  int
	Stdout    string
	Stderr    string
	Success   bool
	Duration  time.Duration
}

// Analysis 保存 LLM 对验证失败的诊断结果。
type Analysis struct {
	SkillName string
	StepName  string
	Reason    string
	Suggest   string
	FixType   string // "command", "instruction", "dependency"
}

// SkillMeta 保存 skill 的追踪元数据（持久化统计）。
type SkillMeta struct {
	Path         string
	Name         string
	TotalTests   int
	PassedTests  int
	SuccessRate  float64
	LastTestedAt time.Time
}

// ExecutionRecord 存储单次执行历史记录。
type ExecutionRecord struct {
	ID         int64
	SkillPath  string
	Status     string // "pass", "fail", "improved"
	ModelUsed  string
	DurationMs int64
	CreatedAt  time.Time
	ErrorLog   string
	Analysis   string
}
```

- [ ] **步骤 2：编写验证类型的测试**

创建 `internal/skill/types_test.go`：

```go
package skill

import (
	"testing"
	"time"
)

func TestSkillFields(t *testing.T) {
	s := Skill{
		Name:        "test-skill",
		Description: "Use when testing skill parsing",
		Path:        "/skills/test-skill/SKILL.md",
		Dir:         "/skills/test-skill/",
		Frontmatter: map[string]interface{}{
			"name":        "test-skill",
			"description": "Use when testing skill parsing",
			"version":     1,
		},
		Steps: []Step{
			{Name: "step1", Command: "echo hello", Expected: "hello", Order: 1},
		},
		Deps: []Dependency{
			{Name: "go", Version: ">=1.21", Type: "language"},
		},
		Purpose:        "Test skill parsing functionality",
		UseCases:       []string{"unit testing", "integration testing"},
		CorePrinciples: "Parse accurately",
		SkillType:      "mixed",
		HasExecutable:  true,
		HasGuidance:    true,
		SupportFiles:   map[string]string{"scripts/setup.sh": "#!/bin/bash\necho setup"},
	}

	if s.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", s.Name)
	}
	if len(s.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(s.Steps))
	}
	if len(s.Deps) != 1 {
		t.Errorf("expected 1 dep, got %d", len(s.Deps))
	}
	if s.Steps[0].Command != "echo hello" {
		t.Errorf("expected command 'echo hello', got %q", s.Steps[0].Command)
	}
	if s.SkillType != "mixed" {
		t.Errorf("expected skill_type 'mixed', got %q", s.SkillType)
	}
	if !s.HasExecutable {
		t.Error("expected has_executable=true")
	}
	if !s.HasGuidance {
		t.Error("expected has_guidance=true")
	}
}

func TestTestScenario(t *testing.T) {
	ts := TestScenario{
		Name:        "test-scenario",
		Description: "Test if agent follows TDD",
		Steps:       []string{"Write failing test", "Run test", "Write code"},
		Checkpoints: []Checkpoint{
			{Description: "writes test first", Type: "must_do", Required: true},
			{Description: "runs test before code", Type: "order", Required: true},
		},
	}

	if ts.Name != "test-scenario" {
		t.Errorf("expected name 'test-scenario', got %q", ts.Name)
	}
	if len(ts.Checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(ts.Checkpoints))
	}
}

func TestExecResult(t *testing.T) {
	r := ExecResult{
		SkillName: "test-skill",
		StepName:  "step1",
		ExitCode:  0,
		Stdout:    "ok",
		Success:   true,
		Duration:  100 * time.Millisecond,
	}

	if !r.Success {
		t.Error("expected success=true")
	}
}

func TestAnalysis(t *testing.T) {
	a := Analysis{
		SkillName: "test-skill",
		StepName:  "step1",
		Reason:    "missing dependency",
		Suggest:   "add go mod tidy step",
		FixType:   "command",
	}

	if a.Reason == "" {
		t.Error("expected non-empty reason")
	}
	if a.FixType != "command" {
		t.Errorf("expected fix_type 'command', got %q", a.FixType)
	}
}
```

- [ ] **步骤 3：运行测试**

```bash
go test ./internal/skill/ -v
```
预期：PASS

- [ ] **步骤 4：提交**

```bash
git add internal/skill/types.go internal/skill/types_test.go
git commit -m "feat: 添加基于 LLM 解析的 Skill、ExecResult 核心类型"
```
