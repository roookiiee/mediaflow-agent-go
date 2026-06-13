package llm

import (
	"context"

	"github.com/portfolio/mediaflow-agent-go/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompleteOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
}

type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens,omitempty"`
}

type Client interface {
	Name() string
	Complete(ctx context.Context, messages []Message, opts CompleteOptions) (string, Usage, error)
}

func NewFromConfig(cfg config.Config) Client {
	if cfg.APIKey == "" {
		return MockClient{Model: "demo-mediaflow"}
	}
	return NewOpenAICompatibleClient(cfg)
}
