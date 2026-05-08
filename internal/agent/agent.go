package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/EdwarHercules/golem/config"
	"github.com/EdwarHercules/golem/internal/executor"
	"github.com/EdwarHercules/golem/internal/llm"
	"github.com/EdwarHercules/golem/internal/memory"
)

// AgentOptions configura el comportamiento del agente para cada caso de uso.
// Permite que un mismo Agent sirva para análisis, seguridad y fixes
// sin cambiar el código del agente.
type AgentOptions struct {
	SystemPrompt      string
	MaxSteps          int
	TerminationSignal string
	Verbose           bool
}

// Agent es el orquestador del loop ReAct (Razonar → Actuar → Observar).
// No sabe qué analiza — eso lo define el SystemPrompt.
// No sabe cómo ejecuta — eso lo hace el Executor.
// Solo sabe cómo coordinar el ciclo.
type Agent struct {
	llm               llm.LLMClient
	executor          executor.Executor
	config            *config.Config
	memory            *memory.Memory
	systemPrompt      string
	maxSteps          int
	terminationSignal string
	verbose           bool
}

// NewAgent crea un agente con la configuración y opciones dadas.
// Cada llamada crea una memoria fresca — los agentes no comparten historial.
func NewAgent(llmClient llm.LLMClient, exec executor.Executor, cfg *config.Config, opts AgentOptions) *Agent {
	return &Agent{
		llm:               llmClient,
		executor:          exec,
		config:            cfg,
		memory:            memory.New(),
		systemPrompt:      opts.SystemPrompt,
		maxSteps:          opts.MaxSteps,
		terminationSignal: opts.TerminationSignal,
		verbose:           opts.Verbose,
	}
}

// Run ejecuta el loop ReAct hasta obtener un reporte o agotar los reintentos.
// Retorna AgentResult con el reporte y hallazgos, o un error si falló.
func (a *Agent) Run(ctx context.Context, task string) (*AgentResult, error) {
	a.memory.AddUserMessage(task)
	successCount := 0

	for attempt := 1; attempt <= a.config.MaxRetries; attempt++ {
		if a.verbose {
			fmt.Printf("\n[STEP %d/%d] Consultando al LLM...\n", attempt, a.config.MaxRetries)
		} else if attempt > 1 {
			fmt.Printf("  ↻ Reintento %d/%d...\n", attempt, a.config.MaxRetries)
		}

		// RAZONAR — el LLM recibe el historial completo para poder autocorregirse
		response, err := a.llm.Complete(ctx, a.systemPrompt, a.memory.Messages())
		if err != nil {
			return nil, fmt.Errorf("error consultando LLM en intento %d: %w", attempt, err)
		}

		if a.verbose {
			fmt.Printf("[LLM] Respuesta recibida (%d caracteres)\n", len(response))
			fmt.Println("─────────────────────────────────────")
			fmt.Println(response)
			fmt.Println("─────────────────────────────────────")
		}

		a.memory.AddAssistantMessage(response)

		// Guardia: el LLM quiso terminar antes de completar todos los pasos
		if strings.Contains(response, "---FINDINGS_END---") && successCount < a.maxSteps {
			a.memory.AddUserMessage(fmt.Sprintf(
				"Aún no has ejecutado los %d pasos de análisis. Solo completaste %d. "+
					"NO escribas el reporte todavía. Genera el código Go para el paso %d.",
				a.maxSteps, successCount, successCount+1,
			))
			continue
		}

		// ¿Es el reporte final? — texto sin código, con la señal de terminación
		if a.isTerminalReport(response, successCount) {
			if a.verbose {
				fmt.Println("[Golem] Reporte final detectado ✅")
			}
			return &AgentResult{
				Report:   response,
				Findings: parseFindings(response),
			}, nil
		}

		// ACTUAR — extraer y validar el código generado
		code := extractCode(response)

		if code == "" {
			if a.verbose {
				fmt.Println("[Warning] No se detectó código Go en la respuesta, reintentando...")
			}
			a.memory.AddUserMessage(
				"No detecté código Go en tu respuesta. " +
					"Continúa con el siguiente paso del análisis generando el código correspondiente.",
			)
			continue
		}

		if !strings.HasPrefix(strings.TrimSpace(code), "package") {
			if a.verbose {
				fmt.Println("[Warning] Código extraído no empieza con 'package', reintentando...")
			}
			a.memory.AddUserMessage(
				"El código que generaste no es Go válido. " +
					"Asegúrate de generar SOLO el bloque de código Go, empezando con 'package main'.",
			)
			continue
		}

		if a.verbose {
			fmt.Println("\n[CODE] Código generado:")
			fmt.Println("─────────────────────────────────────")
			fmt.Println(code)
			fmt.Println("─────────────────────────────────────")
			fmt.Printf("[EXEC] Ejecutando (%d chars)...\n", len(code))
		} else {
			fmt.Printf("  → Ejecutando paso %d...\n", successCount+1)
		}

		// ACTUAR — ejecutar con timeout
		execCTX, cancel := context.WithTimeout(ctx,
			time.Duration(a.config.ExecutionTimeout)*time.Second)
		result, execErr := a.executor.Execute(execCTX, code)
		cancel()

		// OBSERVAR — ¿qué pasó?
		if execErr != nil {
			// Error del sistema (no del código generado): go no está en PATH, permisos, etc.
			if a.verbose {
				fmt.Printf("[Error] El executor falló: %v\n", execErr)
			}
			if attempt == a.config.MaxRetries {
				return nil, fmt.Errorf("executor falló después de %d intentos: %w",
					a.config.MaxRetries, execErr)
			}
			a.memory.AddUserMessage(fmt.Sprintf(
				"El executor tuvo un error técnico: %v\nGenera el código nuevamente.", execErr,
			))
			continue
		}

		if !result.Success() {
			// Error del código generado: compilación, panic, etc. — esto es CodeAct trabajando
			if a.verbose {
				fmt.Printf("[Fallo] ExitCode: %d\nStderr: %s\n", result.ExitCode, result.Stderr)
			}
			if attempt == a.config.MaxRetries {
				return nil, fmt.Errorf(
					"código generado falló después de %d intentos.\nÚltimo error:\n%s",
					a.config.MaxRetries, result.Stderr,
				)
			}
			a.memory.AddUserMessage(fmt.Sprintf(
				"El código que generaste falló con este error:\n\n%s\n\n"+
					"Corrige el código y genera una nueva versión completa.",
				result.Stderr,
			))
			continue
		}

		// ÉXITO — el código ejecutó correctamente
		successCount++

		if a.verbose {
			fmt.Printf("[OK] Ejecutado en %v\n", result.Duration)
			fmt.Println("[OUTPUT]:", result.Stdout)
		} else {
			fmt.Printf("  ✓ Paso %d completado (%v)\n", successCount, result.Duration)
		}

		// Dar el output al LLM para que continúe — acumular contexto es clave para CodeAct
		a.memory.AddUserMessage(a.buildNextStepMessage(successCount, result.Stdout))
	}

	return nil, fmt.Errorf("se agotaron los %d reintentos sin completar el análisis",
		a.config.MaxRetries)
}

