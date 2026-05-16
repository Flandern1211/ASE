package tools

import (
	"ASE/internal/skill"
	"context"
	"encoding/json"
	"fmt"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"strings"
	"sync"
)

type AgentSimTool struct {
	llm       model.BaseChatModel
	evaluator CheckpointEvaluator
}

func NewAgentSimTool(llm model.BaseChatModel, evaluator CheckpointEvaluator) *AgentSimTool {
	return &AgentSimTool{
		llm:       llm,
		evaluator: evaluator,
	}
}

// 模拟测试结果
type SimulateRsult struct {
	Pass  bool
	Score float64
	Trace string
}

// 执行模拟测试
func (a *AgentSimTool) Simulate(ctx context.Context, sk *skill.Skill, scenario *skill.TestScenario) (*SimulateRsult, error) {
	//1. 先获取对应的测试场景
	if scenario == nil {
		var err error
		scenario, err = a.generateScenario(ctx, sk)
		if err != nil {
			return nil, fmt.Errorf("生成测试场景失败:%w", err)
		}
	}
	//2. 让模拟LLMagent带着skill执行场景
	trace, err := a.simulateAgent(ctx, sk, scenario)
	if err != nil {
		return nil, fmt.Errorf("模拟Agent失败:%w", err)
	}
	return a.evaluateCheckpoints(ctx, scenario.Checkpoints, trace)
}

// 根据SKill生成对应测试场景
func (a *AgentSimTool) generateScenario(ctx context.Context, sk *skill.Skill) (*skill.TestScenario, error) {
	prompt := fmt.Sprintf(`根据以下 skill 的完整内容，生成一个测试场景来验证 skill 的有效性。

## Skill: %s
## 描述: %s

## SKILL.md 完整内容

%s

## 要求
1. 创建一个能触发 skill 使用的场景
2. 定义 3-5 个行为检查点（覆盖 skill 的核心要求和约束）
3. 每个检查点说明类型（must_do/must_not/order/output）

返回 JSON 格式：
{
  "name": "场景名称",
  "description": "场景描述",
  "steps": ["步骤1", "步骤2"],
  "checkpoints": [
    {"description": "检查点描述", "type": "must_do", "required": true}
  ]
}`, sk.Name, sk.Description, sk.RawContent)

	resp, err := a.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: "你是一个测试场景生成器。根据skill的完整内容（包含指导原则和约束）生成相应的测试场景"},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, err
	}

	return a.parseTestScenario(resp.Content)
}

// 解析Json数据成对应测试场景
func (a *AgentSimTool) parseTestScenario(content string) (*skill.TestScenario, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("无法从LLM的响应中提取到相应的Json格式数据")
	}
	jsonStr := content[start : end+1]
	var scenario *skill.TestScenario
	if err := json.Unmarshal([]byte(jsonStr), &scenario); err != nil {
		return nil, fmt.Errorf("在解析生成的测试环境时Json解析失败: %w", err)
	}
	return scenario, nil
}

// 测试skill执行的Agent
func (a *AgentSimTool) simulateAgent(ctx context.Context, sk *skill.Skill, scenario *skill.TestScenario) (string, error) {
	prompt := fmt.Sprintf(`你是一个 AI agent。现在你必须根据以下 skill 指导来执行任务。
		## Skill: %s
		## 描述: %s
		## SKILL.md 完整内容
		%s
		## 测试场景
		%s
		## 执行步骤
		%s
		请描述你将如何执行这个场景，包括你会采取的具体行动。
		重要：你必须严格遵循 skill 中的所有约束和指导原则。`,
		sk.Name,
		sk.Description,
		sk.RawContent,
		scenario.Description,
		strings.Join(scenario.Steps, "\n"))

	resp, err := a.llm.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// evaluateCheckpoints 使用 LLM-as-Judge 评估检查点
func (a *AgentSimTool) evaluateCheckpoints(ctx context.Context, checkpoints []skill.Checkpoint, trace string) (*SimulateRsult, error) {
	// 1. 拆分：required → 逐条, 非 required → 批量
	var required, optional []skill.Checkpoint
	for _, cp := range checkpoints {
		if cp.Required {
			required = append(required, cp)
		} else {
			optional = append(optional, cp)
		}
	}

	// 2. 并发评估
	var allEvals []*CheckpointEval
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cp := range required {
		wg.Add(1)
		go func(cp skill.Checkpoint) {
			defer wg.Done()
			eval, err := a.evaluator.Verify(ctx, cp, trace)
			if err != nil {
				eval = &CheckpointEval{Checkpoint: cp, Met: false, Confidence: 0, Evidence: "评估失败: " + err.Error()}
			}
			mu.Lock()
			allEvals = append(allEvals, eval)
			mu.Unlock()
		}(cp)
	}

	if len(optional) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			evals, err := a.evaluator.VerifyBatch(ctx, optional, trace)
			if err != nil {
				for _, cp := range optional {
					allEvals = append(allEvals, &CheckpointEval{Checkpoint: cp, Met: false, Confidence: 0, Evidence: "批量评估失败: " + err.Error()})
				}
				return
			}
			mu.Lock()
			allEvals = append(allEvals, evals...)
			mu.Unlock()
		}()
	}

	wg.Wait()

	// 3. 计算得分
	score, pass := calculateScore(allEvals)

	// 4. 硬约束检查
	for _, e := range allEvals {
		if e.Checkpoint.Required && !e.Met {
			pass = false
			break
		}
	}

	return &SimulateRsult{
		Pass:  pass,
		Score: score,
		Trace: trace,
	}, nil
}

// calculateScore 根据 checkpoint 评估结果计算加权得分
func calculateScore(evals []*CheckpointEval) (float64, bool) {
	totalWeight, weightedSum := 0.0, 0.0
	for _, e := range evals {
		weight := 1.0
		if e.Checkpoint.Required {
			weight = 2.0
		}
		totalWeight += weight
		if e.Met {
			weightedSum += weight * e.Confidence
		}
	}
	score := weightedSum / totalWeight
	return score, score >= 0.7
}
