# 任务 1：项目初始化

**目标：** 初始化 Go 模块，创建基础配置文件和依赖。

**文件：**
- 创建：`go.mod`
- 创建：`.gitignore`
- 创建：`config.yaml`

---

## 步骤

- [ ] **步骤 1：初始化 Go 模块**

```bash
cd /c/Users/31800/Desktop/Project_practice/Go_Project/Agent
go mod init Agent
```

- [ ] **步骤 2：创建 .gitignore**

```gitignore
# 二进制文件
/bin/
*.exe
*.exe~
*.dll
*.so
*.dylib

# 测试二进制
*.test

# 覆盖率输出
*.out

# IDE
.idea/
.vscode/
*.swp
*.swo

# 操作系统
.DS_Store
Thumbs.db

# ASE 运行时
*.db
*.sqlite
```

- [ ] **步骤 3：创建默认 config.yaml**

```yaml
models:
  - name: deepseek-v3
    provider: deepseek
    api_key: ""
    base_url: https://api.deepseek.com

routing:
  execute: deepseek-v3
  analyze: deepseek-v3
  improve: deepseek-v3

sandbox:
  image: golang:1.22
  network_disabled: true
  cleanup: true

max_retries: 3
```

- [ ] **步骤 4：安装核心依赖**

```bash
go get github.com/spf13/cobra
go get github.com/mattn/go-sqlite3
go get github.com/BurntSushi/toml
go get github.com/cloudwego/eino
go get github.com/docker/docker/client
go get github.com/docker/docker/api/types/container
go get github.com/docker/docker/api/types/image
go get github.com/docker/go-connections/nat
```

- [ ] **步骤 5：提交**

```bash
git add go.mod go.sum .gitignore config.yaml
git commit -m "feat: 初始化 Go 模块和项目配置"
```
