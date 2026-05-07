package llm

import "context"

type Message struct {
	Role    string
	Content string
}

type LLMClient interface {
	Complete(ctx context.Context, systemPrompt string, messages []Message) (string, error)
}
