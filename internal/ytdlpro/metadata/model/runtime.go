package model

import (
	"context"
	"errors"

	"ytdl-pro/internal/ytdlpro/metadata/model/spec"
)

var ErrRuntimeUnavailable = errors.New("metadata runtime unavailable")

type Runtime interface {
	GenerateJSON(ctx context.Context, prompt string, opts GenerateOptions) (string, error)
	Close() error
}

type GenerateOptions = spec.GenerateOptions

type Config struct {
	Runtime         string
	Model           string
	ModelPath       string
	GrammarPath     string
	ContextTokens   int
	MaxOutputTokens int
	Temperature     float32
	TopP            float32
	Threads         string
	GPULayers       string
	Debug           bool
	Explain         bool
}

func NewRuntime(cfg Config) (Runtime, error) {
	return newDefaultRuntime(cfg)
}
