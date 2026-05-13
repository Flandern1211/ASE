# 任务 15：示例 Skill

**目标：** 创建示例 skill 文件用于冒烟测试。

**文件：**
- 创建：`skills/example/skill.md`

---

## 步骤

- [ ] **步骤 1：创建示例 skill**

创建 `skills/example/skill.md`：

```markdown
---
name: hello-world
description: 用于测试 ASE 的简单 hello world skill
version: 1
tags: [example, test]
expected_env:
  - bash: true
---

# Hello World Skill

## Steps

### Step 1: 打印 hello
` + "```bash" + `
echo "Hello from ASE!"
` + "```" + `
**Expected**: outputs Hello from ASE!

### Step 2: 检查当前目录
` + "```bash" + `
pwd
` + "```" + `
**Expected**: outputs /workspace
```

- [ ] **步骤 2：运行 ASE 测试**

```bash
./bin/ase test ./skills/example/skill.md
```
预期：通过，输出显示每个步骤的执行结果

- [ ] **步骤 3：查看 skill 信息**

```bash
./bin/ase info ./skills/example/skill.md
```
预期：显示 skill 名称、版本、2 个步骤

- [ ] **步骤 4：提交**

```bash
git add skills/example/skill.md
git commit -m "feat: 添加示例 skill 用于冒烟测试"
```
