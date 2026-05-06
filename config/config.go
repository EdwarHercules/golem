package config

import (
	"fmt"     // Para formatear
	"os"      // Para leer variables de entorno
	"strconv" // para convertir strings a numeros

	"github.com/joho/godotenv" // godotenv para cargar el .env
)

type Config struct {
	LLMProvider      string
	AnthropicAPIKey  string
	AnthropicModel   string
	MaxRetries       int
	ExecutionTimeout int
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY es requerida")
	}
	maxRetries := 3
	if val := os.Getenv("MAX_RETRIES"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			maxRetries = n
		}
	}

	timeout := 30
	if val := os.Getenv("EXECUTION_TIMEOUT"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			timeout = n
		}
	}
	return &Config{
		LLMProvider:      os.Getenv("LLM_PROVIDER"),
		AnthropicAPIKey:  apiKey,
		AnthropicModel:   os.Getenv("ANTHROPIC_MODEL"),
		MaxRetries:       maxRetries,
		ExecutionTimeout: timeout,
	}, nil
}
