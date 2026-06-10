package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OllamaClient struct {
	baseURL string
	model   string
}

func NewOllamaClient(baseURL, model string) (*OllamaClient, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3"
	}
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
	}, nil
}

type BodyRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type BodyResponse struct {
	Message Message `json:"message"`
}

func (c *OllamaClient) Complete(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	// Ollama no tiene campo separado para system prompt — va como primer mensaje
	// con role "system", igual que el formato OpenAI
	allMessages := append([]Message{{Role: "system", Content: systemPrompt}}, messages...)
	body := BodyRequest{
		Model:    c.model,
		Messages: allMessages,
		Stream:   false,
	}

	// Serializar a JSON para el body del POST
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("Error al serializar a Json: %w", err)
	}

	// POST a /api/chat — endpoint compatible con formato OpenAI
	reqBody := bytes.NewBuffer(jsonBody)
	resp, err := http.Post(c.baseURL+"api/chat", "application/json", reqBody)
	if err != nil {
		return "", fmt.Errorf("Error al realizar el post: %w", err)
	}
	defer resp.Body.Close()

	// Leer y deserializar la respuesta
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Error al leer el body: %w", err)
	}
	var response BodyResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return "", fmt.Errorf("Error al parsear respuesta de Ollama: %w", err)
	}

	return response.Message.Content, nil
}
