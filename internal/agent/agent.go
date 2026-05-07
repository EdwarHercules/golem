package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/EdwarHercules/golem/config"
	"github.com/EdwarHercules/golem/internal/executor"
	"github.com/EdwarHercules/golem/internal/llm"
)

type Agent struct {
	llm      llm.LLMClient
	executor executor.Executor
	config   *config.Config
}

func NewAgent(llmClient llm.LLMClient, exec executor.Executor, cfg *config.Config) *Agent {
	return &Agent{
		llm:      llmClient,
		executor: exec,
		config:   cfg,
	}
}

func (a *Agent) Run(ctx context.Context, task string) (string, error) {
	messages := []llm.Message{
		{Role: "user", Content: task},
	}

	for attempt := 1; attempt <= a.config.MaxRetries; attempt++ {
		fmt.Printf("\n[Intento %d/%d] Consultando al LLM...\n", attempt, a.config.MaxRetries)

		// Paso 1 Razonar
		response, err := a.llm.Complete(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("error consultando LLM en intento %d: %w", attempt, err)
		}

		fmt.Printf("[LLM] Respuesta recibida (%d caracteres)\n", len(response))

		// Paso 2 Actuar
		code := extractCode(response)
		fmt.Printf("[Executor] Ejecutando código (%d caracteres)...\n", len(code))

		execCTX, cancel := context.WithTimeout(ctx,
			time.Duration(a.config.ExecutionTimeout)*time.Second)

		result, execErr := a.executor.Execute(execCTX, code)
		cancel()

		// Paso 3 Observar
		// Caso A: error del sistema (no pudimos ejecutar)
		if execErr != nil {
			fmt.Printf("[Error] El executor falló: %v\n", execErr)

			if attempt == a.config.MaxRetries {
				return "", fmt.Errorf("executor falló después de %d intentos: %w",
					a.config.MaxRetries, execErr)
			}

			// Informar al LLM del errordel execuor para que ajuste
			messages = append(messages,
				llm.Message{Role: "assistant", Content: response},
				llm.Message{Role: "user", Content: fmt.Sprintf(
					"El executor tuvo un error técnico: %v\nGenera el código nuevamente.",
					execErr,
				)},
			)
			continue
		}
		// Caso B: el código ejecutó pero falló (exit code != 0)
		if !result.Success() {
			fmt.Printf("[Fallo] ExitCode: %d\nStderr: %s\n", result.ExitCode, result.Stderr)

			if attempt == a.config.MaxRetries {
				return "", fmt.Errorf(
					"código generado falló después de %d intentos.\nÚltimo error:\n%s",
					a.config.MaxRetries, result.Stderr,
				)
			}

			// Agregar al historial: la respuesta del LLM + el error observado
			// En el próximo intento, el LLM verá TODO el contexto y se corregirá
			messages = append(messages,
				llm.Message{Role: "assistant", Content: response},
				llm.Message{Role: "user", Content: fmt.Sprintf(
					"El código que generaste falló con este error:\n\n%s\n\nCorrige el código y genera una nueva versión completa.",
					result.Stderr,
				)},
			)
			continue
		}
		// Caso C: éxito
		fmt.Printf("[Éxito] Ejecutado en %v\n", result.Duration)
		return result.Stdout, nil
	}
	return "", fmt.Errorf("se agotaron los reintentos")
}

func extractCode(response string) string {
	// Intentar con bloque ```go primero (más específico)
	start := strings.Index(response, "```go")
	if start == -1 {
		// Fallback: bloque ``` genérico
		start = strings.Index(response, "```")
		if start == -1 {
			// El LLM no usó markdown — asumir que toda la respuesta es código
			return strings.TrimSpace(response)
		}
		start += 3
	} else {
		start += 5
	}

	// Buscar el cierre del bloque desde donde empezó el código
	end := strings.Index(response[start:], "```")
	if end == -1 {
		return strings.TrimSpace(response[start:])
	}

	return strings.TrimSpace(response[start : start+end])
}
