package llm

import (
	"context"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicClient struct {
	client *anthropic.Client
	model  string
}

func NewAnthorpicClien(apiKey, model string) (*AnthropicClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: API key reuerida")
	}
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &AnthropicClient{
		client: &client,
		model:  model,
	}, nil
}

func (c *AnthropicClient) Complete(ctx context.Context, messages []Message) (string, error) {
	sdkMessages := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		if msg.Role == "user" {
			sdkMessages[i] = anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			)
		} else {
			sdkMessages[i] = anthropic.NewAssistantMessage(
				anthropic.NewTextBlock(msg.Content),
			)
		}
	}

	// Llamar a la API
	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 1024,
		Messages:  sdkMessages,
	})
	if err != nil {
		return "", fmt.Errorf("anthropic: error llamando API: %w", err)
	}

	// Extraer el texto de la respuesta
	if len(response.Content) == 0 {
		return "", fmt.Errorf("anthropic: respuesta vacía")
	}

	return response.Content[0].Text, nil
}
