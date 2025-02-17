This file encapsulates the LLM (Large Language Model) operations by wrapping the OpenAI client. It abstracts the initialization and usage of the LLM so that other parts of the application (like the server) can easily generate plans based on user instructions.

## Overview

- LLMClient Structure:
  Wraps the underlying OpenAI LLM. It stores a pointer to the OpenAI client.

- NewLLMClient Function:
  Creates and returns a new LLMClient. It requires an API key and a model name. - Validates that the API key is provided. - Initializes the OpenAI client using the openai.New function.
- GeneratePlan Method:
  Accepts a context, a prompt (a slice of llms.MessageContent), and a list of tools. - Calls the underlying client's GenerateContent method. - Returns a pointer to llms.ContentResponse which contains the LLM's response.

## Detailed Explanation

- LLMClient Structure:

```go
type LLMClient struct {
    client *openai.LLM
}
```

This structure is used to encapsulate LLM operations.

- NewLLMClient:

```go
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
```

Checks for a valid API key, initializes the OpenAI client, and returns a new LLMClient.

- GeneratePlan:

```go
func (l *LLMClient) GeneratePlan(ctx context.Context, prompt []llms.MessageContent, tools []llms.Tool) (*llms.ContentResponse, error) {
    return l.client.GenerateContent(ctx, prompt, llms.WithTools(tools))
}
```

Sends the prompt and the list of tools to the OpenAI LLM and returns the generated response.
