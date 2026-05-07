package executor

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecute_ValidCode(t *testing.T) {
	exec := New()
	ctx := context.Background()

	// Usamos \n y \" para evitar problemas con el editor
	code := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"GOLEM_TEST_OK\")\n}"

	result, err := exec.Execute(ctx, code)

	if err != nil {
		t.Fatalf("no esperaba error del executor, obtuve: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("esperaba ExitCode 0, obtuve %d - Stderr: %s", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "GOLEM_TEST_OK") {
		t.Errorf("esperaba GOLEM_TEST_OK en stdout, obtuve: %q", result.Stdout)
	}
	if !result.Success() {
		t.Error("Success() deberia retornar true para codigo valido")
	}
	if result.Duration <= 0 {
		t.Error("Duration deberia ser mayor a cero")
	}
}

func TestExecute_InvalidCode(t *testing.T) {
	exec := New()
	ctx := context.Background()

	code := "package main\n\nfunc main() {\n\testo no es Go valido\n}"

	result, err := exec.Execute(ctx, code)

	if err != nil {
		t.Fatalf("executor no deberia retornar error para codigo invalido: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("esperaba ExitCode != 0 para codigo invalido")
	}
	if result.Stderr == "" {
		t.Error("esperaba mensaje de error en stderr")
	}
	if result.Success() {
		t.Error("Success() deberia retornar false para codigo invalido")
	}
}

func TestExecute_Timeout(t *testing.T) {
	exec := New()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	code := "package main\n\nimport \"time\"\n\nfunc main() {\n\ttime.Sleep(30 * time.Second)\n}"

	start := time.Now()
	_, err := exec.Execute(ctx, code)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("esperaba error de timeout, obtuve nil")
	}
	if elapsed > 8*time.Second {
		t.Errorf("timeout deberia activarse cerca de 2s, tardó %v", elapsed)
	}
}
