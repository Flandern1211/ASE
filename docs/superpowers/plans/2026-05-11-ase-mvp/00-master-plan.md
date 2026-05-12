# Aegis Skill Engine (ASE) MVP — 实施计划

> **致智能体工作者：** 必须使用子技能 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 来逐任务执行本计划。步骤使用 `- [ ]` 语法追踪进度。

**目标：** 构建一个 CLI 工具，验证和自动改进 skill 文件。支持两种验证方式：Docker 沙箱执行（可执行型 skill）和 Agent 模拟测试（指令型 skill）。通过 LLM 分析失败原因并修改 skill 直到验证通过。

**架构：** 线性流水线（解析 → 验证 → 分析 → 改进），使用 eino Graph 编排。Skill 是标准化文件夹（SKILL.md + 辅助文件），SOP 内容为混合型（指令 + 可执行命令）。LLM 负责解析 skill 内容和生成测试场景。模型路由通过 eino ChatModel 接口代理 LLM 调用（DeepSeek）。

**技术栈：** Go 1.22+, eino (CloudWeGo), Docker SDK (Go), Cobra, go-sqlite3, BurntSushi/toml

---

## 项目结构

```
Agent/
├── cmd/
│   └── ase/
│       └── main.go                          # CLI 入口
├── internal/
│   ├── skill/
│   │   ├── types.go                         # Skill、Step、ExecResult 等核心类型
│   │   ├── parser.go                        # LLM 解析器（读取 SKILL.md → Skill struct）
│   │   ├── parser_test.go                   # 解析器测试
│   │   ├── classifier.go                    # Skill 类型分类器（executable/instructional/mixed）
│   │   └── classifier_test.go
│   ├── graph/
│   │   ├── workflow.go                      # eino Graph 工作流定义
│   │   └── workflow_test.go                 # 工作流测试
│   ├── tools/
│   │   ├── docker_tool.go                   # Docker 执行工具
│   │   ├── docker_tool_test.go
│   │   ├── agent_sim_tool.go                # Agent 模拟测试工具
│   │   ├── agent_sim_tool_test.go
│   │   ├── analyze_tool.go                  # LLM 失败分析工具
│   │   ├── analyze_tool_test.go
│   │   ├── improve_tool.go                  # LLM skill 改进工具
│   │   ├── improve_tool_test.go
│   │   ├── validate_tool.go                 # Docker 结果验证工具
│   │   ├── validate_tool_test.go
│   │   ├── checkpoint_tool.go               # 检查点验证工具
│   │   └── checkpoint_tool_test.go
│   ├── model/
│   │   ├── provider.go                      # 模型配置管理 + Provider 注册表
│   │   └── deepseek.go                      # DeepSeek ChatModel 实现
│   ├── sandbox/
│   │   ├── docker.go                        # Docker SDK 封装
│   │   ├── docker_test.go
│   │   └── types.go                         # 容器配置、执行结果类型
│   ├── storage/
│   │   ├── db.go                            # SQLite 初始化 + 迁移
│   │   ├── history.go                       # 执行历史 CRUD
│   │   └── history_test.go
│   └── cli/
│       ├── commands.go                      # Cobra 命令定义
│       └── config.go                        # 交互式配置命令
├── skills/
│   └── example/
│       └── skill.md                         # 示例 skill
├── config.yaml
├── .gitignore
├── go.mod
└── go.sum
```

---

## 任务清单

| # | 任务 | 文件 | 依赖 |
|---|------|------|------|
| 1 | [项目初始化](01-project-init.md) | go.mod, .gitignore, config.yaml | 无 |
| 2 | [核心类型](02-core-types.md) | internal/skill/types.go | Task 1 |
| 3 | [Skill 解析器](03-skill-parser.md) | internal/skill/parser.go | Task 2, 5 |
| 4 | [Docker 沙箱](04-docker-sandbox.md) | internal/sandbox/ | Task 1 |
| 5 | [模型 Provider](05-model-provider.md) | internal/model/ | Task 1 |
| 6 | [Docker 验证工具](06-validate-tool.md) | internal/tools/validate_tool.go | Task 2 |
| 7 | [Docker 执行工具](07-docker-tool.md) | internal/tools/docker_tool.go | Task 4 |
| 8 | [分析工具](08-analyze-tool.md) | internal/tools/analyze_tool.go | Task 5 |
| 9 | [改进工具](09-improve-tool.md) | internal/tools/improve_tool.go | Task 5 |
| 10 | [存储层](10-storage.md) | internal/storage/ | Task 2 |
| 11 | [Graph 工作流](11-graph-workflow.md) | internal/graph/workflow.go | Task 6-9 |
| 12 | [CLI 命令](12-cli-commands.md) | internal/cli/commands.go | Task 11 |
| 13 | [入口点](13-main-entry.md) | cmd/ase/main.go | Task 12 |
| 14 | [配置命令](14-config-command.md) | internal/cli/config.go | Task 5 |
| 15 | [示例 Skill](15-example-skill.md) | skills/example/skill.md | Task 13 |
| 16 | [完整测试](16-full-tests.md) | 全部 | 全部 |
| 17 | [集成验证](17-integration.md) | 无 | Task 15, 16 |
| 18 | [Skill 分类器](18-skill-classifier.md) | internal/skill/classifier.go | Task 2, 3 |
| 19 | [Agent 模拟测试](19-agent-simulation.md) | internal/tools/agent_sim_tool.go | Task 2, 5 |
| 20 | [检查点验证](20-checkpoint-validation.md) | internal/tools/checkpoint_tool.go | Task 2 |

---

## 设计流水线

```
Parse (LLM) → Classify Skill Type
                    ↓
        ┌───────────┴───────────┐
        ↓                       ↓
  可执行型                    指令型/混合型
        ↓                       ↓
  Docker 执行              Agent 模拟测试
        ↓                       ↓
  验证退出码/输出           验证检查点
        ↓                       ↓
        └───────────┬───────────┘
                    ↓
              [成功] → 完成
              [失败] → Analyze → Improve → 重新验证 (重试，最多 N 次)
```
