//go:build integration

package tools

import (
	"ASE/internal/config"
	"ASE/internal/model/provider"
	"ASE/internal/skill"
	"context"
	"strings"
	"testing"
	"time"
)

const testSkillContent = `# 文件整理 Skill

## 描述
自动整理当前目录下的文件，按扩展名分类到子目录中。

## 约束
- 不得删除任何文件
- 必须在操作前备份文件列表
- 必须输出整理前后的文件统计

## 执行步骤
1. 列出当前目录所有文件
2. 按扩展名创建子目录（如 images/, documents/, scripts/）
3. 将文件移动到对应子目录
4. 输出整理报告

## Red Flags
- 绝对不能使用 rm 命令
- 不能移动隐藏文件`

func loadModel(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../config/config.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if len(cfg.Models) == 0 {
		t.Skip("配置中无模型，跳过集成测试")
	}
	return cfg
}

func createBaseChatModel(t *testing.T) *config.Config {
	t.Helper()
	return loadModel(t)
}

// === CheckpointTool 集成测试 ===

func TestIntegration_Verify_RealLLM(t *testing.T) {
	cfg := loadModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cm, err := provider.CreateChatModel(ctx, cfg.Models[0])
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	ct := NewCheckpointTool(cm)

	cp := skill.Checkpoint{
		Description: "脚本使用了 set -e 确保失败时退出",
		Type:        "must_do",
		Required:    true,
	}
	trace := `#!/bin/bash
set -e
echo "开始整理文件"
mkdir -p images documents scripts
for f in *; do
  case "${f##*.}" in
    jpg|png) mv "$f" images/ ;;
    pdf|doc) mv "$f" documents/ ;;
    sh|py) mv "$f" scripts/ ;;
  esac
done
echo "整理完成"`

	result, err := ct.Verify(ctx, cp, trace)
	if err != nil {
		t.Fatalf("Verify 失败: %v", err)
	}
	t.Logf("Verify 结果: Met=%v, Confidence=%.2f, Evidence=%s", result.Met, result.Confidence, result.Evidence)
	if result.Confidence <= 0 || result.Confidence > 1 {
		t.Errorf("置信度超出有效范围: %f", result.Confidence)
	}
	if result.Evidence == "" {
		t.Error("期望非空的证据")
	}
}

func TestIntegration_VerachBatch_RealLLM(t *testing.T) {
	cfg := loadModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cm, err := provider.CreateChatModel(ctx, cfg.Models[0])
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	ct := NewCheckpointTool(cm)

	checkpoints := []skill.Checkpoint{
		{Description: "输出了整理前的文件列表", Type: "must_do", Required: false},
		{Description: "按扩展名创建了子目录", Type: "must_do", Required: false},
		{Description: "没有删除任何文件", Type: "must_not", Required: true},
		{Description: "输出了整理报告", Type: "output", Required: false},
	}
	trace := `执行日志:
$ ls
file1.jpg file2.pdf script.sh notes.txt
$ mkdir -p images documents scripts
$ mv file1.jpg images/
$ mv file2.pdf documents/
$ mv script.sh scripts/
$ mv notes.txt documents/
$ echo "整理完成：4 个文件已分类到 3 个目录"
整理完成：4 个文件已分类到 3 个目录`

	results, err := ct.VerifyBatch(ctx, checkpoints, trace)
	if err != nil {
		t.Fatalf("VerifyBatch 失败: %v", err)
	}
	if len(results) != len(checkpoints) {
		t.Fatalf("结果数量不匹配: 期望 %d, 实际 %d", len(checkpoints), len(results))
	}
	for i, r := range results {
		t.Logf("检查点[%d]: Met=%v, Confidence=%.2f, Evidence=%s", i, r.Met, r.Confidence, r.Evidence)
		if r.Confidence <= 0 || r.Confidence > 1 {
			t.Errorf("检查点[%d] 置信度超出有效范围: %f", i, r.Confidence)
		}
	}
}

// === ValidateTool 集成测试 ===

func TestIntegration_GenerateScript_RealLLM(t *testing.T) {
	cfg := loadModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cm, err := provider.CreateChatModel(ctx, cfg.Models[0])
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	vt := NewValidateTool(cm)
	sk := &skill.Skill{
		Name:        "file-organizer",
		Description: "按扩展名整理文件到子目录",
		RawContent:  testSkillContent,
	}

	script, err := vt.GenerateScript(ctx, sk)
	if err != nil {
		t.Fatalf("GenerateScript 失败: %v", err)
	}
	t.Logf("生成的脚本:\n%s", script)

	if script == "" {
		t.Fatal("期望非空脚本")
	}
	if !strings.Contains(script, "set -e") && !strings.Contains(script, "set -o") {
		t.Log("警告: 脚本未包含 set -e，可能不符合要求")
	}
}

func TestIntegration_ValidateOutput_Passed_RealLLM(t *testing.T) {
	cfg := loadModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cm, err := provider.CreateChatModel(ctx, cfg.Models[0])
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	vt := NewValidateTool(cm)
	sk := &skill.Skill{
		Name:        "file-organizer",
		Description: "按扩展名整理文件到子目录",
		RawContent:  testSkillContent,
	}

	stdout := `开始整理文件
file1.jpg -> images/
file2.pdf -> documents/
script.sh -> scripts/
整理完成：3 个文件已分类到 3 个目录
统计: images/ 1 个, documents/ 1 个, scripts/ 1 个`

	result, err := vt.ValidateOutput(ctx, sk, stdout, "", 0)
	if err != nil {
		t.Fatalf("ValidateOutput 失败: %v", err)
	}
	t.Logf("验证结果: Passed=%v, Summary=%s, Details=%s", result.Passed, result.Summary, result.Details)
	if !result.Passed {
		t.Error("期望验证通过")
	}
}

