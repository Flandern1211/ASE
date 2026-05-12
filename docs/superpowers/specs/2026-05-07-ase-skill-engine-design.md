# Aegis Skill Engine (ASE) — 设计文档

**日期**: 2026-05-07
**版本**: MVP (1.0)
**技术栈**: Go + eino (CloudWeGo) + Docker

---

## 1. 概述

ASE 是一个 CLI 工具，帮助用户**测试和自动改进 skill 文件**。用户编写描述工作流步骤的 markdown 文件，ASE 在 Docker 沙箱中执行这些步骤，验证结果，并在失败时通过 LLM 自动分析原因、修改 skill 文件。

### 核心能力

- **测试 (test)**: 在 Docker 沙箱中执行 skill 步骤，验证是否通过
- **改进 (improve)**: 测试失败时，自动分析原因并修改 skill 文件，重试直到通过
- **模型路由**: 支持为不同阶段（执行/分析/改进）分配不同的 LLM
- **历史追踪**: 记录每次执行的结果、成功率、改进历史

### 设计原则

- **渐进式架构**: MVP 用 eino Graph 实现线性流水线，关键接口预留好，后期自然演进为插件引擎
- **模型抽象**: 通过 eino ChatModel 接口统一调用，MVP 用 DeepSeek，后期可扩展
- **安全第一**: 所有 skill 执行都在 Docker 沙箱中进行，容器自动清理

---

## 2. 项目结构

```
Agent/
├── cmd/
│   └── ase/                    # CLI 入口
│       └── main.go
├── internal/
│   ├── skill/
│   │   ├── parser.go           # 解析 skill.md → 结构化步骤
│   │   └── types.go            # Skill、Step、Result 等核心类型
│   ├── graph/
│   │   └── workflow.go         # eino Graph 定义：Parse→Execute→Validate→Analyze→Improve
│   ├── tools/
│   │   ├── docker_tool.go      # Docker 执行 Tool
│   │   ├── analyze_tool.go     # LLM 分析 Tool
│   │   ├── improve_tool.go     # LLM 改进 Tool
│   │   └── validate_tool.go    # 结果验证 Tool
│   ├── model/
│   │   ├── provider.go         # 模型配置管理
│   │   └── deepseek.go         # DeepSeek ChatModel 实现
│   ├── sandbox/
│   │   ├── docker.go           # Docker SDK 封装
│   │   └── types.go            # 容器配置、执行结果类型
│   ├── storage/
│   │   ├── history.go          # 执行历史存储
│   │   └── db.go               # SQLite 存储
│   └── cli/
│       └── commands.go         # CLI 命令定义 (cobra)
├── skills/                     # 用户 skill 文件目录
│   └── example/
│       ├── skill.md
│       ├── scripts/
│       ├── tests/
│       └── config/
├── config.yaml                 # 全局配置（模型、默认参数）
└── go.mod
```

---

## 3. Skill 文件格式

### 3.1 单文件形式

```markdown
---
name: go-test-build
description: 运行 Go 测试并构建项目
version: 1
tags: [go, test, build]
expected_env:
  - go: ">=1.21"
  - docker: true
---

# Go Test & Build

## 步骤

### Step 1: 运行测试
​```bash
go test ./... -v
​```
**预期**: 所有测试通过，退出码为 0

### Step 2: 构建项目
​```bash
go build -o ./bin/app .
​```
**预期**: 生成 `./bin/app` 文件

## 回滚步骤（可选）
​```bash
rm -rf ./bin
​```
```

### 3.2 目录形式

```
skills/
├── go-test-build/
│   ├── skill.md                # 主定义文件（必须）
│   ├── scripts/                # 辅助脚本（可选）
│   │   └── setup.sh
│   ├── tests/                  # 测试文件（可选）
│   │   └── check.go
│   └── config/                 # 配置文件（可选）
│       └── env.yaml
```

### 3.3 解析规则

- 路径是文件 → 直接解析该 `.md` 文件
- 路径是目录 → 找到 `skill.md` 作为主文件，其他文件作为上下文
- `scripts/` 下的文件可在 skill 步骤中被引用
- `tests/` 下的文件是验证脚本，执行完 skill 步骤后自动运行
- `config/` 下的文件是环境配置，执行前注入到 Docker

### 3.4 Frontmatter 字段

