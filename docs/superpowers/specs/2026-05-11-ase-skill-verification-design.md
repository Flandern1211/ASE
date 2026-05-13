# ASE Skill 验证架构设计

**日期**: 2026-05-11
**版本**: 1.0
**状态**: 设计中

---

## 1. Skill 的定义

**Skill = 标准化文件夹 = 知识 + 工具 的 SOP 打包单元**

```
my-skill/
├── SKILL.md           # 入口：frontmatter（元数据）+ body（混合型 SOP）
├── scripts/           # 工具：可执行脚本（可选）
├── tests/             # 验证：测试场景文件（可选）
└── config/            # 配置：环境参数（可选）
```

### 1.1 SKILL.md 格式

采用 Claude Code 的 SKILL.md 格式：

```markdown
---
name: "my-skill"
description: "Use when [触发条件]"
argument-hint: "参数提示"
compatibility: "环境要求"
metadata:
  author: "作者"
  source: "来源"
user-invocable: true
disable-model-invocation: false
---

# 标题

## SOP 内容（混合型）

### 可执行步骤
​```bash
go test ./... -v
​```
**预期**: 测试通过

### 指导原则
遇到 bug 时，先找根因再修复。

### 检查清单
- [ ] 是否运行了测试
- [ ] 是否检查了边界情况
```

### 1.2 SOP 类型

**混合型 SOP** — body 中同时包含：
- **可执行部分**: shell 命令、脚本调用
- **指导部分**: 原则、流程、检查清单

ASE 需要**区分**哪些可执行、哪些是指导。

---

## 2. Skill 分类与验证策略

### 2.1 Skill 类型

| 类型 | 特征 | 示例 |
|---|---|---|
| **可执行型** | 包含 shell 命令，无纯指令 | `go-test-build` skill |
| **指令型** | 纯文字描述，无命令 | `test-driven-development` skill |
| **混合型** | 两者都有 | 大多数实际 skill |

### 2.2 验证策略

```
解析 Skill（LLM）
    ↓
有可执行步骤？
├── 有 → Docker 执行验证
└── 无 → Agent 模拟测试
         ├── tests/ 有文件？ → 使用现有测试
         └── 无测试 → LLM 生成测试场景
```

---

## 3. Docker 执行验证

**适用**: 可执行型 skill（包含 shell 命令）

### 3.1 流程

1. LLM 从 SKILL.md 中提取可执行步骤
2. 选择基础镜像（根据 skill 的环境要求）
3. 创建 Docker 容器，挂载 skill 目录
4. 逐步执行 shell 命令
5. 检查退出码和输出是否符合预期
6. 记录执行结果

### 3.2 数据结构

```go
type ExecResult struct {
    SkillName string
    StepName  string
    ExitCode  int
    Stdout    string
    Stderr    string
    Success   bool
    Duration  time.Duration
}
```

---

## 4. Agent 模拟测试

**适用**: 指令型 skill（无 shell 命令）

### 4.1 流程

```
1. 读取 skill 内容
2. 检查 tests/ 目录
   ├── 有测试文件 → 直接使用
   └── 无测试 → LLM 根据 skill 内容生成测试场景
3. LLM 从 skill 中提取行为检查点
4. 让 AI agent 带着 skill 执行测试场景
5. LLM 逐项对比 agent 行为 vs 检查点
6. 判定通过/失败
```

### 4.2 测试场景格式

测试场景可以是 markdown 文件，包含：

```markdown
# 测试场景：调试一个 flaky test

## 场景描述
你有一个测试有时通过有时失败。测试使用了 setTimeout。

## 预期行为
- 先调查根因，不直接修复
- 识别出竞态条件
- 提出基于时间等待的解决方案

## 检查点
- [ ] 是否进行了根因分析
- [ ] 是否识别出竞态条件
- [ ] 解决方案是否涉及条件等待
```

### 4.3 检查点生成

LLM 从 skill 内容中提取行为检查点：

| 检查点类型 | 说明 | 示例 |
|---|---|---|
| 必须做 | skill 要求必须执行的行为 | "必须先运行测试" |
| 禁止做 | skill 明确禁止的行为 | "不能直接修复不找根因" |
| 顺序要求 | 行为的执行顺序 | "先分析后修复" |
| 输出要求 | 行为的产出要求 | "测试覆盖率必须 > 80%" |

### 4.4 结果判定

