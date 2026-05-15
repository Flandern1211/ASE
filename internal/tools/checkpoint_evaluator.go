package tools

import (
	"ASE/internal/skill"
	"context"
)

// CheckpointEval 检查点的评估结果
type CheckpointEval struct {
	Checkpoint skill.Checkpoint
	Met        bool
	Confidence float64
	Evidence   string
}

type CheckpointEvaluator interface {
	//单次评估 主要是用于特殊检查点
	Verify(ctx context.Context, checkpoint skill.Checkpoint, trace string) (*CheckpointEval, error)
	//批量评估
	VerifyBatch(ctx context.Context, checkpoints []skill.Checkpoint, trace string) ([]*CheckpointEval, error)
}
