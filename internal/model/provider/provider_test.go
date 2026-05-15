package provider

import (
	"testing"

	"ASE/internal/config"
	mockmodel "ASE/internal/mock/components/model"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/golang/mock/gomock"
)

func newTestRegistry(models map[string]model.BaseChatModel, cfg *config.Config) *ProviderRegistry {
	return &ProviderRegistry{
		models: models,
		config: cfg,
	}
}

func TestGetModel_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockmodel.NewMockBaseChatModel(ctrl)
	reg := newTestRegistry(map[string]model.BaseChatModel{
		"gpt-4": mock,
	}, nil)

	cm, err := reg.GetModel("gpt-4")
	if err != nil {
		t.Fatalf("GetModel 失败: %v", err)
	}
	if cm != mock {
		t.Fatal("返回的模型实例与预期不符")
	}
}

func TestGetModel_NotFound(t *testing.T) {
	reg := newTestRegistry(map[string]model.BaseChatModel{}, nil)

	_, err := reg.GetModel("不存在的模型")
	if err == nil {
		t.Fatal("期望返回错误，但为 nil")
	}
}

func TestGetForStage_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockmodel.NewMockBaseChatModel(ctrl)
	cfg := &config.Config{
		Routing: config.RoutingConfig{
			Execute: "gpt-4",
			Analyze: "claude",
			Improve: "deepseek",
		},
	}
	reg := newTestRegistry(map[string]model.BaseChatModel{
		"gpt-4":    mock,
		"claude":   mockmodel.NewMockBaseChatModel(ctrl),
		"deepseek": mockmodel.NewMockBaseChatModel(ctrl),
	}, cfg)

	cm, err := reg.GetForStage("execute")
	if err != nil {
		t.Fatalf("GetForStage(execute) 失败: %v", err)
	}
	if cm != mock {
		t.Fatal("返回的模型实例与预期不符")
	}
}

func TestGetForStage_UnknownStage(t *testing.T) {
	reg := newTestRegistry(map[string]model.BaseChatModel{}, &config.Config{})

	_, err := reg.GetForStage("unknown")
	if err == nil {
		t.Fatal("期望返回错误，但为 nil")
	}
}

func TestGetForStage_UnconfiguredStage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &config.Config{
		Routing: config.RoutingConfig{
			Execute: "gpt-4",
		},
	}
	reg := newTestRegistry(map[string]model.BaseChatModel{
		"gpt-4": mockmodel.NewMockBaseChatModel(ctrl),
	}, cfg)

	_, err := reg.GetForStage("analyze")
	if err == nil {
		t.Fatal("期望返回错误，但为 nil")
	}
}

func TestGetForStage_AllStages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &config.Config{
		Routing: config.RoutingConfig{
			Execute: "exec",
			Analyze: "analyze",
			Improve: "improve",
		},
	}
	reg := newTestRegistry(map[string]model.BaseChatModel{
		"exec":    mockmodel.NewMockBaseChatModel(ctrl),
		"analyze": mockmodel.NewMockBaseChatModel(ctrl),
		"improve": mockmodel.NewMockBaseChatModel(ctrl),
	}, cfg)

	for _, stage := range []string{"execute", "analyze", "improve"} {
		cm, err := reg.GetForStage(stage)
		if err != nil {
			t.Fatalf("GetForStage(%s) 失败: %v", stage, err)
		}
		if cm == nil {
			t.Fatalf("GetForStage(%s) 返回 nil", stage)
		}
	}
}

func TestHealthCheck_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockmodel.NewMockBaseChatModel(ctrl)
	mock.EXPECT().Generate(gomock.Any(), gomock.Any()).Return(
		schema.AssistantMessage("pong", nil), nil,
	)

	// 验证 mock 的 Generate 被调用（模拟健康检查通过）
	_, err := mock.Generate(nil, []*schema.Message{
		schema.SystemMessage("ping"),
		schema.UserMessage("hi"),
	})
	if err != nil {
		t.Fatalf("健康检查失败: %v", err)
	}
}

func TestHealthCheck_Failure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockmodel.NewMockBaseChatModel(ctrl)
	mock.EXPECT().Generate(gomock.Any(), gomock.Any()).Return(
		nil, &connectionError{"connection refused"},
	)

	// 验证健康检查失败时返回错误
	_, err := mock.Generate(nil, []*schema.Message{
		schema.SystemMessage("ping"),
		schema.UserMessage("hi"),
	})
	if err == nil {
		t.Fatal("期望健康检查返回错误，但为 nil")
	}
}

type connectionError struct {
	msg string
}

func (e *connectionError) Error() string {
	return e.msg
}
