package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config contiene toda la configuración del agente GOLEM.
// Se carga una sola vez al arrancar y se comparte entre subcomandos.
type Config struct {
	LLMProvider      string // "anthropic" o "ollama"
	AnthropicAPIKey  string // requerida para usar Claude
	AnthropicModel   string // modelo a usar, ej: "claude-haiku-4-5"
	MaxRetries       int    // cuántos intentos hace el agente antes de rendirse
	ExecutionTimeout int    // segundos máximos para ejecutar código generado
}

// Load carga la configuración desde variables de entorno.
// Intenta cargar un archivo .env si existe — si no existe, no es error.
// Retorna error solo si faltan variables requeridas.
func Load() (*Config, error) {
	// godotenv.Load() falla si no hay .env — eso es normal en producción
	// donde las variables vienen del sistema operativo directamente
	_ = godotenv.Load()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf(
			"ANTHROPIC_API_KEY no encontrada\n" +
				"   💡 Crea un archivo .env con: ANTHROPIC_API_KEY=tu_key_aqui\n" +
				"   📄 Puedes copiar .env.example como punto de partida",
		)
	}

	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	maxRetries := 10 // security needs 3 steps + 1 report + room for retries
	if val := os.Getenv("MAX_RETRIES"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			maxRetries = n
		}
		// Si el valor no es número válido, usamos el default silenciosamente
	}

	timeout := 30
	if val := os.Getenv("EXECUTION_TIMEOUT"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			timeout = n
		}
	}

	return &Config{
		LLMProvider:      os.Getenv("LLM_PROVIDER"),
		AnthropicAPIKey:  apiKey,
		AnthropicModel:   model,
		MaxRetries:       maxRetries,
		ExecutionTimeout: timeout,
	}, nil
}
