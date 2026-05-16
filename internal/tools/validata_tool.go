package tools

import (
	"ASE/internal/skill"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ValidationResult 持有验证结果。
type ValidationResult struct {
	Passed  bool   `json:"passed"`
	Summary string `json:"summary"`
	Details string `json:"details"`
}

// ValidateTool 使用 LLM 读取完整 SKILL.md 并生成执行脚本，在 Docker 中执行后验证。
type ValidateTool struct {
	llm model.BaseChatModel
}

// NewValidateTool 创建 ValidateTool。
func NewValidateTool(llm model.BaseChatModel) *ValidateTool {
	return &ValidateTool{llm: llm}
}

const generateScriptSystemPrompt = `你是一个 Skill 执行器。阅读完整的 SKILL.md 内容，生成一个可执行的 shell 脚本。

规则：
1. 提取所有可执行的 bash 命令，按正确顺序组织
2. 如果 skill 是指导型（instructional），将指导转化为具体命令
3. 脚本应该可独立运行，不需要人工交互
4. 在关键步骤后添加 echo 输出进度
5. 任何步骤失败时立即退出（set -e）
6. 只输出脚本内容，不要解释`

// GenerateScript 让 LLM 根据完整 SKILL.md 生成可执行的 shell 脚本。
func (v *ValidateTool) GenerateScript(ctx context.Context, sk *skill.Skill) (string, error) {
	prompt := buildGenerateScriptPrompt(sk)

	resp, err := v.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: generateScriptSystemPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("LLM 调用失败: %w", err)
	}

	return strings.TrimSpace(resp.Content), nil
}

func buildGenerateScriptPrompt(sk *skill.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Skill: %s\n", sk.Name)
	fmt.Fprintf(&b, "## 描述: %s\n\n", sk.Description)
	fmt.Fprintf(&b, "## SKILL.md 完整内容\n\n%s\n\n", sk.RawContent)

	if len(sk.SupportFiles) > 0 {
		fmt.Fprintf(&b, "## 辅助文件\n\n")
		for path, content := range sk.SupportFiles {
			truncated := content
			if len(truncated) > 500 {
				truncated = truncated[:500] + "..."
			}
			fmt.Fprintf(&b, "### %s\n```\n%s\n```\n\n", path, truncated)
		}
	}

	fmt.Fprintf(&b, "\n请根据以上内容生成可执行脚本。\n")
	return b.String()
}

const validateOutputSystemPrompt = `你是一个 Skill 验证器。判断执行结果是否符合 SKILL.md 的预期。

输出必须是 JSON 格式：
{"passed": true/false, "summary": "一句话总结", "details": "详细说明"}

判断标准：
1. 脚本是否执行了 skill 描述的核心操作？
2. 输出中是否有错误或异常？
3. 是否达到了 skill 的预期目标？`

// ValidateOutput 验证执行输出是否符合 skill 预期。
func (v *ValidateTool) ValidateOutput(ctx context.Context, sk *skill.Skill, stdout, stderr string, exitCode int) (*ValidationResult, error) {
	prompt := buildValidateOutputPrompt(sk, stdout, stderr, exitCode)

	resp, err := v.llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: validateOutputSystemPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parseValidationResult(resp.Content)
}

func buildValidateOutputPrompt(sk *skill.Skill, stdout, stderr string, exitCode int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Skill: %s\n", sk.Name)
	fmt.Fprintf(&b, "## 描述: %s\n\n", sk.Description)
	fmt.Fprintf(&b, "## 执行结果\n")
	fmt.Fprintf(&b, "退出码: %d\n", exitCode)
	if stdout != "" {
		fmt.Fprintf(&b, "Stdout:\n```\n%s\n```\n", truncateStr(stdout, 2000))
	}
	if stderr != "" {
		fmt.Fprintf(&b, "Stderr:\n```\n%s\n```\n", truncateStr(stderr, 2000))
	}
	fmt.Fprintf(&b, "\n请判断执行结果是否符合 skill 预期。返回 JSON。\n")
	return b.String()
}

func parseValidationResult(content string) (*ValidationResult, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return &ValidationResult{
			Passed:  false,
			Summary: "无法解析验证结果",
			Details: content,
		}, nil
	}

	jsonStr := content[start : end+1]
	var result struct {
		Passed  bool   `json:"passed"`
		Summary string `json:"summary"`
		Details string `json:"details"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return &ValidationResult{
			Passed:  false,
			Summary: "JSON 解析失败",
			Details: content,
		}, nil
	}

	return &ValidationResult{
		Passed:  result.Passed,
		Summary: result.Summary,
		Details: result.Details,
	}, nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
