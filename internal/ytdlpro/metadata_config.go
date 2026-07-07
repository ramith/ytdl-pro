package ytdlpro

import (
	"fmt"
	"strings"
	"time"
)

type MetadataConfig struct {
	Enabled             bool
	DryRun              bool
	Write               bool
	ReviewOnly          bool
	Path                string
	Recursive           bool
	Explain             bool
	Debug               bool
	WriteBaseTags       bool
	Model               string
	Runtime             string
	ModelPath           string
	GrammarPath         string
	ContextTokens       int
	MaxOutputTokens     int
	Threads             string
	GPULayers           string
	MinFullConfidence   float64
	MinFieldConfidence  float64
	MinReviewConfidence float64
	JSONReport          string
	Timeout             time.Duration
	Retries             int
	NoBackup            bool
	SourceMusicBrainz   bool
}

func DefaultMetadataConfig() MetadataConfig {
	return MetadataConfig{
		Model:               "qwen3-1.7b-instruct-q4_k_m",
		Runtime:             "libllama",
		ModelPath:           "./models/qwen3-1.7b-instruct-q4_k_m.gguf",
		GrammarPath:         "./grammars/metadata-decision.gbnf",
		ContextTokens:       4096,
		MaxOutputTokens:     512,
		Threads:             "auto",
		GPULayers:           "auto",
		MinFullConfidence:   0.90,
		MinFieldConfidence:  0.85,
		MinReviewConfidence: 0.70,
		Timeout:             2 * time.Minute,
		Retries:             2,
		SourceMusicBrainz:   true,
	}
}

func (c MetadataConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.ReviewOnly && c.Write {
		return fmt.Errorf("--review cannot be combined with write mode")
	}

	if c.Timeout < 0 {
		return fmt.Errorf("--timeout cannot be negative")
	}

	if c.Retries < 0 {
		return fmt.Errorf("metadata retries cannot be negative")
	}

	for _, threshold := range []struct {
		name  string
		value float64
	}{
		{"metadata full confidence", c.MinFullConfidence},
		{"metadata field confidence", c.MinFieldConfidence},
		{"metadata review confidence", c.MinReviewConfidence},
	} {
		if threshold.value < 0 || threshold.value > 1 {
			return fmt.Errorf("%s must be between 0.0 and 1.0", threshold.name)
		}
	}

	if c.MinFullConfidence < c.MinFieldConfidence {
		return fmt.Errorf("metadata full confidence must be greater than or equal to metadata field confidence")
	}
	if c.MinFieldConfidence < c.MinReviewConfidence {
		return fmt.Errorf("metadata field confidence must be greater than or equal to metadata review confidence")
	}

	switch strings.ToLower(strings.TrimSpace(c.Runtime)) {
	case "", "libllama":
	case "disabled", "none":
	default:
		return fmt.Errorf("invalid metadata runtime %q", c.Runtime)
	}

	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("missing metadata model configuration")
	}
	if strings.TrimSpace(c.ModelPath) == "" && strings.ToLower(strings.TrimSpace(c.Runtime)) != "disabled" {
		return fmt.Errorf("missing metadata model path configuration")
	}
	if strings.TrimSpace(c.GrammarPath) == "" && strings.ToLower(strings.TrimSpace(c.Runtime)) != "disabled" {
		return fmt.Errorf("missing metadata grammar path configuration")
	}
	if c.ContextTokens <= 0 {
		return fmt.Errorf("metadata context tokens must be positive")
	}
	if c.MaxOutputTokens <= 0 {
		return fmt.Errorf("metadata max output tokens must be positive")
	}

	return nil
}