func TestIntegration_ValidateOutput_Failed_RealLLM(t *testing.T) {
	cfg := loadModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cm, err := provider.CreateChatModel(ctx, cfg.Models[0])
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	vt := NewValidateTool(cm)
	sk := &skill.Skill{
		Name:        "file-organizer",
		Description: "按扩展名整理文件到子目录",
		RawContent:  testSkillContent,
	}

	stderr := `rm: cannot remove '.hidden': Operation not permitted
Error: 文件删除失败`

	result, err := vt.ValidateOutput(ctx, sk, "", stderr, 1)
	if err != nil {
		t.Fatalf("ValidateOutput 失败: %v", err)
	}
	t.Logf("验证结果: Passed=%v, Summary=%s, Details=%s", result.Passed, result.Summary, result.Details)
	if result.Passed {
		t.Error("期望验证失败（脚本报错且违反了 Red Flags）")
	}
}

// === AgentSimTool 集成测试 ===

func TestIntegration_Simulate_WithScenario_RealLLM(t *testing.T) {
	cfg := loadModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cm, err := provider.CreateChatModel(ctx, cfg.Models[0])
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	evaluator := NewCheckpointTool(cm)
	at := NewAgentSimTool(cm, evaluator)

	sk := &skill.Skill{
		Name:        "file-organizer",
		Description: "按扩展名整理文件到子目录",
		RawContent:  testSkillContent,
	}

	scenario := &skill.TestScenario{
		Name:        "基本文件整理",
		Description: "用户有一个混合文件的目录，需要按扩展名分类整理",
		Steps: []string{
			"用户请求整理当前目录下的文件",
			"Agent 应列出所有文件并按扩展名分类",
			"Agent 应创建子目录并移动文件",
			"Agent 应输出整理报告",
		},
		Checkpoints: []skill.Checkpoint{
			{Description: "Agent 列出了当前目录的文件", Type: "must_do", Required: true},
			{Description: "Agent 创建了按扩展名分类的子目录", Type: "must_do", Required: true},
			{Description: "Agent 没有删除任何文件", Type: "must_not", Required: true},
			{Description: "Agent 输出了整理前后的统计信息", Type: "output", Required: false},
		},
	}

	result, err := at.Simulate(ctx, sk, scenario)
	if err != nil {
		t.Fatalf("Simulate 失败: %v", err)
	}
	t.Logf("模拟结果: Pass=%v, Score=%.2f", result.Pass, result.Score)
	t.Logf("Trace:\n%s", result.Trace)

	if result.Score < 0 || result.Score > 1 {
		t.Errorf("分数超出有效范围: %f", result.Score)
	}
}

func TestIntegration_Simulate_AutoScenario_RealLLM(t *testing.T) {
	cfg := loadModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cm, err := provider.CreateChatModel(ctx, cfg.Models[0])
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	evaluator := NewCheckpointTool(cm)
	at := NewAgentSimTool(cm, evaluator)

	sk := &skill.Skill{
		Name:        "file-organizer",
		Description: "按扩展名整理文件到子目录",
		RawContent:  testSkillContent,
	}

	// 不传入 scenario，让 LLM 自动生成
	result, err := at.Simulate(ctx, sk, nil)
	if err != nil {
		t.Fatalf("Simulate (自动生成场景) 失败: %v", err)
	}
	t.Logf("模拟结果: Pass=%v, Score=%.2f", result.Pass, result.Score)
	t.Logf("Trace:\n%s", result.Trace)

	if result.Score < 0 || result.Score > 1 {
		t.Errorf("分数超出有效范围: %f", result.Score)
	}
}

// === 端到端集成测试：完整验证流程 ===

func TestIntegration_FullValidationFlow_RealLLM(t *testing.T) {
	cfg := loadModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	cm, err := provider.CreateChatModel(ctx, cfg.Models[0])
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}

	sk := &skill.Skill{
		Name:        "file-organizer",
		Description: "按扩展名整理文件到子目录",
		RawContent:  testSkillContent,
	}

	// 阶段 1: 生成脚本
	vt := NewValidateTool(cm)
	script, err := vt.GenerateScript(ctx, sk)
	if err != nil {
		t.Fatalf("GenerateScript 失败: %v", err)
	}
	t.Logf("[阶段1] 生成脚本:\n%s", script)

	// 阶段 2: 模拟执行并验证输出
	fakeStdout := `开始整理文件
file1.jpg -> images/
file2.pdf -> documents/
整理完成`
	result, err := vt.ValidateOutput(ctx, sk, fakeStdout, "", 0)
	if err != nil {
		t.Fatalf("ValidateOutput 失败: %v", err)
	}
	t.Logf("[阶段2] 验证结果: Passed=%v, Summary=%s", result.Passed, result.Summary)

	// 阶段 3: Agent 模拟测试
	evaluator := NewCheckpointTool(cm)
	at := NewAgentSimTool(cm, evaluator)
	scenario := &skill.TestScenario{
		Name:        "端到端测试",
		Description: "验证文件整理 skill 的完整流程",
		Steps:       []string{"整理当前目录文件"},
		Checkpoints: []skill.Checkpoint{
			{Description: "列出了文件", Type: "must_do", Required: true},
			{Description: "没有删除文件", Type: "must_not", Required: true},
		},
	}
	simResult, err := at.Simulate(ctx, sk, scenario)
	if err != nil {
		t.Fatalf("Simulate 失败: %v", err)
	}
	t.Logf("[阶段3] 模拟结果: Pass=%v, Score=%.2f", simResult.Pass, simResult.Score)
}
