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
	successCount := 0

	for attempt := 1; attempt <= a.config.MaxRetries; attempt++ {
		fmt.Printf("\n[Intento %d/%d] Consultando al LLM...\n", attempt, a.config.MaxRetries)

		// RAZONAR — pasamos el historial completo de Memory
		response, err := a.llm.Complete(ctx, SystemPrompt, a.memory.Messages())

		if err != nil {
			return "", fmt.Errorf("error consultando LLM en intento %d: %w", attempt, err)
		}

		fmt.Printf("[LLM] Respuesta recibida (%d caracteres)\n", len(response))

		// Guardar respuesta del LLM — crucial para autocorrección
		a.memory.AddAssistantMessage(response)

		// ¿El LLM escribió el reporte final? — texto sin código
		// Si contiene REPORTE GOLEM pero no bloques de código, el análisis terminó
		if strings.Contains(response, "REPORTE GOLEM") && !strings.Contains(response, "```") {
			fmt.Println("[Golem] Reporte final detectado ✅")
			return response, nil
		}

		// ACTUAR
		code := extractCode(response)

		// Validar que el código extraído parece Go real
		// Si no empieza con "package", no es código válido — pedirle al LLM que corrija
		if code != "" && !strings.HasPrefix(strings.TrimSpace(code), "package") {
			fmt.Println("[Warning] Código extraído no parece Go válido — solicitando corrección")
			a.memory.AddUserMessage("El código que generaste no es Go válido. Asegúrate de generar SOLO el bloque de código Go, empezando con 'package main'.")
			continue
		}
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

		// ÉXITO — pasar output al LLM para que continúe con el siguiente paso
		// El agente no para aquí — le da el resultado al LLM y sigue el análisis
		fmt.Printf("[Éxito] Ejecutado en %v\n", result.Duration)
		successCount++ // NUEVO

		// Mensaje diferente según cuántos pasos exitosos llevamos
		var nextMsg string
		if successCount >= 3 {
			// Ya completamos los 3 pasos — pedirle el reporte directamente
			nextMsg = fmt.Sprintf(
				"RESULTADO DE EJECUCIÓN (paso %d):\n%s\n\nYa completaste los 3 pasos de análisis. Ahora escribe SOLAMENTE el REPORTE GOLEM final en texto. NO generes más código.",
				successCount, result.Stdout,
			)
		} else {
			// Todavía hay pasos pendientes
			nextMsg = fmt.Sprintf(
				"RESULTADO DE EJECUCIÓN (paso %d de 3):\n%s\n\nContinúa con el paso %d del análisis.",
				successCount, result.Stdout, successCount+1,
			)
		}
		a.memory.AddUserMessage(nextMsg)

	}

	return "", fmt.Errorf("se agotaron los reintentos")
}

func extractCode(response string) string {
	// Buscar inicio de cualquier bloque de código
	// Primero intentamos el más específico: ```go
	markers := []string{"```go", "```Go", "```golang"}

	for _, marker := range markers {
		start := strings.Index(response, marker)
		if start != -1 {
			start += len(marker)
			// Saltar el resto de la primera línea si hay texto después del marker
			if nl := strings.Index(response[start:], "\n"); nl != -1 {
				start += nl + 1
			}
			end := strings.Index(response[start:], "```")
			if end != -1 {
				return strings.TrimSpace(response[start : start+end])
			}
		}
	}

	// Fallback: cualquier bloque ```
	start := strings.Index(response, "```")
	if start == -1 {
		// No hay bloque — asumir que toda la respuesta es código
		return strings.TrimSpace(response)
	}

	// Saltar la línea del marker (puede ser ```bash, ```powershell, etc.)
	start += 3
	if nl := strings.Index(response[start:], "\n"); nl != -1 {
		start += nl + 1
	}

	end := strings.Index(response[start:], "```")
	if end == -1 {
		return strings.TrimSpace(response[start:])
	}

	return strings.TrimSpace(response[start : start+end])
}
