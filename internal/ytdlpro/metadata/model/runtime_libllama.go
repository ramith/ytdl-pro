//go:build libllama

package model

import "ytdl-pro/internal/ytdlpro/metadata/model/llama"

func newDefaultRuntime(cfg Config) (Runtime, error) {
	return llama.NewRuntime(llama.Config{
		Model:           cfg.Model,
		ModelPath:       cfg.ModelPath,
		GrammarPath:     cfg.GrammarPath,
		ContextTokens:   cfg.ContextTokens,
		MaxOutputTokens: cfg.MaxOutputTokens,
		Threads:         cfg.Threads,
		GPULayers:       cfg.GPULayers,
		Debug:           cfg.Debug,
		Explain:         cfg.Explain,
	})
}
