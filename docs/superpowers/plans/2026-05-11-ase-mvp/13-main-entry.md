# 任务 13：入口点

**目标：** 创建 CLI 入口点，连接所有组件。

**文件：**
- 创建：`cmd/ase/main.go`

---

## 步骤

- [ ] **步骤 1：创建 main.go**

创建 `cmd/ase/main.go`：

```go
package main

import (
	"Agent/internal/cli"
	"os"
)

func main() {
	if err := cli.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **步骤 2：构建并验证二进制文件**

```bash
go build -o ./bin/ase ./cmd/ase/
```
预期：生成 `bin/ase` 二进制文件

- [ ] **步骤 3：测试基础帮助输出**

```bash
./bin/ase --help
```
预期输出：
```
Aegis Skill Engine - 测试和改进 skill 文件

Usage:
  ase [command]

Available Commands:
  config      配置模型和设置
  help        帮助信息
  history     查看 skill 的执行历史
  info        查看 skill 详情
  improve     测试并自动改进 skill 文件
  list        列出所有已测试的 skill
  test        测试 skill 文件
```

- [ ] **步骤 4：提交**

```bash
git add cmd/ase/main.go
git commit -m "feat: 添加 CLI 入口点"
```
