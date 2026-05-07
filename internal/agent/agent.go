package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/EdwarHercules/golem/config"
	"github.com/EdwarHercules/golem/internal/executor"
	"github.com/EdwarHercules/golem/internal/llm"
	"github.com/EdwarHercules/golem/internal/memory"
)

type Agent struct {
	llm      llm.LLMClient
	executor executor.Executor
	config   *config.Config
	memory   *memory.Memory // NUEVO
}

func NewAgent(llmClient llm.LLMClient, exec executor.Executor, cfg *config.Config) *Agent {
	return &Agent{
		llm:      llmClient,
		executor: exec,
		config:   cfg,
		memory:   memory.New(), // NUEVO
	}
}

func (a *Agent) Run(ctx context.Context, task string) (string, error) {
	// En lugar de crear messages local, usamos Memory
	// Memory construye el historial que el LLM recibe en cada turno
	a.memory.AddUserMessage(task)

	for attempt := 1; attempt <= a.config.MaxRetries; attempt++ {
		fmt.Printf("\n[Intento %d/%d] Consultando al LLM...\n", attempt, a.config.MaxRetries)

		// RAZONAR — pasamos el historial completo de Memory
		response, err := a.llm.Complete(ctx, a.memory.Messages())
		if err != nil {
			return "", fmt.Errorf("error consultando LLM en intento %d: %w", attempt, err)
		}

		fmt.Printf("[LLM] Respuesta recibida (%d caracteres)\n", len(response))

		// Guardar respuesta del LLM — crucial para autocorrección
		a.memory.AddAssistantMessage(response)

		// ACTUAR
		code := extractCode(response)
		fmt.Printf("[Executor] Ejecutando código (%d caracteres)...\n", len(code))

		execCTX, cancel := context.WithTimeout(ctx,
			time.Duration(a.config.ExecutionTimeout)*time.Second)
		result, execErr := a.executor.Execute(execCTX, code)
		cancel()

		// OBSERVAR
		if execErr != nil {
			fmt.Printf("[Error] El executor falló: %v\n", execErr)

			if attempt == a.config.MaxRetries {
				return "", fmt.Errorf("executor falló después de %d intentos: %w",
					a.config.MaxRetries, execErr)
			}

			// Memory ya tiene la respuesta del LLM guardada arriba.
			// Solo agregamos el error para el próximo turno.
			a.memory.AddUserMessage(fmt.Sprintf(
				"El executor tuvo un error técnico: %v\nGenera el código nuevamente.",
				execErr,
			))
			continue
		}

		if !result.Success() {
			fmt.Printf("[Fallo] ExitCode: %d\nStderr: %s\n", result.ExitCode, result.Stderr)

			if attempt == a.config.MaxRetries {
				return "", fmt.Errorf(
					"código generado falló después de %d intentos.\nÚltimo error:\n%s",
					a.config.MaxRetries, result.Stderr,
				)
			}

			a.memory.AddUserMessage(fmt.Sprintf(
				"El código que generaste falló con este error:\n\n%s\n\nCorrige el código y genera una nueva versión completa.",
				result.Stderr,
			))
			continue
		}

		// ÉXITO
		fmt.Printf("[Éxito] Ejecutado en %v\n", result.Duration)
		return result.Stdout, nil
	}

	return "", fmt.Errorf("se agotaron los reintentos")
}

func extractCode(response string) string {
	start := strings.Index(response, "```go")
	if start == -1 {
		start = strings.Index(response, "```")
		if start == -1 {
			return strings.TrimSpace(response)
		}
		start += 3
	} else {
		start += 5
	}

	end := strings.Index(response[start:], "```")
	if end == -1 {
		return strings.TrimSpace(response[start:])
	}

	return strings.TrimSpace(response[start : start+end])
}
