# 任务 4：Docker 沙箱

**目标：** 使用 Docker SDK 封装容器执行能力，在隔离环境中运行命令。

**文件：**
- 创建：`internal/sandbox/types.go`
- 创建：`internal/sandbox/docker.go`
- 创建：`internal/sandbox/docker_test.go`

---

## 步骤

- [ ] **步骤 1：创建沙箱类型**

创建 `internal/sandbox/types.go`：

```go
package sandbox

import "time"

// Config 持有 Docker 容器配置。
type Config struct {
	Image           string
	WorkspaceDir    string // 挂载到容器的宿主机目录
	NetworkDisabled bool
	TimeoutSeconds  int
}

// ExecOutput 持有容器内命令执行结果。
type ExecOutput struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// DefaultConfig 返回合理的默认沙箱配置。
func DefaultConfig() Config {
	return Config{
		Image:           "golang:1.22",
		NetworkDisabled: true,
		TimeoutSeconds:  300,
	}
}
```

- [ ] **步骤 2：编写失败测试**

创建 `internal/sandbox/docker_test.go`：

```go
package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDockerSandbox(t *testing.T) {
	sb, err := New(context.Background())
	if err != nil {
		t.Skipf("Docker 不可用: %v", err)
	}
	defer sb.Close()
}

func TestRunCommand(t *testing.T) {
	sb, err := New(context.Background())
	if err != nil {
		t.Skipf("Docker 不可用: %v", err)
	}
	defer sb.Close()

	tmp := t.TempDir()
	f := filepath.Join(tmp, "hello.sh")
	if err := os.WriteFile(f, []byte("#!/bin/sh\necho hello-world"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Image:           "alpine:latest",
		WorkspaceDir:    tmp,
		NetworkDisabled: true,
		TimeoutSeconds:  30,
	}

	out, err := sb.RunCommand(context.Background(), cfg, "sh /workspace/hello.sh")
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}

	if out.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", out.ExitCode)
	}
	if out.Stdout != "hello-world\n" {
		t.Errorf("expected 'hello-world\\n', got %q", out.Stdout)
	}
}

func TestRunCommandFailure(t *testing.T) {
	sb, err := New(context.Background())
	if err != nil {
		t.Skipf("Docker 不可用: %v", err)
	}
	defer sb.Close()

	tmp := t.TempDir()
	cfg := Config{
		Image:           "alpine:latest",
		WorkspaceDir:    tmp,
		NetworkDisabled: true,
		TimeoutSeconds:  30,
	}

	out, err := sb.RunCommand(context.Background(), cfg, "sh -c 'exit 1'")
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}

	if out.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", out.ExitCode)
	}
}
```

- [ ] **步骤 3：运行测试确认失败**

```bash
go test ./internal/sandbox/ -v
```
预期：FAIL（New 函数未定义）

- [ ] **步骤 4：实现 Docker 沙箱**

创建 `internal/sandbox/docker.go`：

```go
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// Sandbox 封装 Docker 容器执行能力。
type Sandbox struct {
	client *client.Client
}

// New 创建一个连接到 Docker 守护进程的 Sandbox。
func New(ctx context.Context) (*Sandbox, error) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("创建 Docker 客户端失败: %w", err)
	}

	if _, err := c.Ping(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("Docker 守护进程不可达: %w", err)
	}

	return &Sandbox{client: c}, nil
}

// Close 释放 Docker 客户端资源。
func (s *Sandbox) Close() error {
	return s.client.Close()
}

// RunCommand 创建容器，执行命令，收集输出，然后清理。
func (s *Sandbox) RunCommand(ctx context.Context, cfg Config, command string) (*ExecOutput, error) {
	if err := s.ensureImage(ctx, cfg.Image); err != nil {
		return nil, err
	}

	workDir := "/workspace"
	containerCfg := &container.Config{
		Image:        cfg.Image,
		Cmd:          []string{"sh", "-c", command},
		WorkingDir:   workDir,
		AttachStdout: true,
		AttachStderr: true,
	}

	hostCfg := &container.HostConfig{
		Binds: []string{cfg.WorkspaceDir + ":" + workDir},
	}

	if cfg.NetworkDisabled {
		hostCfg.NetworkMode = container.NetworkMode("none")
	}

	timeoutSecs := cfg.TimeoutSeconds
	if timeoutSecs <= 0 {
		timeoutSecs = 300
	}

	resp, err := s.client.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("创建容器失败: %w", err)
	}
	containerID := resp.ID

	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = s.client.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true})
	}()

	if err := s.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("启动容器失败: %w", err)
	}

	attachResp, err := s.client.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("附加容器失败: %w", err)
	}
	defer attachResp.Close()

	var stdout, stderr bytes.Buffer
	doneCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(&stdout, attachResp.Reader)
		doneCh <- err
	}()

	timeout := time.After(time.Duration(timeoutSecs) * time.Second)
	statusCh, errCh := s.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		return nil, fmt.Errorf("容器等待错误: %w", err)
	case status := <-statusCh:
		<-doneCh
		return &ExecOutput{
			ExitCode: int(status.StatusCode),
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
		}, nil
	case <-timeout:
		return nil, fmt.Errorf("容器执行超时（%d 秒）", timeoutSecs)
	}
}

func (s *Sandbox) ensureImage(ctx context.Context, imageRef string) error {
	_, _, err := s.client.ImageInspectWithRaw(ctx, imageRef)
	if err == nil {
		return nil
	}

	pullResp, err := s.client.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("拉取镜像 %s 失败: %w", imageRef, err)
	}
	defer pullResp.Close()

	_, err = io.Copy(io.Discard, pullResp)
	if err != nil {
		return fmt.Errorf("拉取镜像 %s 失败: %w", imageRef, err)
	}

	return nil
}
```

- [ ] **步骤 5：运行测试**

```bash
go test ./internal/sandbox/ -v -count=1
```
预期：PASS（如果 Docker 正在运行）

- [ ] **步骤 6：提交**

```bash
git add internal/sandbox/
git commit -m "feat: 实现 Docker 沙箱，支持隔离执行命令"
```
