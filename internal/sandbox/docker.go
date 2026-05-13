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
	_, err := s.client.ImageInspect(ctx, imageRef)
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
