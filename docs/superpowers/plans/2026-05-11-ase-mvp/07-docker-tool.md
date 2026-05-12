# 任务 7：Docker 执行工具

**目标：** 封装沙箱能力，提供步骤级执行接口。

**文件：**
- 创建：`internal/tools/docker_tool.go`
- 创建：`internal/tools/docker_tool_test.go`

---

## 步骤

- [ ] **步骤 1：编写失败测试**

创建 `internal/tools/docker_tool_test.go`：

```go
package tools

import (
	"Agent/internal/sandbox"
	"Agent/internal/skill"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDockerToolExecute(t *testing.T) {
	dt := NewDockerTool()

	tmp := t.TempDir()
	script := filepath.Join(tmp, "test.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho success"), 0755); err != nil {
		t.Fatal(err)
	}

	step := &skill.Step{
		Name:    "echo-test",
		Command: "sh /workspace/test.sh",
	}

	result, err := dt.Execute(context.Background(), step, sandbox.Config{
		Image:           "alpine:latest",
		WorkspaceDir:    tmp,
		NetworkDisabled: true,
		TimeoutSeconds:  30,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "success\n" {
		t.Errorf("expected 'success\\n', got %q", result.Stdout)
	}
	if result.StepName != "echo-test" {
		t.Errorf("expected step name 'echo-test', got %q", result.StepName)
	}
}

func TestDockerToolExecuteFailure(t *testing.T) {
	dt := NewDockerTool()

	tmp := t.TempDir()

	step := &skill.Step{
		Name:    "fail-step",
		Command: "sh -c 'echo oops >&2; exit 42'",
	}

	result, err := dt.Execute(context.Background(), step, sandbox.Config{
		Image:           "alpine:latest",
		WorkspaceDir:    tmp,
		NetworkDisabled: true,
		TimeoutSeconds:  30,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
	if result.Stderr != "oops\n" {
		t.Errorf("expected stderr 'oops\\n', got %q", result.Stderr)
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/tools/ -v -run TestDockerTool
```
预期：FAIL

- [ ] **步骤 3：实现 DockerTool**

创建 `internal/tools/docker_tool.go`：

```go
package tools

import (
	"Agent/internal/sandbox"
	"Agent/internal/skill"
	"context"
	"time"
)

// DockerTool 在 Docker 沙箱中执行 skill 步骤。
type DockerTool struct {
	sandbox *sandbox.Sandbox
}

// NewDockerTool 创建 DockerTool。
func NewDockerTool() *DockerTool {
	return &DockerTool{}
}

func (d *DockerTool) ensureSandbox(ctx context.Context) error {
	if d.sandbox != nil {
		return nil
	}
	sb, err := sandbox.New(ctx)
	if err != nil {
		return err
	}
	d.sandbox = sb
	return nil
}

// Execute 在 Docker 容器中运行单个步骤的命令。
func (d *DockerTool) Execute(ctx context.Context, step *skill.Step, cfg sandbox.Config) (*skill.ExecResult, error) {
	if err := d.ensureSandbox(ctx); err != nil {
		return nil, err
	}

	start := time.Now()

	out, err := d.sandbox.RunCommand(ctx, cfg, step.Command)
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

// Close 释放沙箱资源。
func (d *DockerTool) Close() error {
	if d.sandbox != nil {
		return d.sandbox.Close()
	}
	return nil
}
```

- [ ] **步骤 4：运行测试确认通过**

```bash
go test ./internal/tools/ -v -run TestDockerTool
```
预期：PASS（如果 Docker 正在运行）

- [ ] **步骤 5：提交**

```bash
git add internal/tools/docker_tool.go internal/tools/docker_tool_test.go
git commit -m "feat: 添加 DockerTool，封装容器内步骤执行"
```
