package llm

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// LLMClient wraps the underlying OpenAI LLM.
type LLMClient struct {
	client *openai.LLM
}

// NewLLMClient creates a new LLMClient given an API key and model name.
func NewLLMClient(apiKey, model string) (*LLMClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}
	l, err := openai.New(openai.WithToken(apiKey), openai.WithModel(model))
	if err != nil {
		return nil, err
	}
	return &LLMClient{client: l}, nil
}

// GeneratePlan sends a prompt and returns the LLM response.
func (l *LLMClient) GeneratePlan(ctx context.Context, prompt []llms.MessageContent, tools []llms.Tool) (*llms.ContentResponse, error) {
	return l.client.GenerateContent(ctx, prompt, llms.WithTools(tools))
}
