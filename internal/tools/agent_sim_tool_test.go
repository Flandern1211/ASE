package tools

import (
	"ASE/internal/skill"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type simMockLLM struct {
	responses []string
	idx       int
}

func (m *simMockLLM) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.idx >= len(m.responses) {
		return nil, fmt.Errorf("mock LLM: no more responses (idx=%d)", m.idx)
	}
	resp := m.responses[m.idx]
	m.idx++
	return &schema.Message{Role: schema.Assistant, Content: resp}, nil
}

func (m *simMockLLM) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("Stream not implemented in mock")
}

type errMockLLM struct {
	err error
}

func (m *errMockLLM) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return nil, m.err
}

func (m *errMockLLM) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, m.err
}

type mockEvaluator struct {
	verifyResults map[string]*CheckpointEval
	batchResults  []*CheckpointEval
	verifyErr     error
	batchErr      error
}

func (m *mockEvaluator) Verify(ctx context.Context, checkpoint skill.Checkpoint, trace string) (*CheckpointEval, error) {
	if m.verifyErr != nil {
		return nil, m.verifyErr
	}
	if result, ok := m.verifyResults[checkpoint.Description]; ok {
		return result, nil
	}
	return &CheckpointEval{Checkpoint: checkpoint, Met: false, Confidence: 0, Evidence: "not found"}, nil
}

func (m *mockEvaluator) VerifyBatch(ctx context.Context, checkpoints []skill.Checkpoint, trace string) ([]*CheckpointEval, error) {
	if m.batchErr != nil {
		return nil, m.batchErr
	}
	return m.batchResults, nil
}