// isTerminalReport detecta si la respuesta del LLM es el reporte final.
// Extraído a método propio para que sea testeable y más legible.
func (a *Agent) isTerminalReport(response string, successCount int) bool {
	isFindingsReport := strings.Contains(response, "---FINDINGS_END---") &&
		successCount >= a.maxSteps

	isTextReport := strings.Contains(response, a.terminationSignal) &&
		!strings.Contains(response, "```go") &&
		!strings.Contains(response, "package main") &&
		len(strings.TrimSpace(response)) > 50

	return isFindingsReport || isTextReport
}

// buildNextStepMessage construye el mensaje que le dice al LLM qué hacer después
// de cada ejecución exitosa. Diferencia entre "sigue con el paso N" y "escribe el reporte".
func (a *Agent) buildNextStepMessage(successCount int, stdout string) string {
	if successCount >= a.maxSteps {
		return fmt.Sprintf(
			"RESULTADO DE EJECUCIÓN (paso %d):\n%s\n\n"+
				"Ya completaste los %d pasos. Ahora DEBES:\n"+
				"1. Imprimir cada hallazgo como JSON (un objeto por línea)\n"+
				"2. Imprimir exactamente: ---FINDINGS_END---\n"+
				"3. Escribir el REPORTE GOLEM en texto\n"+
				"NO generes más código Go.",
			successCount, stdout, a.maxSteps,
		)
	}
	return fmt.Sprintf(
		"RESULTADO DE EJECUCIÓN (paso %d de %d):\n%s\n\nContinúa con el paso %d del análisis.",
		successCount, a.maxSteps, stdout, successCount+1,
	)
}

// extractCode extrae el primer bloque de código Go de la respuesta del LLM.
// Prioriza bloques marcados como ```go antes de intentar cualquier ```.
func extractCode(response string) string {
	markers := []string{"```go", "```Go", "```golang"}

	for _, marker := range markers {
		start := strings.Index(response, marker)
		if start != -1 {
			start += len(marker)
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
		return ""
	}
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

// parseFindings extrae los hallazgos JSON del output del agente de seguridad.
// Los hallazgos están antes de ---FINDINGS_END--- , uno por línea en formato JSON.
func parseFindings(output string) []Finding {
	var findings []Finding
	parts := strings.SplitN(output, "---FINDINGS_END---", 2)

	for _, line := range strings.Split(parts[0], "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var f Finding
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			continue
		}
		findings = append(findings, f)
	}
	return findings
}