package tools

import (
	"ASE/internal/skill"
	"context"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type validateMockLLM struct {
	response string
	err      error
}

func (m *validateMockLLM) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &schema.Message{
		Role:    schema.Assistant,
		Content: m.response,
	}, nil
}

func (m *validateMockLLM) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("Stream not implemented in mock")
}

func TestGenerateScript(t *testing.T) {
	llm := &validateMockLLM{response: `#!/bin/bash
set -e
echo "开始执行"
mkdir -p output
echo "hello" > output/result.txt
echo "完成"`}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{
		Name:        "test-skill",
		Description: "创建输出文件",
		RawContent:  "# Test Skill\n\n创建 output/result.txt 文件并写入 hello",
	}
	script, err := vt.GenerateScript(context.Background(), sk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if script == "" {
		t.Error("expected non-empty script")
	}
	if script != `#!/bin/bash
set -e
echo "开始执行"
mkdir -p output
echo "hello" > output/result.txt
echo "完成"` {
		t.Errorf("unexpected script content: %s", script)
	}
}

func TestGenerateScriptWithSupportFiles(t *testing.T) {
	llm := &validateMockLLM{response: `#!/bin/bash
echo "using support file"`}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{
		Name:        "test-skill",
		Description: "使用辅助文件",
		RawContent:  "# Test",
		SupportFiles: map[string]string{
			"config.yaml": "key: value",
			"script.sh":   "echo hello",
		},
	}
	script, err := vt.GenerateScript(context.Background(), sk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if script == "" {
		t.Error("expected non-empty script")
	}
}

func TestGenerateScriptLLMError(t *testing.T) {
	llm := &validateMockLLM{err: fmt.Errorf("LLM 超时")}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{Name: "test", RawContent: "# Test"}
	_, err := vt.GenerateScript(context.Background(), sk)
	if err == nil {
		t.Error("expected error when LLM fails")
	}
}

func TestValidateOutputPassed(t *testing.T) {
	llm := &validateMockLLM{response: `{"passed": true, "summary": "执行成功", "details": "脚本正确创建了文件并输出了内容"}`}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{Name: "test", Description: "创建文件", RawContent: "# Test"}
	result, err := vt.ValidateOutput(context.Background(), sk, "hello\n", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected validation to pass")
	}
	if result.Summary != "执行成功" {
		t.Errorf("expected summary '执行成功', got '%s'", result.Summary)
	}
}

func TestValidateOutputFailed(t *testing.T) {
	llm := &validateMockLLM{response: `{"passed": false, "summary": "执行失败", "details": "脚本报错退出"}`}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{Name: "test", Description: "创建文件", RawContent: "# Test"}
	result, err := vt.ValidateOutput(context.Background(), sk, "", "Error: permission denied", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected validation to fail")
	}
}

func TestValidateOutputInvalidJSON(t *testing.T) {
	llm := &validateMockLLM{response: "无法判断结果"}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{Name: "test", RawContent: "# Test"}
	result, err := vt.ValidateOutput(context.Background(), sk, "output", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected validation to fail for unparseable response")
	}
	if result.Summary != "无法解析验证结果" {
		t.Errorf("expected summary '无法解析验证结果', got '%s'", result.Summary)
	}
}

func TestValidateOutputMalformedJSON(t *testing.T) {
	llm := &validateMockLLM{response: `{"passed": true, broken`}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{Name: "test", RawContent: "# Test"}
	result, err := vt.ValidateOutput(context.Background(), sk, "output", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected validation to fail for malformed JSON")
	}
}

func TestValidateOutputLLMError(t *testing.T) {
	llm := &validateMockLLM{err: fmt.Errorf("网络错误")}
	vt := NewValidateTool(llm)

	sk := &skill.Skill{Name: "test", RawContent: "# Test"}
	_, err := vt.ValidateOutput(context.Background(), sk, "output", "", 0)
	if err == nil {
		t.Error("expected error when LLM fails")
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc..."},
	}

	for _, tt := range tests {
		result := truncateStr(tt.input, tt.max)
		if result != tt.expected {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.expected)
		}
	}
}

func TestBuildGenerateScriptPrompt(t *testing.T) {
	sk := &skill.Skill{
		Name:        "my-skill",
		Description: "测试 skill",
		RawContent:  "# 内容",
		SupportFiles: map[string]string{
			"file.txt": "content",
		},
	}
	prompt := buildGenerateScriptPrompt(sk)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !contains(prompt, "my-skill") {
		t.Error("expected prompt to contain skill name")
	}
	if !contains(prompt, "file.txt") {
		t.Error("expected prompt to contain support file name")
	}
}

func TestBuildValidateOutputPrompt(t *testing.T) {
	sk := &skill.Skill{Name: "test", Description: "desc", RawContent: "# Test"}
	prompt := buildValidateOutputPrompt(sk, "stdout output", "stderr output", 0)
	if !contains(prompt, "stdout output") {
		t.Error("expected prompt to contain stdout")
	}
	if !contains(prompt, "stderr output") {
		t.Error("expected prompt to contain stderr")
	}
	if !contains(prompt, "退出码: 0") {
		t.Error("expected prompt to contain exit code")
	}
}

func TestBuildValidateOutputPromptNoStderr(t *testing.T) {
	sk := &skill.Skill{Name: "test", Description: "desc", RawContent: "# Test"}
	prompt := buildValidateOutputPrompt(sk, "output", "", 0)
	if contains(prompt, "Stderr") {
		t.Error("expected prompt to not contain Stderr section when empty")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
