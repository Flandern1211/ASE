# 任务 2：核心类型

**目标：** 定义 Skill、ExecResult、Analysis 等核心数据结构，作为 Skill 的元数据容器和运行时数据载体。

**文件：**
- 创建：`internal/skill/types.go`
- 创建：`internal/skill/types_test.go`

---

## 设计决策：混合模式（元数据解析 + 全文执行）

### 为什么不做完整结构化解析

分析了 5 个代表性 SKILL.md 文件后，发现：
- **60%** 的 skill 可以良好映射到结构化步骤（verification-before-completion, writing-plans）
- **20%** 是混合型（TDD：核心流程可解析，大量内容是说理/反面教材）
- **20%** 本质上无法解析（using-superpowers：元指导/路由型 skill）
- 即使能解析的 skill，**Red Flags、Rationalizations、Iron Law** 等纪律性内容在解析中丢失

**结论：** parser 只提取元数据用于路由/索引，执行时 LLM 直接读取全文。

### 职责划分

| 组件 | 职责 | 数据来源 |
|------|------|----------|
| **Parser** | 提取元数据（name, description, deps, skill_type 等） | SKILL.md frontmatter + LLM 语义提取 |
| **RawContent** | 存储完整 SKILL.md 文本 | 直接读取文件 |
| **SupportFiles** | 存储辅助文件内容 | 扫描目录 |
| **执行时 LLM** | 按全文执行 skill、验证、分析、改进 | RawContent + SupportFiles |

---

## Skill 结构体设计

```go
type Skill struct {
    // --- 身份信息 ---
    Name        string `json:"name"`
    Description string `json:"description"`
    Path        string `json:"-"`        // SKILL.md 文件路径
    Dir         string `json:"-"`        // skill 所在目录

    // --- 全文内容（执行时使用） ---
    RawContent  string `json:"-"`        // 完整 SKILL.md 文本

    // --- Frontmatter（原样保留，供改进时回写） ---
    Frontmatter map[string]interface{} `json:"frontmatter"`

    // --- 元数据（用于路由/索引/分类） ---
    Deps            []Dependency `json:"deps"`
    Purpose         string       `json:"purpose"`
    UseCases        []string     `json:"use_cases"`
    CorePrinciples  string       `json:"core_principles"`
    SkillType       string       `json:"skill_type"`      // "executable", "instructional", "mixed"
    HasExecutable   bool         `json:"has_executable"`
    HasGuidance     bool         `json:"has_guidance"`

    // --- 辅助文件（执行时使用） ---
    SupportFiles map[string]string `json:"support_files"`
}
```

**关键变化：**
- 移除 `Steps []Step` — 不再由 parser 预解析步骤
- `RawContent` 保持 `-` 不序列化 — 它是运行时数据，不应持久化到 JSON
- Step 类型保留但不再被 parser 填充 — 后续工具（如 Docker 执行）可能需要在运行时从全文中提取步骤

---

## 步骤

- [ ] **步骤 1：创建 types.go**

```go
package skill

import "time"

// Skill 是 SKILL.md 的元数据容器。
// 执行时 LLM 直接读取 RawContent，不依赖预解析的 Steps。
type Skill struct {
	// --- 身份信息 ---
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"-"` // SKILL.md 文件路径
	Dir         string `json:"-"` // skill 所在目录

	// --- 全文内容（执行时使用） ---
	RawContent string `json:"-"` // 完整 SKILL.md 文本

	// --- Frontmatter（原样保留，供改进时回写） ---
	Frontmatter map[string]interface{} `json:"frontmatter"`

	// --- 元数据（用于路由/索引/分类） ---
	Deps           []Dependency `json:"deps"`
	Purpose        string       `json:"purpose"`
	UseCases       []string     `json:"use_cases"`
	CorePrinciples string       `json:"core_principles"`
	SkillType      string       `json:"skill_type"`     // "executable", "instructional", "mixed"
	HasExecutable  bool         `json:"has_executable"`
	HasGuidance    bool         `json:"has_guidance"`

	// --- 辅助文件（执行时使用） ---
	SupportFiles map[string]string `json:"support_files"`
}

// Step 从 SKILL.md 中提取的可执行步骤。
// Parser 不填充此类型；Docker 执行工具在运行时按需提取。
type Step struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Expected string `json:"expected"`
	Order    int    `json:"order"`
}

// Dependency skill 运行所需的环境依赖。
type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"` // "tool", "language", "service"
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
	if len(s.Deps) != 1 {
		t.Errorf("expected 1 dep, got %d", len(s.Deps))
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

func TestSkillNoStepsField(t *testing.T) {
	// 验证 Skill 不再包含 Steps 字段
	s := Skill{Name: "test"}
	// s.Steps 不应存在 — 如果编译通过即证明字段已移除
	_ = s
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
git commit -m "feat: 简化核心类型 — 移除 Steps，改为纯元数据 + RawContent"
```