func TestSimulateWithEvaluator(t *testing.T) {
	scenarioJSON, _ := json.Marshal(map[string]interface{}{
		"name": "test", "description": "test", "steps": []string{"step1"},
		"checkpoints": []map[string]interface{}{
			{"description": "创建了输出文件", "type": "must_do", "required": true},
			{"description": "输出帮助信息", "type": "output", "required": false},
		},
	})

	llm := &simMockLLM{responses: []string{string(scenarioJSON), "我创建了文件并输出了帮助"}}
	evaluator := &mockEvaluator{
		verifyResults: map[string]*CheckpointEval{
			"创建了输出文件": {Met: true, Confidence: 0.9, Evidence: "创建了 output.txt"},
		},
		batchResults: []*CheckpointEval{
			{Checkpoint: skill.Checkpoint{Description: "输出帮助信息"}, Met: true, Confidence: 0.8, Evidence: "输出了帮助"},
		},
	}

	at := NewAgentSimTool(llm, evaluator)
	result, err := at.Simulate(context.Background(), &skill.Skill{Name: "test", RawContent: "# Test"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Pass {
		t.Error("expected simulation to pass")
	}
	if result.Score < 0.7 {
		t.Errorf("expected score >= 0.7, got %f", result.Score)
	}
}

func TestSimulateRequiredFails(t *testing.T) {
	scenarioJSON, _ := json.Marshal(map[string]interface{}{
		"name": "test", "description": "test", "steps": []string{"step1"},
		"checkpoints": []map[string]interface{}{
			{"description": "必须做的事", "type": "must_do", "required": true},
		},
	})

	llm := &simMockLLM{responses: []string{string(scenarioJSON), "我什么都没做"}}
	evaluator := &mockEvaluator{
		verifyResults: map[string]*CheckpointEval{
			"必须做的事": {Met: false, Confidence: 0.9, Evidence: "没有执行"},
		},
	}

	at := NewAgentSimTool(llm, evaluator)
	result, err := at.Simulate(context.Background(), &skill.Skill{Name: "test", RawContent: "# Test"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass {
		t.Error("expected simulation to fail when required checkpoint not met")
	}
}

func TestSimulateWithProvidedScenario(t *testing.T) {
	scenario := &skill.TestScenario{
		Name:        "预定义场景",
		Description: "测试预定义场景",
		Steps:       []string{"step1"},
		Checkpoints: []skill.Checkpoint{
			{Description: "检查点A", Type: "must_do", Required: true},
		},
	}

	llm := &simMockLLM{responses: []string{"我执行了检查点A"}}
	evaluator := &mockEvaluator{
		verifyResults: map[string]*CheckpointEval{
			"检查点A": {Met: true, Confidence: 0.95, Evidence: "已执行"},
		},
	}

	at := NewAgentSimTool(llm, evaluator)
	result, err := at.Simulate(context.Background(), &skill.Skill{Name: "test", RawContent: "# Test"}, scenario)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Pass {
		t.Error("expected simulation to pass")
	}
}

func TestSimulateGenerateScenarioFails(t *testing.T) {
	llm := &errMockLLM{err: fmt.Errorf("LLM 不可用")}
	evaluator := &mockEvaluator{}

	at := NewAgentSimTool(llm, evaluator)
	_, err := at.Simulate(context.Background(), &skill.Skill{Name: "test", RawContent: "# Test"}, nil)
	if err == nil {
		t.Error("expected error when scenario generation fails")
	}
}

func TestSimulateAgentFails(t *testing.T) {
	scenarioJSON, _ := json.Marshal(map[string]interface{}{
		"name": "test", "description": "test", "steps": []string{"step1"},
		"checkpoints": []map[string]interface{}{
			{"description": "cp1", "type": "must_do", "required": true},
		},
	})

	llm := &simMockLLM{responses: []string{string(scenarioJSON)}}
	// 第二次调用会没有更多响应，触发错误
	evaluator := &mockEvaluator{}

	at := NewAgentSimTool(llm, evaluator)
	_, err := at.Simulate(context.Background(), &skill.Skill{Name: "test", RawContent: "# Test"}, nil)
	if err == nil {
		t.Error("expected error when agent simulation fails")
	}
}

func TestSimulateEvaluatorVerifyError(t *testing.T) {
	scenarioJSON, _ := json.Marshal(map[string]interface{}{
		"name": "test", "description": "test", "steps": []string{"step1"},
		"checkpoints": []map[string]interface{}{
			{"description": "cp1", "type": "must_do", "required": true},
		},
	})

	llm := &simMockLLM{responses: []string{string(scenarioJSON), "执行了任务"}}
	evaluator := &mockEvaluator{
		verifyErr: fmt.Errorf("评估器错误"),
	}

	at := NewAgentSimTool(llm, evaluator)
	result, err := at.Simulate(context.Background(), &skill.Skill{Name: "test", RawContent: "# Test"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 评估失败时应将 checkpoint 标记为未满足
	if result.Pass {
		t.Error("expected simulation to fail when evaluator returns error")
	}
}

func TestSimulateEvaluatorBatchError(t *testing.T) {
	scenarioJSON, _ := json.Marshal(map[string]interface{}{
		"name": "test", "description": "test", "steps": []string{"step1"},
		"checkpoints": []map[string]interface{}{
			{"description": "optional1", "type": "output", "required": false},
		},
	})

	llm := &simMockLLM{responses: []string{string(scenarioJSON), "执行了任务"}}
	evaluator := &mockEvaluator{
		batchErr: fmt.Errorf("批量评估失败"),
	}

	at := NewAgentSimTool(llm, evaluator)
	result, err := at.Simulate(context.Background(), &skill.Skill{Name: "test", RawContent: "# Test"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 批量评估失败时应将所有 optional checkpoint 标记为未满足
	if result.Pass {
		t.Error("expected simulation to fail when batch evaluator returns error")
	}
}

func TestParseTestScenario(t *testing.T) {
	at := &AgentSimTool{}

	content := `这是 LLM 的响应：
		{"name": "测试场景", "description": "描述", "steps": ["步骤1"], "checkpoints": [{"description": "cp1", "type": "must_do", "required": true}]}
		以上是 JSON`
	scenario, err := at.parseTestScenario(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scenario.Name != "测试场景" {
		t.Errorf("expected name '测试场景', got '%s'", scenario.Name)
	}
	if len(scenario.Checkpoints) != 1 {
		t.Errorf("expected 1 checkpoint, got %d", len(scenario.Checkpoints))
	}
}

func TestParseTestScenarioInvalidJSON(t *testing.T) {
	at := &AgentSimTool{}

	_, err := at.parseTestScenario("这里没有 JSON")
	if err == nil {
		t.Error("expected error for content without JSON")
	}
}

func TestParseTestScenarioMalformedJSON(t *testing.T) {
	at := &AgentSimTool{}

	_, err := at.parseTestScenario(`{"name": "test", invalid}`)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestSimulateScoreThreshold(t *testing.T) {
	scenarioJSON, _ := json.Marshal(map[string]interface{}{
		"name": "test", "description": "test", "steps": []string{"step1"},
		"checkpoints": []map[string]interface{}{
			{"description": "cp1", "type": "output", "required": false},
			{"description": "cp2", "type": "output", "required": false},
		},
	})

	llm := &simMockLLM{responses: []string{string(scenarioJSON), "执行"}}
	evaluator := &mockEvaluator{
		batchResults: []*CheckpointEval{
			{Checkpoint: skill.Checkpoint{Description: "cp1"}, Met: true, Confidence: 0.6},
			{Checkpoint: skill.Checkpoint{Description: "cp2"}, Met: false, Confidence: 0},
		},
	}

	at := NewAgentSimTool(llm, evaluator)
	result, err := at.Simulate(context.Background(), &skill.Skill{Name: "test", RawContent: "# Test"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// (0.6 + 0) / 2 = 0.3 < 0.7
	if result.Pass {
		t.Error("expected simulation to fail with low score")
	}
	if result.Score >= 0.7 {
		t.Errorf("expected score < 0.7, got %f", result.Score)
	}
}
