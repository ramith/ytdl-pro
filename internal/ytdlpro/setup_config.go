package ytdlpro

import (
	"fmt"
	"strings"
)

type SetupConfig struct {
	SkipRuntime bool
	SkipModel   bool
	SkipBuild   bool
	Force       bool
	ModelURL    string
	ModelPath   string
}

func DefaultSetupConfig() SetupConfig {
	return SetupConfig{
		ModelURL:  "https://huggingface.co/bartowski/Qwen_Qwen3-1.7B-GGUF/resolve/main/Qwen_Qwen3-1.7B-Q4_K_M.gguf?download=true",
		ModelPath: "./models/qwen3-1.7b-instruct-q4_k_m.gguf",
	}
}

func (c SetupConfig) Validate() error {
	if !c.SkipModel {
		if strings.TrimSpace(c.ModelURL) == "" {
			return fmt.Errorf("missing model download URL")
		}
		if strings.TrimSpace(c.ModelPath) == "" {
			return fmt.Errorf("missing model path")
		}
	}
	return nil
}
