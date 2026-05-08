package agent 
import (
	"context"
	"errors"
	"testing"

	"github.com/EdwarHercules/golem/config"
	"github.com/EdwarHercules/golem/internal/executor"
	"github.com/EdwarHercules/golem/internal/llm"
)

// ── Mocks ────────────────────────────────────────────────────────────────────

type mockLLM struct {
	responses []string
	callCount int
}

func (m *mockLLM) Complete(_ context.Context, _ string, _ []llm.Message) (string, error) {
	if m.callCount >= len(m.responses) {
		return "", errors.New("mockLLM: sin más respuestas configuradas")
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

type mockLLMWithError struct {
	err error
}

func (m *mockLLMWithError) Complete(_ context.Context, _ string, _ []llm.Message) (string, error) {
	return "", m.err
}

type mockExecutor struct {
	results   []executor.ExecutionResult
	callCount int
}

func (m *mockExecutor) Execute(_ context.Context, _ string) (executor.ExecutionResult, error) {
	if m.callCount >= len(m.results) {
		return executor.ExecutionResult{}, nil
	}
	result := m.results[m.callCount]
	m.callCount++
	return result, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func testConfig() *config.Config {
	return &config.Config{
		MaxRetries:       5,
		ExecutionTimeout: 10,
	}
}

func newTestAgent(llmClient llm.LLMClient, exec executor.Executor) *Agent {
	return NewAgent(llmClient, exec, testConfig(), AgentOptions{
		SystemPrompt:      "eres un agente de test",
		MaxSteps:          1,
		TerminationSignal: "REPORTE GOLEM",
		Verbose:           false,
	})
}

// ── Tests del agente ─────────────────────────────────────────────────────────

func TestAgentReturnsReportOnSuccess(t *testing.T) {
	validCode := "```go\npackage main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"ok\") }\n```"
	report := "REPORTE GOLEM\nAnálisis completado exitosamente.\nEl código no presenta problemas de calidad ni errores de compilación."

	llmMock := &mockLLM{
		responses: []string{validCode, report},
	}
	execMock := &mockExecutor{
		results: []executor.ExecutionResult{
			{ExitCode: 0, Stdout: "ok"},
		},
	}

	ag := newTestAgent(llmMock, execMock)
	result, err := ag.Run(context.Background(), "analiza este código")

	if err != nil {
		t.Fatalf("esperaba nil error, obtuve: %v", err)
	}
	if result == nil {
		t.Fatal("esperaba AgentResult, obtuve nil")
	}
	if result.Report != report {
		t.Errorf("reporte incorrecto\nquería:   %q\nobtenido: %q", report, result.Report)
	}
}

func TestAgentRetriesOnExecutionFailure(t *testing.T) {
	badCode  := "```go\npackage main\n\nfunc main() { sintaxis inválida }\n```"
	goodCode := "```go\npackage main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"corregido\") }\n```"
	report := "REPORTE GOLEM\nSe corrigió el código exitosamente.\nEl error de sintaxis fue identificado y resuelto en el segundo intento."

	llmMock := &mockLLM{
		responses: []string{badCode, goodCode, report},
	}
	execMock := &mockExecutor{
		results: []executor.ExecutionResult{
			{ExitCode: 1, Stderr: "syntax error: unexpected token"},
			{ExitCode: 0, Stdout: "corregido"},
		},
	}

	ag := newTestAgent(llmMock, execMock)
	result, err := ag.Run(context.Background(), "analiza este código")

	if err != nil {
		t.Fatalf("esperaba éxito después de reintento, obtuve: %v", err)
	}
	if result.Report != report {
		t.Errorf("reporte incorrecto después de reintento")
	}
	if llmMock.callCount < 2 {
		t.Errorf("esperaba al menos 2 llamadas al LLM, obtuve %d", llmMock.callCount)
	}
}

func TestAgentStopsAtMaxRetries(t *testing.T) {
	badCode := "```go\npackage main\n\nfunc main() { panic(\"siempre falla\") }\n```"

	responses := make([]string, 10)
	for i := range responses {
		responses[i] = badCode
	}
	results := make([]executor.ExecutionResult, 10)
	for i := range results {
		results[i] = executor.ExecutionResult{ExitCode: 1, Stderr: "runtime error: panic"}
	}

	llmMock  := &mockLLM{responses: responses}
	execMock := &mockExecutor{results: results}

	ag := newTestAgent(llmMock, execMock)
	result, err := ag.Run(context.Background(), "analiza este código")

	if err == nil {
		t.Fatal("esperaba error al agotar reintentos, obtuve nil")
	}
	if result != nil {
		t.Errorf("esperaba nil result, obtuve: %+v", result)
	}
	if llmMock.callCount > testConfig().MaxRetries+1 {
		t.Errorf("el agente llamó al LLM %d veces, máximo permitido: %d",
			llmMock.callCount, testConfig().MaxRetries)
	}
}

func TestAgentHandlesLLMError(t *testing.T) {
	apiErr  := errors.New("connection timeout: anthropic API unreachable")
	llmMock := &mockLLMWithError{err: apiErr}
	execMock := &mockExecutor{}

	ag := newTestAgent(llmMock, execMock)
	result, err := ag.Run(context.Background(), "analiza este código")

	if err == nil {
		t.Fatal("esperaba error de LLM, obtuve nil")
	}
	if result != nil {
		t.Errorf("esperaba nil result")
	}
	if !errors.Is(err, apiErr) {
		t.Errorf("el error debería envolver el error original\nobtuve: %v", err)
	}
}

// ── Tests de funciones internas ───────────────────────────────────────────────
// Estos tests solo son posibles porque usamos `package agent` (caja blanca).
// Con `package agent_test` no podríamos llamar extractCode ni parseFindings.

func TestExtractCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bloque go estándar",
			input:    "Aquí el código:\n```go\npackage main\n\nfunc main() {}\n```\nFin.",
			expected: "package main\n\nfunc main() {}",
		},
		{
			name:     "bloque sin lenguaje",
			input:    "```\npackage main\n\nfunc main() {}\n```",
			expected: "package main\n\nfunc main() {}",
		},
		{
			name:     "sin código",
			input:    "No hay código aquí, solo texto.",
			expected: "",
		},
		{
			name:     "prefiere go sobre bloque genérico",
			input:    "```\nalgo\n```\n```go\npackage main\n```",
			expected: "package main",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractCode(tc.input) // ← funciona porque estamos en package agent
			if got != tc.expected {
				t.Errorf("caso %q\nquería:   %q\nobtenido: %q", tc.name, tc.expected, got)
			}
		})
	}
}

func TestParseFindings(t *testing.T) {
	input := `{"line":10,"severity":"CRÍTICO","type":"SQL_INJECTION","description":"concatenación","code_snippet":"query += id"}
{"line":25,"severity":"MEDIO","type":"HARDCODED_CRED","description":"password fijo","code_snippet":"password := \"1234\""}
---FINDINGS_END---
REPORTE GOLEM
Se encontraron 2 vulnerabilidades.`

	findings := parseFindings(input) // ← también accesible en caja blanca

	if len(findings) != 2 {
		t.Fatalf("esperaba 2 findings, obtuve %d", len(findings))
	}
	if findings[0].Type != "SQL_INJECTION" {
		t.Errorf("primer finding debería ser SQL_INJECTION, obtuve %q", findings[0].Type)
	}
	if findings[1].Severity != "MEDIO" {
		t.Errorf("segundo finding debería ser MEDIO, obtuve %q", findings[1].Severity)
	}
}