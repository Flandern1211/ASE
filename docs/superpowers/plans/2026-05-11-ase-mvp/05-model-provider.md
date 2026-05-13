# 任务 5：模型 Provider

**目标：** 实现模型配置管理、Provider 注册表和 DeepSeek ChatModel。

**文件：**
- 创建：`internal/model/provider.go`
- 创建：`internal/model/deepseek.go`

---

## 步骤

- [ ] **步骤 1：创建模型配置和 Provider 注册表**

创建 `internal/model/provider.go`：

```go
package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/cloudwego/eino/components/model"
)

// Config 表示完整的模型配置文件。
type Config struct {
	Models   []ModelEntry  `toml:"models"`
	Routing  RoutingConfig `toml:"routing"`
	Sandbox  SandboxConfig `toml:"sandbox"`
	MaxRetry int           `toml:"max_retries"`
}

type ModelEntry struct {
	Name     string `toml:"name"`
	Provider string `toml:"provider"`
	APIKey   string `toml:"api_key"`
	BaseURL  string `toml:"base_url"`
}

type RoutingConfig struct {
	Execute string `toml:"execute"`
	Analyze string `toml:"analyze"`
	Improve string `toml:"improve"`
}

type SandboxConfig struct {
	Image           string `toml:"image"`
	NetworkDisabled bool   `toml:"network_disabled"`
	Cleanup         bool   `toml:"cleanup"`
}

// ProviderRegistry 管理 ChatModel 实例。
type ProviderRegistry struct {
	models map[string]model.ChatModel
	config Config
}

// ConfigPath 返回全局配置文件路径。
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ase", "config.toml"), nil
}

// LoadConfig 从默认路径读取配置。
func LoadConfig() (*Config, error) {
	cfgPath, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{MaxRetry: 3}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(cfgPath, cfg); err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	return cfg, nil
}

// NewRegistry 从配置创建 ProviderRegistry。
func NewRegistry(cfg *Config) (*ProviderRegistry, error) {
	reg := &ProviderRegistry{
		models: make(map[string]model.ChatModel),
		config: *cfg,
	}

	for _, m := range cfg.Models {
		cm, err := CreateChatModel(m)
		if err != nil {
			return nil, fmt.Errorf("创建模型 %q 失败: %w", m.Name, err)
		}
		reg.models[m.Name] = cm
	}

	return reg, nil
}

// GetModel 按名称返回 ChatModel。
func (r *ProviderRegistry) GetModel(name string) (model.ChatModel, error) {
	cm, ok := r.models[name]
	if !ok {
		return nil, fmt.Errorf("模型 %q 不在注册表中", name)
	}
	return cm, nil
}

// GetForStage 返回指定流水线阶段的 ChatModel。
func (r *ProviderRegistry) GetForStage(stage string) (model.ChatModel, error) {
	var modelName string
	switch stage {
	case "execute":
		modelName = r.config.Routing.Execute
	case "analyze":
		modelName = r.config.Routing.Analyze
	case "improve":
		modelName = r.config.Routing.Improve
	default:
		return nil, fmt.Errorf("未知阶段: %s", stage)
	}

	if modelName == "" {
		return nil, fmt.Errorf("阶段 %q 未配置模型", stage)
	}

	return r.GetModel(modelName)
}

// CreateChatModel 根据 ModelEntry 实例化 ChatModel。
func CreateChatModel(entry ModelEntry) (model.ChatModel, error) {
	switch entry.Provider {
	case "deepseek":
		return NewDeepSeekModel(entry.APIKey, entry.BaseURL), nil
	default:
		return nil, fmt.Errorf("不支持的 Provider: %s", entry.Provider)
	}
}
```

- [ ] **步骤 2：实现 DeepSeek ChatModel**

创建 `internal/model/deepseek.go`：

```go
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// DeepSeekModel 实现 DeepSeek API 的 ChatModel。
type DeepSeekModel struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewDeepSeekModel 创建 DeepSeek ChatModel 实例。
func NewDeepSeekModel(apiKey, baseURL string) *DeepSeekModel {
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	return &DeepSeekModel{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

type deepSeekRequest struct {
	Model    string            `json:"model"`
	Messages []deepSeekMessage `json:"messages"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekResponse struct {
	Choices []struct {
		Message deepSeekMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Generate 向 DeepSeek 发送消息并返回响应。
func (m *DeepSeekModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	dkMessages := make([]deepSeekMessage, len(messages))
	for i, msg := range messages {
		role := "user"
		switch msg.Role {
		case schema.System:
			role = "system"
		case schema.Assistant:
			role = "assistant"
		case schema.User:
			role = "user"
		}
		dkMessages[i] = deepSeekMessage{
			Role:    role,
			Content: msg.Content,
		}
	}

	reqBody := deepSeekRequest{
		Model:    "deepseek-chat",
		Messages: dkMessages,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var dkResp deepSeekResponse
	if err := json.Unmarshal(respBody, &dkResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if dkResp.Error != nil {
		return nil, fmt.Errorf("API 错误: %s", dkResp.Error.Message)
	}

	if len(dkResp.Choices) == 0 {
		return nil, fmt.Errorf("响应中没有 choices")
	}

	return &schema.Message{
		Role:    schema.Assistant,
		Content: dkResp.Choices[0].Message.Content,
	}, nil
}
```

- [ ] **步骤 3：验证编译**

```bash
go build ./internal/model/
```
预期：编译成功

- [ ] **步骤 4：提交**

```bash
git add internal/model/
git commit -m "feat: 添加模型 Provider 注册表和 DeepSeek ChatModel"
```
