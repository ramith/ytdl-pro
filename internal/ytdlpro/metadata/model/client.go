package model

import "context"

type Client struct {
	runtime Runtime
	config  Config
}

func NewClient(cfg Config) (*Client, error) {
	runtime, err := NewRuntime(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{runtime: runtime, config: cfg}, nil
}

func (c *Client) GenerateDecisionJSON(ctx context.Context, inputJSON string) (string, error) {
	return c.runtime.GenerateJSON(ctx, BuildDecisionPrompt(inputJSON), GenerateOptions{
		MaxTokens:   c.config.MaxOutputTokens,
		Temperature: c.config.Temperature,
		TopP:        c.config.TopP,
	})
}

func (c *Client) GenerateRepairJSON(ctx context.Context, inputJSON string) (string, error) {
	return c.runtime.GenerateJSON(ctx, BuildRepairPrompt(inputJSON), GenerateOptions{
		MaxTokens:   c.config.MaxOutputTokens,
		Temperature: c.config.Temperature,
		TopP:        c.config.TopP,
	})
}

func (c *Client) Close() error {
	if c == nil || c.runtime == nil {
		return nil
	}
	return c.runtime.Close()
}
