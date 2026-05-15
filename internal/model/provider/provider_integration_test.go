//go:build integration

package provider

import (
	"context"
	"testing"
	"time"

	"ASE/internal/config"
)

func loadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../../config/config.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	return cfg
}

func TestCreateChatModel_Real(t *testing.T) {
	cfg := loadConfig(t)
	if len(cfg.Models) == 0 {
		t.Skip("配置中无模型")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entry := cfg.Models[0]
	cm, err := CreateChatModel(ctx, entry)
	if err != nil {
		t.Fatalf("CreateChatModel 失败: %v", err)
	}
	if cm == nil {
		t.Fatal("CreateChatModel 返回 nil")
	}
	t.Logf("模型 %q 创建成功", entry.Name)
}

func TestNewRegistry_HealthCheck(t *testing.T) {
	cfg := loadConfig(t)
	if len(cfg.Models) == 0 {
		t.Skip("配置中无模型")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg, err := NewRegistry(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRegistry 失败: %v", err)
	}
	if reg == nil {
		t.Fatal("NewRegistry 返回 nil")
	}
	t.Logf("注册了 %d 个模型，健康检查全部通过", len(cfg.Models))
}

func TestGetForStage_WithRealModel(t *testing.T) {
	cfg := loadConfig(t)
	if len(cfg.Models) == 0 {
		t.Skip("配置中无模型")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reg, err := NewRegistry(ctx, cfg)
	if err != nil {
		t.Fatalf("NewRegistry 失败: %v", err)
	}

	for _, stage := range []string{"execute", "analyze", "improve"} {
		cm, err := reg.GetForStage(stage)
		if err != nil {
			t.Fatalf("GetForStage(%s) 失败: %v", stage, err)
		}
		if cm == nil {
			t.Fatalf("GetForStage(%s) 返回 nil", stage)
		}
		t.Logf("阶段 %q -> 模型获取成功", stage)
	}
}
