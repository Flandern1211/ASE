package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// llmMetaResult 是 LLM 元数据提取的 JSON 结构
type llmMetaResult struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Frontmatter    map[string]interface{} `json:"frontmatter"`
	Deps           []Dependency           `json:"deps"`
	Purpose        string                 `json:"purpose"`
	UseCases       []string               `json:"use_cases"`
	CorePrinciples json.RawMessage        `json:"core_principles"`
	SkillType      string                 `json:"skill_type"`
	HasExecutable  bool                   `json:"has_executable"`
	HasGuidance    bool                   `json:"has_guidance"`
}

// Parse 从目录读取 skill，使用 LLM 提取元数据。
// 完整 SKILL.md 文本保存在 RawContent 中，供执行时使用。
func Parse(path string, llm model.BaseChatModel) (*Skill, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	var skillDir, skillFile string
	if info.IsDir() {
		skillDir = path
		skillFile = filepath.Join(path, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			return nil, fmt.Errorf("在 %s 中未找到 SKILL.md", path)
		}
	} else {
		skillFile = path
		skillDir = filepath.Dir(path)
	}

	// 读取 SKILL.md 原始内容
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("读取 skill 文件失败: %w", err)
	}
	rawContent := string(data)

	// 验证 frontmatter 存在
	if !strings.HasPrefix(strings.TrimSpace(rawContent), "---") {
		return nil, fmt.Errorf("SKILL.md 缺少 YAML frontmatter（应以 --- 开头）")
	}

	// 扫描辅助文件
	supportFiles, err := scanSupportFiles(skillDir)
	if err != nil {
		return nil, fmt.Errorf("扫描辅助文件失败: %w", err)
	}

	// 使用 LLM 提取元数据
	skill, err := extractMetadata(context.Background(), llm, rawContent)
	if err != nil {
		return nil, fmt.Errorf("LLM 元数据提取失败: %w", err)
	}

	// 填充运行时字段
	skill.Path = skillFile
	skill.Dir = skillDir
	skill.RawContent = rawContent
	skill.SupportFiles = supportFiles

	return skill, nil
}

// scanSupportFiles 扫描 skill 目录中的辅助文件，返回相对路径 → 绝对路径映射
func scanSupportFiles(skillDir string) (map[string]string, error) {
	files := make(map[string]string)

	// 扫描根目录下的非 SKILL.md 文件
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 跳过 SKILL.md 和隐藏文件
		if strings.EqualFold(name, "SKILL.md") || strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(skillDir, name)
		files[name] = fullPath
	}

	// 扫描子目录（scripts, tests, config 等）
	supportDirs := []string{"scripts", "tests", "config"}
	for _, dir := range supportDirs {
		dirPath := filepath.Join(skillDir, dir)
		if _, existErr := os.Stat(dirPath); os.IsNotExist(existErr) {
			continue
		}

		walkErr := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			relPath, relErr := filepath.Rel(skillDir, path)
			if relErr != nil {
				return relErr
			}
			files[relPath] = path
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}

	return files, nil
}

// extractMetadata 使用 LLM 从 SKILL.md 内容中提取元数据
func extractMetadata(ctx context.Context, llm model.BaseChatModel, content string) (*Skill, error) {
	prompt := fmt.Sprintf("## SKILL.md 内容\n\n%s\n\n请分析以上内容，提取元数据。返回 JSON 格式。", content)

	resp, err := llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: metaExtractPrompt},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	return parseMetaResponse(resp.Content)
}

const metaExtractPrompt = `你是一个 Skill 元数据提取器。分析 SKILL.md 内容，提取元数据信息。

输出必须是 JSON 格式，包含以下字段：
- name: skill 名称（从 frontmatter 提取）
- description: skill 描述（从 frontmatter 提取）
- frontmatter: 原始 frontmatter 对象（保留所有字段）
- deps: 环境依赖数组，每个包含 name, version, type（如无法确定，返回空数组）
- purpose: skill 的核心目的（一句话概括）
- use_cases: 适用场景列表
- core_principles: 核心原则或约束
- skill_type: "executable"（只有命令）, "instructional"（只有指导）, 或 "mixed"（混合）
- has_executable: 是否包含可执行命令
- has_guidance: 是否包含指导原则/约束规则

注意：不要解析具体步骤，只提取元数据。`

func parseMetaResponse(content string) (*Skill, error) {
	content = strings.TrimSpace(content)

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("无法从 LLM 响应中提取 JSON")
	}

	jsonStr := content[start : end+1]
	var result llmMetaResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	if result.Name == "" {
		return nil, fmt.Errorf("LLM 未提取到 skill 名称")
	}
	if result.Description == "" {
		return nil, fmt.Errorf("LLM 未提取到 skill 描述")
	}

	return &Skill{
		Name:           result.Name,
		Description:    result.Description,
		Frontmatter:    result.Frontmatter,
		Deps:           result.Deps,
		Purpose:        result.Purpose,
		UseCases:       result.UseCases,
		CorePrinciples: rawMessageToString(result.CorePrinciples),
		SkillType:      result.SkillType,
		HasExecutable:  result.HasExecutable,
		HasGuidance:    result.HasGuidance,
	}, nil
}

// rawMessageToString 将 json.RawMessage 转为字符串。
// 兼容 LLM 返回 string 或 []string 两种格式。
func rawMessageToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// 尝试解析为 string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// 尝试解析为 []string，用 ", " 连接
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return strings.Join(arr, ", ")
	}

	// 都失败，返回原始字符串
	return string(raw)
}
