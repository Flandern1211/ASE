package skill

import (
	"testing"
	"time"
)

func TestSkillFields(t *testing.T) {
	s := Skill{
		Name:        "test-skill",
		Description: "Use when testing skill parsing",
		Path:        "/skills/test-skill/SKILL.md",
		Dir:         "/skills/test-skill/",
		Frontmatter: map[string]interface{}{
			"name":        "test-skill",
			"description": "Use when testing skill parsing",
		},
		Deps: []Dependency{
			{Name: "go", Version: ">=1.21", Type: "language"},
		},
		Purpose:        "Test skill parsing functionality",
		UseCases:       []string{"unit testing", "integration testing"},
		CorePrinciples: "Parse accurately",
		SkillType:      "mixed",
		HasExecutable:  true,
		HasGuidance:    true,
		SupportFiles:   map[string]string{"scripts/setup.sh": "#!/bin/bash\necho setup"},
	}

	if s.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", s.Name)
	}
	if len(s.Deps) != 1 {
		t.Errorf("expected 1 dep, got %d", len(s.Deps))
	}
	if s.SkillType != "mixed" {
		t.Errorf("expected skill_type 'mixed', got %q", s.SkillType)
	}
	if !s.HasExecutable {
		t.Error("expected has_executable=true")
	}
	if !s.HasGuidance {
		t.Error("expected has_guidance=true")
	}
}

func TestTestScenario(t *testing.T) {
	ts := TestScenario{
		Name:        "test-scenario",
		Description: "Test if agent follows TDD",
		Steps:       []string{"Write failing test", "Run test", "Write code"},
		Checkpoints: []Checkpoint{
			{Description: "writes test first", Type: "must_do", Required: true},
			{Description: "runs test before code", Type: "order", Required: true},
		},
	}

	if ts.Name != "test-scenario" {
		t.Errorf("expected name 'test-scenario', got %q", ts.Name)
	}
	if len(ts.Checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(ts.Checkpoints))
	}
}

func TestExecResult(t *testing.T) {
	r := ExecResult{
		SkillName: "test-skill",
		StepName:  "step1",
		ExitCode:  0,
		Stdout:    "ok",
		Success:   true,
		Duration:  100 * time.Millisecond,
	}

	if !r.Success {
		t.Error("expected success=true")
	}
}

func TestAnalysis(t *testing.T) {
	a := Analysis{
		SkillName: "test-skill",
		StepName:  "step1",
		Reason:    "missing dependency",
		Suggest:   "add go mod tidy step",
		FixType:   "command",
	}

	if a.Reason == "" {
		t.Error("expected non-empty reason")
	}
	if a.FixType != "command" {
		t.Errorf("expected fix_type 'command', got %q", a.FixType)
	}
}
