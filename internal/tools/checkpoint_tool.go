package tools

import (
	"ASE/internal/skill"
	"context"
	"encoding/json"
	"fmt"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"strings"
)

const VerifySystemPrompt = `你是一个行为验证器。评估 agent 行为是否满足检查点要求。
返回 JSON 格式：
{"met": true/false, "confidence": 0.0-1.0, "evidence": "具体引用 agent 行为中的证据"}
- met: 行为是否满足检查点
- confidence: 你对判断的置信度（0-1 之间的小数）
- evidence: 从 agent 行为记录中引用的具体证据`

const VerifyBatchSystemPrompt = `你是一个行为验证器。批量评估 agent 行为是否满足多个检查点要求。
返回 JSON 格式：
{"result": [{"met": true/false, "confidence": 0.0-1.0, "evidence": "具体证据"}]}
- result 数组的顺序必须与输入的检查点顺序一致
- met: 行为是否满足该检查点
- confidence: 你对判断的置信度（0-1 之间的小数）
- evidence: 从 agent 行为记录中引用的具体证据`

type CheckpointTool struct {
	llm model.BaseChatModel
}

func NewCheckpointTool(llm model.BaseChatModel) *CheckpointTool {
	return &CheckpointTool{llm: llm}
}

// Verify 逐条评估单个 checkpoint（用于 required=true 的关键检查点）
func (c *CheckpointTool) Verify(ctx context.Context, checkpoint skill.Checkpoint, trace string) (*CheckpointEval, error) {
	prompt := fmt.Sprintf(`## 检查点
描述: %s
类型: %s
是否必须: %v

## Agent 行为记录
%s

请判断检查点是否满足，返回 JSON。`,
		checkpoint.Description,
		checkpoint.Type,
		checkpoint.Required,
		truncateStr(trace, 2000))

	resp, err := c.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: VerifySystemPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parseCheckpointEval(resp.Content, checkpoint)
}

// 批量评估
func (c *CheckpointTool) VerifyBatch(ctx context.Context, checkpoints []skill.Checkpoint, trace string) ([]*CheckpointEval, error) {
	if len(checkpoints) == 0 {
		return nil, nil
	}

	var b strings.Builder

	b.WriteString("## 检查点列表\n\n")
	for i, cp := range checkpoints {
		fmt.Fprintf(&b, "%d. 描述: %s | 类型: %s | 必须: %v\n", i+1, cp.Description, cp.Type, cp.Required)
	}
	fmt.Fprintf(&b, "\n## Agent 行为记录\n\n%s\n\n请批量评估所有检查点，返回 JSON。", truncateStr(trace, 2000))

	resp, err := c.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: VerifyBatchSystemPrompt},
		{Role: schema.User, Content: b.String()},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM调用失败:%w", err)
	}
	return parseBatchCheckpointEval(resp.Content, checkpoints)

}

// 解析模型单条评估结果
func parseCheckpointEval(content string, checkpoint skill.Checkpoint) (*CheckpointEval, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("无法从 LLM 响应中提取 JSON: %s", truncateStr(content, 100))
	}

	jsonStr := content[start : end+1]
	var result struct {
		Met        bool    `json:"met"`
		Confidence float64 `json:"confidence"`
		Evidence   string  `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w, 原文: %s", err, truncateStr(content, 100))
	}

	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1 {
		result.Confidence = 1
	}

	return &CheckpointEval{
		Checkpoint: checkpoint,
		Met:        result.Met,
		Confidence: result.Confidence,
		Evidence:   result.Evidence,
	}, nil
}

// 解析模型返回的批量检查结果
func parseBatchCheckpointEval(content string, checkpoints []skill.Checkpoint) ([]*CheckpointEval, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("无法从LLm的响应中提取到Json结构的内容")
	}
	jsonStr := content[start : end+1]

	// 兼容 "result" 和 "results" 两种 key
	var batchResult struct {
		Results []struct {
			Met        bool    `json:"met"`
			Confidence float64 `json:"confidence"`
			Evidence   string  `json:"evidence"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &batchResult); err != nil || len(batchResult.Results) == 0 {
		// 尝试 "results" (复数)
		var batchResult2 struct {
			Results []struct {
				Met        bool    `json:"met"`
				Confidence float64 `json:"confidence"`
				Evidence   string  `json:"evidence"`
			} `json:"results"`
		}
		if err2 := json.Unmarshal([]byte(jsonStr), &batchResult2); err2 == nil && len(batchResult2.Results) > 0 {
			batchResult.Results = batchResult2.Results
		} else if err != nil {
			return nil, fmt.Errorf("JSON解析失败:%w, 原文 %s", err, content)
		}
	}
	if len(batchResult.Results) != len(checkpoints) {
		return nil, fmt.Errorf("返回结果数量 (%d) 与检查点数量 (%d) 不匹配", len(batchResult.Results), len(checkpoints))
	}
	evals := make([]*CheckpointEval, len(checkpoints))

	for i, r := range batchResult.Results {
		conf := r.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}
		evals[i] = &CheckpointEval{
			Checkpoint: checkpoints[i],
			Met:        r.Met,
			Confidence: conf,
			Evidence:   r.Evidence,
		}
	}
	return evals, nil
}
