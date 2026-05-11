package skill

import (
	"time"
)

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"-"` //SKILL.md文件路径
	Dir         string `json:"-"` //skill所在目录
	RawContent  string `json:"-"` //原始文件内容

	Frontmatter map[string]interface{} `json:"frontmatter"` //

	//执行信息
	Steps []Step       `json:"steps"`
	Deps  []Dependency `json:"deps"`

	Purpose        string   `json:"purpose"`        //skill的核心目的
	UseCases       []string `json:"use_cases"`      //使用场景
	CorePrinciples string   `json:"corePrinciples"` //核心原则/约束
	//分类结果
	SkillType     string `json:"skillType"`     //
	HasExecutable bool   `json:"hasExecutable"` //是否包含可执行步骤
	HasGuidance   bool   `json:"hasGuidance"`   //是否包含指导原则

	//辅助文件，比如一些脚本和参考，指导原则等
	SupportFiles map[string]string `json:"supportFiles"`
}

// 从skill.md中提取到的可执行步骤
type Step struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Expected string `json:"expected"` //预期结果/输出
	Order    int    `json:"order"`
}

// 运行这个skill需要的环境
type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
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
