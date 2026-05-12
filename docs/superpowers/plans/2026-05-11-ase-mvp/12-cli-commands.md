# 任务 12：CLI 命令

**目标：** 使用 Cobra 实现 test、improve、info、list、history 等 CLI 命令。

**文件：**
- 创建：`internal/cli/commands.go`

---

## 步骤

- [ ] **步骤 1：实现 CLI 命令**

创建 `internal/cli/commands.go`：

```go
package cli

import (
	"Agent/internal/graph"
	"Agent/internal/model"
	"Agent/internal/sandbox"
	"Agent/internal/skill"
	"Agent/internal/storage"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "ase",
	Short: "Aegis Skill Engine - 测试和改进 skill 文件",
}

func init() {
	RootCmd.AddCommand(testCmd)
	RootCmd.AddCommand(improveCmd)
	RootCmd.AddCommand(configCmd)
	RootCmd.AddCommand(infoCmd)
	RootCmd.AddCommand(listCmd)
	RootCmd.AddCommand(historyCmd)
}

var testCmd = &cobra.Command{
	Use:   "test [path]",
	Short: "测试 skill 文件",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTest(args[0], false)
	},
}

var improveCmd = &cobra.Command{
	Use:   "improve [path]",
	Short: "测试并自动改进 skill 文件",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTest(args[0], true)
	},
}

var infoCmd = &cobra.Command{
	Use:   "info [path]",
	Short: "查看 skill 详情",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sk, err := skill.Parse(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("[Parse]    Skill: %s v%d\n", sk.Name, sk.Version)
		fmt.Printf("  描述: %s\n", sk.Description)
		fmt.Printf("  标签: %v\n", sk.Tags)
		fmt.Printf("  步骤: %d\n", len(sk.Steps))
		for _, s := range sk.Steps {
			fmt.Printf("    Step %d: %s\n", s.Order, s.Name)
		}
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有已测试的 skill",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := storage.NewDB(dbPath())
		if err != nil {
			return err
		}
		defer db.Close()

		skills, err := db.ListSkills()
		if err != nil {
			return err
		}

		if len(skills) == 0 {
			fmt.Println("尚无已测试的 skill。")
			return nil
		}

		fmt.Printf("%-30s %-8s %-6s %-8s %-20s\n", "名称", "版本", "测试数", "通过率", "最后测试时间")
		fmt.Println("---")
		for _, s := range skills {
			fmt.Printf("%-30s %-8d %-6d %-8.0f%% %-20s\n",
				s.Name, s.Version, s.TotalTests, s.SuccessRate*100,
				s.LastTestedAt.Format("2006-01-02 15:04"))
		}
		return nil
	},
}

var historyCmd = &cobra.Command{
	Use:   "history [path]",
	Short: "查看 skill 的执行历史",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := storage.NewDB(dbPath())
		if err != nil {
			return err
		}
		defer db.Close()

		records, err := db.GetHistory(args[0], 10)
		if err != nil {
			return err
		}

		if len(records) == 0 {
			fmt.Println("未找到执行历史。")
			return nil
		}

		for _, r := range records {
			fmt.Printf("[%s] %s | %d/%d 步骤 | %d 次重试 | %dms\n",
				r.CreatedAt.Format("2006-01-02 15:04"),
				r.Status, r.StepsPassed, r.StepsTotal,
				r.Retries, r.DurationMs)
		}
		return nil
	},
}

func runTest(path string, withImprove bool) error {
	start := time.Now()

	// 解析 skill
	fmt.Printf("[Parse]    解析 skill: %s...\n", path)
	sk, err := skill.Parse(path)
	if err != nil {
		return fmt.Errorf("[Parse] 失败: %w", err)
	}
	fmt.Printf("[Parse]    OK（发现 %d 个步骤）\n", len(sk.Steps))

	// 加载配置
	cfg, err := model.LoadConfig()
	if err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}

	// 创建沙箱配置
	sbCfg := sandbox.Config{
		Image:           cfg.Sandbox.Image,
		NetworkDisabled: cfg.Sandbox.NetworkDisabled,
		TimeoutSeconds:  300,
	}

	// 创建工具
	dockerTool := newDockerToolWrapper(sbCfg)
	validateTool := newValidateToolWrapper()

	var analyzer graph.Analyzer
	var improver graph.Improver

	if withImprove {
		reg, err := model.NewRegistry(cfg)
		if err != nil {
			return fmt.Errorf("模型注册表错误: %w", err)
		}

		analyzeModel, err := reg.GetForStage("analyze")
		if err != nil {
			return fmt.Errorf("未配置分析阶段模型: %w", err)
		}
		improveModel, err := reg.GetForStage("improve")
		if err != nil {
			return fmt.Errorf("未配置改进阶段模型: %w", err)
		}

		analyzer = newAnalyzeToolWrapper(analyzeModel)
		improver = newImproveToolWrapper(improveModel)
	}

	// 运行工作流
	wf := graph.NewWorkflow(dockerTool, validateTool, analyzer, improver)
	result := wf.Run(context.Background(), sk, &graph.Config{MaxRetries: cfg.MaxRetry, SandboxConfig: sbCfg})

	// 输出结果
	if result.Success {
		fmt.Printf("[Result]   通过")
		if result.Retries > 0 {
			fmt.Printf("（%d 次自动修复）", result.Retries)
		}
		fmt.Println()
	} else {
		fmt.Printf("[Result]   失败，重试 %d 次后放弃（耗时 %v）\n", result.Retries, result.Duration)
	}

	// 记录到存储
	db, err := storage.NewDB(dbPath())
	if err == nil {
		defer db.Close()
		status := "fail"
		if result.Success {
			if result.Retries > 0 {
				status = "improved"
			} else {
				status = "pass"
			}
		}
		db.RecordExecution(recordFromResult(sk, result, status))
	}

	return nil
}

// recordFromResult 从工作流结果创建执行记录。
func recordFromResult(sk *skill.Skill, result *graph.WorkflowResult, status string) skill.ExecutionRecord {
	return skill.ExecutionRecord{
		SkillPath:    sk.Path,
		SkillVersion: sk.Version,
		Status:       status,
		StepsTotal:   result.TotalSteps,
		StepsPassed:  result.StepsPassed,
		Retries:      result.Retries,
		DurationMs:   result.Duration.Milliseconds(),
		CreatedAt:    time.Now(),
	}
}

func dbPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "ase.db"
	}
	return home + "/.ase/ase.db"
}

// --- 适配器：将内部工具适配到 graph 接口 ---

type dockerToolWrapper struct {
	tool *dockerToolAdapter
}

func newDockerToolWrapper(cfg sandbox.Config) *dockerToolWrapper {
	return &dockerToolWrapper{tool: &dockerToolAdapter{cfg: cfg}}
}

func (w *dockerToolWrapper) Execute(ctx context.Context, step *skill.Step, cfg interface{}) (*skill.ExecResult, error) {
	return w.tool.execute(ctx, step)
}

type dockerToolAdapter struct {
	cfg sandbox.Config
}

func (a *dockerToolAdapter) execute(ctx context.Context, step *skill.Step) (*skill.ExecResult, error) {
	sb, err := sandbox.New(ctx)
	if err != nil {
		return nil, err
	}
	defer sb.Close()

	start := time.Now()
	out, err := sb.RunCommand(ctx, a.cfg, step.Command)
	if err != nil {
		return nil, err
	}

	return &skill.ExecResult{
		StepName: step.Name,
		ExitCode: out.ExitCode,
		Stdout:   out.Stdout,
		Stderr:   out.Stderr,
		Success:  out.ExitCode == 0,
		Duration: time.Since(start),
	}, nil
}

type validateToolWrapper struct{}

func newValidateToolWrapper() *validateToolWrapper {
	return &validateToolWrapper{}
}

func (w *validateToolWrapper) Validate(ctx context.Context, result *skill.ExecResult, step *skill.Step) (bool, string) {
	if result.ExitCode != 0 {
		return false, fmt.Sprintf("退出码 %d", result.ExitCode)
	}
	return true, ""
}

type analyzeToolWrapper struct {
	model model.ChatModel
}

func newAnalyzeToolWrapper(m model.ChatModel) *analyzeToolWrapper {
	return &analyzeToolWrapper{model: m}
}

func (w *analyzeToolWrapper) Analyze(ctx context.Context, result *skill.ExecResult, sk *skill.Skill) (*skill.Analysis, error) {
	return &skill.Analysis{
		StepName: result.StepName,
		Reason:   result.Stderr,
		Suggest:  "检查错误输出",
	}, nil
}

type improveToolWrapper struct {
	model model.ChatModel
}

func newImproveToolWrapper(m model.ChatModel) *improveToolWrapper {
	return &improveToolWrapper{model: m}
}

func (w *improveToolWrapper) Improve(ctx context.Context, sk *skill.Skill, analysis *skill.Analysis) (string, error) {
	return sk.RawContent, nil
}
```

- [ ] **步骤 2：验证编译**

```bash
go build ./internal/cli/
```
注意：适配器使用简化接口，真实实现将在整合工具时完善。现在验证包可以编译。

- [ ] **步骤 3：提交**

```bash
git add internal/cli/
git commit -m "feat: 添加 test、improve、info、list、history CLI 命令"
```