```
LLM 生成检查点
    ↓
Agent 执行测试场景
    ↓
LLM 对比 agent 行为 vs 检查点
    ↓
判定结果
├── 通过：所有检查点满足
├── 失败：有检查点不满足
└── 部分通过：核心检查点满足，次要不满足
```

---

## 5. 混合型 Skill 验证

**适用**: 同时包含可执行步骤和指导原则的 skill

### 5.1 流程

1. LLM 解析 skill，识别可执行部分和指导部分
2. 可执行部分 → Docker 执行验证
3. 指导部分 → Agent 模拟测试
4. 综合两部分结果，判定整体通过/失败

---

## 6. 改进循环

```
验证失败
    ↓
LLM 分析失败原因
├── 可执行部分失败 → 分析命令、环境、依赖问题
└── 指导部分失败 → 分析指令是否清晰、是否有歧义
    ↓
LLM 生成改进建议
    ↓
修改 skill 内容
    ↓
重新验证
    ↓
重试次数 ≤ N？
├── 是 → 继续改进
└── 否 → 报告失败，输出所有尝试记录
```

---

## 7. 数据结构设计

### 7.1 Skill（LLM 解析输出）

```go
type Skill struct {
    // 身份信息
    Name        string `json:"name"`
    Description string `json:"description"`
    Path        string `json:"-"`
    Dir         string `json:"-"`
    RawContent  string `json:"-"`

    // Frontmatter（原样保留，供改进时回写）
    Frontmatter map[string]interface{} `json:"frontmatter"`

    // 执行信息
    Steps []Step         `json:"steps"`
    Deps  []Dependency   `json:"deps"`

    // 语义理解
    Purpose        string   `json:"purpose"`
    UseCases       []string `json:"use_cases"`
    CorePrinciples string   `json:"core_principles"`

    // 辅助文件
    SupportFiles map[string]string `json:"support_files"`

    // 分类结果
    SkillType     string `json:"skill_type"`     // "executable", "instructional", "mixed"
    HasExecutable bool   `json:"has_executable"` // 是否包含可执行步骤
    HasGuidance   bool   `json:"has_guidance"`   // 是否包含指导原则
}
```

### 7.2 Step（可执行步骤）

```go
type Step struct {
    Name     string `json:"name"`
    Command  string `json:"command"`
    Expected string `json:"expected"`
    Order    int    `json:"order"`
}
```

### 7.3 Dependency（环境依赖）

```go
type Dependency struct {
    Name    string `json:"name"`    // 工具名
    Version string `json:"version"` // 版本要求
    Type    string `json:"type"`    // "tool", "language", "service"
}
```

### 7.4 TestScenario（测试场景）

```go
type TestScenario struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    Steps       []string `json:"steps"`
    Checkpoints []Checkpoint `json:"checkpoints"`
}

type Checkpoint struct {
    Description string `json:"description"`
    Type        string `json:"type"` // "must_do", "must_not", "order", "output"
    Required    bool   `json:"required"`
}
```

### 7.5 VerificationResult（验证结果）

```go
type VerificationResult struct {
    SkillName   string            `json:"skill_name"`
    SkillType   string            `json:"skill_type"`
    Pass        bool              `json:"pass"`
    Score       float64           `json:"score"`       // 0-1
    Checkpoints []CheckpointResult `json:"checkpoints"`
    Duration    time.Duration     `json:"duration"`
    Retries     int               `json:"retries"`
}

type CheckpointResult struct {
    Checkpoint Checkpoint `json:"checkpoint"`
    Met        bool       `json:"met"`
    Evidence   string     `json:"evidence"` // agent 的实际行为
}
```

### 7.6 Analysis（LLM 诊断）

```go
type Analysis struct {
    SkillName string `json:"skill_name"`
    Reason    string `json:"reason"`
    Suggest   string `json:"suggest"`
    FixType   string `json:"fix_type"` // "command", "instruction", "dependency"
}
```

---

## 8. 待讨论问题

1. **Agent 模拟的成本**: 每次验证都需要调用 LLM，成本如何控制？
2. **检查点的质量**: LLM 生成的检查点是否足够准确？
3. **测试场景的复用**: 生成的测试场景是否可以保存和复用？
4. **多模型协作**: 可执行部分和指导部分是否可以用不同的模型？
5. **并发验证**: 多个 skill 是否可以并行验证？

---

## 9. 下一步

1. 完成核心类型定义（任务 2）
2. 实现 LLM 解析器（任务 3）
3. 实现 Docker 执行器（任务 4）
4. 实现 Agent 模拟测试（任务 5）
5. 实现改进循环（任务 6）
