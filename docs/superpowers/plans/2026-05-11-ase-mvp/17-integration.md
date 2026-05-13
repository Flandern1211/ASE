# 任务 17：集成验证

**目标：** 端到端冒烟测试，确认所有组件协同工作。

**文件：** 无

---

## 步骤

- [ ] **步骤 1：构建最终二进制文件**

```bash
go build -o ./bin/ase ./cmd/ase/
```

- [ ] **步骤 2：运行端到端冒烟测试**

```bash
./bin/ase test ./skills/example/
./bin/ase info ./skills/example/
./bin/ase list
```
预期：所有命令正常执行，无错误。

- [ ] **步骤 3：验证 git 状态干净**

```bash
git status
```
预期：工作区干净（或仅有预期的未跟踪文件）。

- [ ] **步骤 4：最终提交（如有需要）**

```bash
git add -A
git commit -m "chore: ASE MVP 完成 — 所有功能已集成并测试"
```
仅在有未提交的更改时执行。
