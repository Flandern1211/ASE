package config

// Config 应用配置结构体
type Config struct {
	Models   []ModelEntry  `mapstructure:"models"`
	Routing  RoutingConfig `mapstructure:"routing"`
	Sandbox  SandboxConfig `mapstructure:"sandbox"`
	MaxRetry int           `mapstructure:"max_retries"`
}

// ModelEntry 模型配置
type ModelEntry struct {
	Name     string `mapstructure:"name"`
	Provider string `mapstructure:"provider"`
	APIKey   string `mapstructure:"api_key"`
	BaseURL  string `mapstructure:"base_url"`
	Model    string `mapstructure:"model"`
}

// RoutingConfig 阶段路由配置
type RoutingConfig struct {
	Execute string `mapstructure:"execute"`
	Analyze string `mapstructure:"analyze"`
	Improve string `mapstructure:"improve"`
}

// SandboxConfig 沙箱配置
type SandboxConfig struct {
	Image           string `mapstructure:"image"`
	NetworkDisabled bool   `mapstructure:"network_disabled"`
	Cleanup         bool   `mapstructure:"cleanup"`
}
