package provider

import (
	"context"
	"fmt"
	"github.com/cloudwego/eino/schema"

	"ASE/internal/config"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// ProviderRegistry 管理 ChatModel 实例
type ProviderRegistry struct {
	models map[string]model.BaseChatModel
	config *config.Config
}

// CreateChatModel 根据 ModelEntry 创建 ChatModel 实例
func CreateChatModel(ctx context.Context, entry config.ModelEntry) (model.BaseChatModel, error) {
	cfg := &openai.ChatModelConfig{
		Model:   entry.Model,
		APIKey:  entry.APIKey,
		BaseURL: entry.BaseURL,
	}
	return openai.NewChatModel(ctx, cfg)
}

// NewRegistry 从配置创建 ProviderRegistry
func NewRegistry(ctx context.Context, cfg *config.Config) (*ProviderRegistry, error) {
	reg := &ProviderRegistry{
		models: make(map[string]model.BaseChatModel),
		config: cfg,
	}

	for _, entry := range cfg.Models {
		cm, err := CreateChatModel(ctx, entry)
		if err != nil {
			return nil, fmt.Errorf("创建模型 %q 失败: %w", entry.Name, err)
		}

		//健康检查
		_, err = cm.Generate(ctx, []*schema.Message{
			schema.SystemMessage("ping"),
			schema.UserMessage("hi"),
		})
		if err != nil {
			return nil, fmt.Errorf("模型 %q 连通性检查失败: %w", entry.Name, err)
		}

		reg.models[entry.Name] = cm
	}

	return reg, nil
}

// GetModel 根据名称获取模型实例
func (r *ProviderRegistry) GetModel(name string) (model.BaseChatModel, error) {
	cm, ok := r.models[name]
	if !ok {
		return nil, fmt.Errorf("模型 %q 未注册", name)
	}
	return cm, nil
}

// GetForStage 根据流水线阶段获取模型
func (r *ProviderRegistry) GetForStage(stage string) (model.BaseChatModel, error) {
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
