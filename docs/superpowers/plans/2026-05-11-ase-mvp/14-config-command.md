# 任务 14：配置命令

**目标：** 实现交互式模型配置命令，引导用户添加模型和设置路由。

**文件：**
- 创建：`internal/cli/config.go`

---

## 步骤

- [ ] **步骤 1：实现配置命令**

创建 `internal/cli/config.go`：

```go
package cli

import (
	"Agent/internal/model"
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "配置模型 Provider 和设置",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfig()
	},
}

func runConfig() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== ASE 模型配置 ===")
	fmt.Println()

	cfg := &model.Config{
		MaxRetry: 3,
		Sandbox: model.SandboxConfig{
			Image:           "golang:1.22",
			NetworkDisabled: true,
			Cleanup:         true,
		},
	}

	// 添加模型
	for {
		fmt.Print("添加模型？(y/n): ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			break
		}

		entry := model.ModelEntry{}

		fmt.Print("模型名称: ")
		entry.Name, _ = reader.ReadString('\n')
		entry.Name = strings.TrimSpace(entry.Name)

		fmt.Print("Provider (deepseek/anthropic): ")
		entry.Provider, _ = reader.ReadString('\n')
		entry.Provider = strings.TrimSpace(entry.Provider)

		fmt.Print("API Key: ")
		entry.APIKey, _ = reader.ReadString('\n')
		entry.APIKey = strings.TrimSpace(entry.APIKey)

		fmt.Print("Base URL（留空使用默认值）: ")
		entry.BaseURL, _ = reader.ReadString('\n')
		entry.BaseURL = strings.TrimSpace(entry.BaseURL)

		cfg.Models = append(cfg.Models, entry)
		fmt.Printf("  已添加模型: %s\n\n", entry.Name)
	}

	if len(cfg.Models) == 0 {
		fmt.Println("未配置模型，使用默认配置。")
		return nil
	}

	// 路由配置
	fmt.Println("=== 模型路由 ===")
	if len(cfg.Models) == 1 {
		name := cfg.Models[0].Name
		cfg.Routing = model.RoutingConfig{
			Execute: name,
			Analyze: name,
			Improve: name,
		}
		fmt.Printf("所有阶段使用: %s\n", name)
	} else {
		fmt.Println("可用模型:")
		for i, m := range cfg.Models {
			fmt.Printf("  %d. %s\n", i+1, m.Name)
		}

		for _, stage := range []string{"execute", "analyze", "improve"} {
			fmt.Printf("用于 %s 阶段的模型: ", stage)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			switch stage {
			case "execute":
				cfg.Routing.Execute = input
			case "analyze":
				cfg.Routing.Analyze = input
			case "improve":
				cfg.Routing.Improve = input
			}
		}
	}

	// 保存
	configPath, err := model.ConfigPath()
	if err != nil {
		return fmt.Errorf("获取配置路径失败: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	f, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("创建配置文件失败: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}

	fmt.Printf("\n配置已保存到 %s\n", configPath)
	return nil
}
```

- [ ] **步骤 2：验证编译**

```bash
go build ./cmd/ase/
```
预期：编译成功

- [ ] **步骤 3：提交**

```bash
git add internal/cli/config.go
git commit -m "feat: 添加交互式配置命令"
```
