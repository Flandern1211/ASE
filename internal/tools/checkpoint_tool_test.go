package tools

import (
	"ASE/internal/skill"
	"context"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type cpMockLLM struct {
	response string
}

func (m *cpMockLLM) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{
		Role:    schema.Assistant,
		Content: m.response,
	}, nil
}

func (m *cpMockLLM) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("Stream not implemented in mock")
}

func TestVerifyMet(t *testing.T) {
	llm := &cpMockLLM{response: `{"met": true, "confidence": 0.9, "evidence": "agent 在第 3 步明确创建了文件"}`}
	ct := NewCheckpointTool(llm)

	cp := skill.Checkpoint{Description: "创建了输出文件", Type: "must_do", Required: true}
	result, err := ct.Verify(context.Background(), cp, "我首先检查了目录，然后创建了 output.txt 文件，最后写入了结果")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Met {
		t.Error("expected checkpoint to be met")
	}
	if result.Confidence < 0.5 {
		t.Errorf("expected confidence >= 0.5, got %f", result.Confidence)
	}
	if result.Evidence == "" {
		t.Error("expected non-empty evidence")
	}
}

func TestVerifyNotMet(t *testing.T) {
	llm := &cpMockLLM{response: `{"met": false, "confidence": 0.8, "evidence": "agent 没有创建任何文件"}`}
	ct := NewCheckpointTool(llm)

	cp := skill.Checkpoint{Description: "创建了输出文件", Type: "must_do", Required: true}
	result, err := ct.Verify(context.Background(), cp, "我检查了目录但没有做任何操作")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Met {
		t.Error("expected checkpoint to not be met")
	}
}

func TestVerifyConfidenceClamping(t *testing.T) {
	// 测试置信度超出范围时的钳位处理
	llm := &cpMockLLM{response: `{"met": true, "confidence": 1.5, "evidence": "超出范围"}`}
	ct := NewCheckpointTool(llm)

	cp := skill.Checkpoint{Description: "测试置信度", Type: "must_do", Required: true}
	result, err := ct.Verify(context.Background(), cp, "trace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != 1.0 {
		t.Errorf("expected confidence clamped to 1.0, got %f", result.Confidence)
	}
}

func TestVerifyNegativeConfidenceClamping(t *testing.T) {
	llm := &cpMockLLM{response: `{"met": false, "confidence": -0.5, "evidence": "负值"}`}
	ct := NewCheckpointTool(llm)

	cp := skill.Checkpoint{Description: "测试负置信度", Type: "must_do", Required: false}
	result, err := ct.Verify(context.Background(), cp, "trace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != 0 {
		t.Errorf("expected confidence clamped to 0, got %f", result.Confidence)
	}
}

func TestVerifyInvalidJSON(t *testing.T) {
	llm := &cpMockLLM{response: "这不是 JSON 响应"}
	ct := NewCheckpointTool(llm)

	cp := skill.Checkpoint{Description: "测试", Type: "must_do", Required: true}
	_, err := ct.Verify(context.Background(), cp, "trace")
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestVerifyLLMError(t *testing.T) {
	llm := &errMockLLM{err: fmt.Errorf("LLM 服务不可用")}
	ct := NewCheckpointTool(llm)

	cp := skill.Checkpoint{Description: "测试", Type: "must_do", Required: true}
	_, err := ct.Verify(context.Background(), cp, "trace")
	if err == nil {
		t.Error("expected error when LLM fails")
	}
}

func TestVerifyBatch(t *testing.T) {
	batchResponse := `{"result": [
		{"met": true, "confidence": 0.85, "evidence": "agent 输出了帮助信息"},
		{"met": false, "confidence": 0.7, "evidence": "agent 没有检查依赖"}
	]}`
	llm := &cpMockLLM{response: batchResponse}
	ct := NewCheckpointTool(llm)

	checkpoints := []skill.Checkpoint{
		{Description: "输出帮助信息", Type: "output", Required: false},
		{Description: "检查依赖", Type: "must_do", Required: false},
	}
	results, err := ct.VerifyBatch(context.Background(), checkpoints, "agent 运行了 --help 命令")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Met {
		t.Error("expected first checkpoint to be met")
	}
	if results[1].Met {
		t.Error("expected second checkpoint to not be met")
	}
}

func TestVerifyBatchEmpty(t *testing.T) {
	llm := &cpMockLLM{response: ""}
	ct := NewCheckpointTool(llm)

	results, err := ct.VerifyBatch(context.Background(), nil, "trace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty checkpoints, got %v", results)
	}
}

func TestVerifyBatchCountMismatch(t *testing.T) {
	batchResponse := `{"result": [
		{"met": true, "confidence": 0.85, "evidence": "只有一个结果"}
	]}`
	llm := &cpMockLLM{response: batchResponse}
	ct := NewCheckpointTool(llm)

	checkpoints := []skill.Checkpoint{
		{Description: "检查点1", Type: "must_do", Required: false},
		{Description: "检查点2", Type: "must_do", Required: false},
	}
	_, err := ct.VerifyBatch(context.Background(), checkpoints, "trace")
	if err == nil {
		t.Error("expected error when result count doesn't match checkpoint count")
	}
}

func TestCalculateScoreWeighted(t *testing.T) {
	evals := []*CheckpointEval{
		{Checkpoint: skill.Checkpoint{Required: true}, Met: true, Confidence: 0.9},
		{Checkpoint: skill.Checkpoint{Required: true}, Met: true, Confidence: 0.8},
		{Checkpoint: skill.Checkpoint{Required: false}, Met: true, Confidence: 0.7},
	}
	// (2*0.9 + 2*0.8 + 1*0.7) / (2+2+1) = 4.1/5 = 0.82
	score, pass := calculateScore(evals)
	if score < 0.81 || score > 0.83 {
		t.Errorf("expected score ~0.82, got %.2f", score)
	}
	if !pass {
		t.Error("expected to pass with score 0.82")
	}
}

func TestCalculateScoreAllMet(t *testing.T) {
	evals := []*CheckpointEval{
		{Checkpoint: skill.Checkpoint{Required: true}, Met: true, Confidence: 1.0},
	}
	score, pass := calculateScore(evals)
	if score != 1.0 {
		t.Errorf("expected score 1.0, got %.2f", score)
	}
	if !pass {
		t.Error("expected to pass with perfect score")
	}
}

func TestCalculateScoreNoneMet(t *testing.T) {
	evals := []*CheckpointEval{
		{Checkpoint: skill.Checkpoint{Required: false}, Met: false, Confidence: 0},
	}
	score, pass := calculateScore(evals)
	if score != 0 {
		t.Errorf("expected score 0, got %.2f", score)
	}
	if pass {
		t.Error("expected not to pass with score 0")
	}
}

func TestCalculateScoreBelowThreshold(t *testing.T) {
	evals := []*CheckpointEval{
		{Checkpoint: skill.Checkpoint{Required: false}, Met: true, Confidence: 0.5},
		{Checkpoint: skill.Checkpoint{Required: false}, Met: false, Confidence: 0},
	}
	// (1*0.5 + 0) / 2 = 0.25
	score, pass := calculateScore(evals)
	if score < 0.24 || score > 0.26 {
		t.Errorf("expected score ~0.25, got %.2f", score)
	}
	if pass {
		t.Error("expected not to pass with score 0.25")
	}
}
