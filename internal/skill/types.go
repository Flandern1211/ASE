package skill

import (
	"time"
)

// Skill 是 SKILL.md 的元数据容器。
// 执行时 LLM 直接读取 RawContent，不依赖预解析的 Steps。
type Skill struct {
	// 身份信息
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"-"` // SKILL.md 文件路径
	Dir         string `json:"-"` // skill 所在目录

	// 全文内容（执行时使用）
	RawContent string `json:"-"` // 完整 SKILL.md 文本

	// Frontmatter（原样保留，供改进时回写）
	Frontmatter map[string]interface{} `json:"frontmatter"`

	// 元数据（用于路由/索引/分类）
	Deps           []Dependency `json:"deps"`
	Purpose        string       `json:"purpose"`
	UseCases       []string     `json:"use_cases"`
	CorePrinciples string       `json:"core_principles"`
	SkillType      string       `json:"skill_type"` // "executable", "instructional", "mixed"
	HasExecutable  bool         `json:"has_executable"`
	HasGuidance    bool         `json:"has_guidance"`

	// 辅助文件（执行时使用）
	SupportFiles map[string]string `json:"support_files"` // 相对路径 → 文件内容
}

// Step 从 SKILL.md 中提取的可执行步骤。
// Parser 不填充此类型；Docker 执行工具在运行时按需使用。
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
