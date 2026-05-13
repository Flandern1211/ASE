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