| 字段 | 类型 | 必须 | 说明 |
|---|---|---|---|
| name | string | 是 | skill 名称 |
| description | string | 是 | 一句话描述 |
| version | int | 是 | 版本号，每次改进自增 |
| tags | []string | 否 | 标签，用于分类 |
| expected_env | []map | 是 | 依赖环境（语言版本、工具等） |

---

## 4. 核心引擎 — eino Graph 工作流

### 4.1 流水线结构

使用 eino 的 Graph 组件编排工作流，每个阶段是一个 Graph 节点：

```
Parse → Execute → Validate → [成功] → 完成
                          ↓ [失败]
                       Analyze → Improve → Execute (重试，最多 N 次)
```

### 4.2 节点定义

| 节点 | 输入 | 输出 | 说明 |
|---|---|---|---|
| Parse | skill.md 路径 | Skill 结构体 | 解析 markdown 为结构化步骤 |
| Execute | Step + Docker 配置 | ExecResult | 在 Docker 沙箱中执行单个步骤 |
| Validate | ExecResult + 预期 | bool + 差异 | 检查退出码、输出是否符合预期 |
| Analyze | ExecResult + Skill | Analysis | LLM 分析失败原因 |
| Improve | Skill + Analysis | Skill | LLM 修改 skill.md |

### 4.3 条件路由

- Validate 通过 → 流程结束，报告成功
- Validate 失败 → 进入 Analyze → Improve → 重新 Execute
- 重试次数超过上限（默认 3 次）→ 报告失败，输出所有尝试记录

### 4.4 关键接口（为后期插件化预留）

```go
// Tool — eino 标准 Tool 接口，封装具体能力
// DockerTool: 执行 shell 命令
// AnalyzeTool: LLM 分析失败原因
// ImproveTool: LLM 修改 skill 文件

// ChatModel — eino 标准模型接口
// 后期可实现多个 Provider（DeepSeek、Claude、Qwen 等）
// 通过配置切换
```

---

## 5. Docker 沙箱

### 5.1 设计原则

- 每次 skill 执行都在干净容器中进行，执行完销毁
- 使用 Go Docker SDK (`github.com/docker/docker/client`)

### 5.2 容器配置

```
宿主机                          Docker 容器
┌─────────────┐               ┌──────────────────────┐
│ skill.md    │──mount──>     │ /workspace/skill.md  │
│ scripts/    │               │ /workspace/scripts/  │
│ tests/      │               │ /workspace/tests/    │
│ config/     │               │ /workspace/config/   │
└─────────────┘               │                      │
                              │ 执行 Step 1, 2, 3... │
                              │ 输出: stdout, stderr  │
                              │ 退出码               │
                              └──────────────────────┘
```

### 5.3 执行流程

1. 根据 `expected_env` 选择基础镜像（MVP: `golang:1.22`, `ubuntu:22.04`）
2. 创建容器，挂载 skill 目录到 `/workspace`
3. 每个 Step 单独执行（不是整个 skill 一次性跑完）
4. 逐步检查结果，失败时立即停止并收集错误信息
5. 容器执行完自动清理（`docker rm`）

### 5.4 安全措施

- 容器内不以 root 运行
- 网络默认关闭（除非 skill 声明需要）
- 文件系统限制：只挂载 skill 目录

---

## 6. 模型配置

### 6.1 配置流程

首次运行 `ase test` 或 `ase improve` 时，如果未检测到配置文件，自动引导用户完成配置：

1. **添加模型**: 输入模型名称、Provider、API Key、Base URL（可添加多个）
2. **选择路由模式**:
   - **全局配置**: 所有阶段（执行/分析/改进）使用同一个模型
   - **分阶段配置**: 为执行、分析、改进阶段分别选择模型（只能选已添加的模型）

执行 `ase config` 可随时重新配置。配置存储在 `~/.ase/config.yaml`。

### 6.2 模型抽象

通过 eino 的 `ChatModel` 接口统一调用：

```go
// 统一的模型调用接口
type ChatModel interface {
    Generate(ctx context.Context, messages []*schema.Message, opts ...Option) (*schema.Message, error)
}
```

MVP 实现 DeepSeek Provider，后期可扩展：
- DeepSeek-V3（执行层，便宜）
- Claude Sonnet（分析/改进层，质量高）
- Qwen（备选）

### 6.3 配置存储

全局配置存储在 `~/.ase/config.yaml`：

