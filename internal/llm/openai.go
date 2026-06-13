package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/portfolio/mediaflow-agent-go/internal/config"
)

type OpenAICompatibleClient struct {
	provider string
	model    string
	baseURL  string
	apiKey   string
	client   *http.Client
}

func NewOpenAICompatibleClient(cfg config.Config) *OpenAICompatibleClient {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &OpenAICompatibleClient{
		provider: cfg.Provider,
		model:    cfg.Model,
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:   cfg.APIKey,
		client:   &http.Client{Timeout: timeout},
	}
}

func (c *OpenAICompatibleClient) Name() string {
	if c.provider == "" {
		return "openai-compatible"
	}
	return c.provider + ":" + c.model
}

func (c *OpenAICompatibleClient) Complete(ctx context.Context, messages []Message, opts CompleteOptions) (string, Usage, error) {
	model := opts.Model
	if model == "" {
		model = c.model
	}
	if model == "" {
		model = "gpt-4.1-mini"
	}

	reqBody := openAIChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: opts.Temperature,
	}
	if reqBody.Temperature == 0 {
		reqBody.Temperature = 0.3
	}
	if opts.MaxTokens > 0 {
		reqBody.MaxTokens = opts.MaxTokens
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", Usage{}, err
	}

	endpoint := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()

	var parsed openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", Usage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error.Message != "" {
			return "", Usage{}, fmt.Errorf("llm request failed: %s", parsed.Error.Message)
		}
		return "", Usage{}, fmt.Errorf("llm request failed with status %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("llm response has no choices")
	}

	usage := Usage{
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
		TotalTokens:  parsed.Usage.TotalTokens,
	}
	return parsed.Choices[0].Message.Content, usage, nil
}

type openAIChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
