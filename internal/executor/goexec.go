package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

type GoExecutor struct{}

func New() *GoExecutor {
	return &GoExecutor{}
}

func (e *GoExecutor) Execute(ctx context.Context, code string) (ExecutionResult, error) {

	// Crear archivo temporal
	tmpFile, err := os.CreateTemp("", "golem_*.go")
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("crear archivo temporal: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Escribir código y cerrar
	if _, err := tmpFile.WriteString(code); err != nil {
		return ExecutionResult{}, fmt.Errorf("escribir código temporal: %w", err)
	}
	tmpFile.Close()

	// Preparar comando SIN context — lo manejamos manualmente para Windows
	cmd := exec.Command("go", "run", tmpFile.Name())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()

	// Iniciar el proceso
	if err := cmd.Start(); err != nil {
		return ExecutionResult{}, fmt.Errorf("iniciar subproceso: %w", err)
	}

	// Esperar en goroutine separada
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// select: proceso termina vs timeout
	select {
	case err = <-done:
		// Proceso terminó — manejar exit code
		duration := time.Since(start)
		result := ExecutionResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			Duration: duration,
			ExitCode: 0,
		}
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				result.ExitCode = exitErr.ExitCode()
				return result, nil
			}
			return ExecutionResult{}, fmt.Errorf("ejecutar subproceso: %w", err)
		}
		return result, nil

	case <-ctx.Done():
		// Matar proceso según el sistema operativo
		killProcess(cmd)
		<-done
		return ExecutionResult{}, fmt.Errorf("timeout excedido: %w", ctx.Err())
	}
}

func killProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}

	if runtime.GOOS == "windows" {
		// taskkill /F=forzar /T=árbol completo /PID=por ID
		killer := exec.Command("taskkill", "/F", "/T", "/PID",
			fmt.Sprintf("%d", cmd.Process.Pid))
		killer.Run()
	}

	// Siempre intentamos Kill() también — en Linux es suficiente,
	// en Windows es un respaldo por si taskkill falla
	cmd.Process.Kill()
}