```yaml
models:
  - name: deepseek-v3
    provider: deepseek
    api_key: sk-xxx
    base_url: https://api.deepseek.com
  - name: claude-sonnet
    provider: anthropic
    api_key: sk-ant-xxx

routing:
  execute: deepseek-v3
  analyze: deepseek-v3
  improve: deepseek-v3
```

---

## 7. CLI 接口

### 7.1 命令列表

| 命令 | 说明 | 示例 |
|---|---|---|
| `ase test <path>` | 测试 skill（只验证） | `ase test ./skills/go-test-build/` |
| `ase improve <path>` | 测试 + 改进 skill | `ase improve ./skills/go-test-build/` |
| `ase config` | 配置模型 | `ase config` |
| `ase info <path>` | 查看 skill 详情 | `ase info ./skills/go-test-build/` |
| `ase list` | 列出所有已测试的 skill | `ase list` |
| `ase history <path>` | 查看执行历史 | `ase history ./skills/go-test-build/` |

### 7.2 输出格式

每个阶段用 `[Tag]` 前缀标记：

```
[Parse]    解析 skill.md... OK (3 steps found)
[Execute]  Step 1: 运行测试... OK (exit 0)
[Execute]  Step 2: 构建项目... FAILED (exit 1)
[Analyze]  分析失败原因...
  原因: go.mod 中缺少依赖 github.com/foo/bar
[Improve]  修改 skill.md...
  → 在 Step 2 前增加: go mod tidy
[Execute]  Step 1: 运行测试... OK
[Execute]  Step 2: go mod tidy... OK
[Execute]  Step 3: 构建项目... OK
[Result]   PASS (1 次自动修复)
```

---

## 8. 数据存储

### 8.1 执行历史

使用 SQLite 存储每次执行记录：

```sql
CREATE TABLE execution_history (
    id INTEGER PRIMARY KEY,
    skill_path TEXT NOT NULL,
    skill_version INTEGER,
    status TEXT,          -- pass / fail / improved
    steps_total INTEGER,
    steps_passed INTEGER,
    retries INTEGER,
    model_used TEXT,
    duration_ms INTEGER,
    created_at TIMESTAMP,
    error_log TEXT,
    analysis TEXT
);
```

### 8.2 Skill 元数据

```sql
CREATE TABLE skills (
    path TEXT PRIMARY KEY,
    name TEXT,
    version INTEGER,
    total_tests INTEGER,
    passed_tests INTEGER,
    success_rate REAL,
    last_tested_at TIMESTAMP
);
```

---

## 9. 错误处理

| 场景 | 处理方式 |
|---|---|
| Skill 文件不存在 | 报错退出，提示路径 |
| Skill 格式错误 | 报错退出，指出解析错误位置 |
| Docker 未运行 | 报错退出，提示启动 Docker |
| 基础镜像不存在 | 自动拉取镜像 |
| LLM 调用失败 | 重试 1 次，失败则报错 |
| 重试次数耗尽 | 报告失败，输出所有尝试的完整记录 |
| 网络不通（容器内） | 如果 skill 声明需要网络，报错提示 |

---

## 10. 测试策略

- **单元测试**: parser、validate、model provider
- **集成测试**: Docker 执行器（需要 Docker 环境）
- **端到端测试**: 用一个简单的 skill 文件跑完整 test 和 improve 流程

---

## 11. MVP 范围与后期扩展

### MVP (1.0) 包含

- [ ] Skill 解析器（支持单文件和目录）
- [ ] Docker 沙箱执行器
- [ ] eino Graph 工作流（Parse→Execute→Validate→Analyze→Improve）
- [ ] LLM 分析和改进（DeepSeek）
- [ ] 模型配置（全局 + 分阶段）
- [ ] CLI 命令（test, improve, config, info, list, history）
- [ ] SQLite 存储执行历史

### V2.0 扩展（不在 MVP 范围）

- 多模型路由（自动选择最优模型）
- TDD 强化（自动生成测试脚本）
- 技能版本控制和成功率追踪
- MCP 协议导出
- Dashboard 可视化

---

## 12. 依赖

```go
require (
    github.com/cloudwego/eino          // Agent 框架
    github.com/docker/docker/client    // Docker SDK
    github.com/spf13/cobra             // CLI 框架
    github.com/mattn/go-sqlite3        // SQLite 驱动
    github.com/BurntSushi/toml         // 配置解析
)
```
