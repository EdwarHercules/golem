# Makefile — GOLEM
# Requiere: Go 1.26+

# Variables
BINARY_NAME=golem
CMD_DIR=./cmd/golem
EXAMPLE_FILE=examples/sample.go

# Detectar OS para el nombre del binario
ifeq ($(OS),Windows_NT)
    BINARY=$(BINARY_NAME).exe
    RM=del /f /q
else
    BINARY=$(BINARY_NAME)
    RM=rm -f
endif

# ─────────────────────────────────────────
# Targets principales
# ─────────────────────────────────────────

## build: Compilar el binario
.PHONY: build
build:
	go build -o $(BINARY) $(CMD_DIR)
	@echo "✅ Binario compilado: $(BINARY)"

## run: Ejecutar análisis de calidad en sample.go
.PHONY: run
run: build
	$(BINARY) analyze --file $(EXAMPLE_FILE)

## run-security: Ejecutar análisis de seguridad en sample.go
.PHONY: run-security
run-security: build
	$(BINARY) security --file $(EXAMPLE_FILE)

## run-fix: Ejecutar security con auto-fix en sample.go
.PHONY: run-fix
run-fix: build
	$(BINARY) security --file $(EXAMPLE_FILE) --fix

## test: Ejecutar todos los tests
.PHONY: test
test:
	go test ./cmd/... ./internal/... ./config/... -v

## test-coverage: Tests con reporte de cobertura
.PHONY: test-coverage
test-coverage:
	go test ./cmd/... ./internal/... ./config/... -coverprofile=coverage.out
	go tool cover -html=coverage.out

## lint: Verificar código con go vet
.PHONY: lint
lint:
	go vet ./cmd/... ./internal/... ./config/...
	@echo "✅ Sin problemas de lint"

## fmt: Formatear código con gofmt
.PHONY: fmt
fmt:
	gofmt -w .
	@echo "✅ Código formateado"

## tidy: Limpiar dependencias de go.mod
.PHONY: tidy
tidy:
	go mod tidy
	@echo "✅ Dependencias actualizadas"

## clean: Eliminar binario y archivos temporales
.PHONY: clean
clean:
	$(RM) $(BINARY)
	$(RM) coverage.out
	@echo "🧹 Limpieza completada"

## help: Mostrar comandos disponibles
.PHONY: help
help:
	@echo.
	@echo   🗿 GOLEM — Comandos disponibles:
	@echo.
	@echo   make build         Compilar el binario
	@echo   make run           Analizar examples/sample.go
	@echo   make run-security  Analizar seguridad en sample.go
	@echo   make run-fix       Analizar + aplicar fixes
	@echo   make test          Ejecutar todos los tests
	@echo   make lint          Verificar codigo con go vet
	@echo   make fmt           Formatear codigo con gofmt
	@echo   make tidy          Limpiar dependencias
	@echo   make clean         Eliminar binario y temporales
	@echo.

# Target por defecto
.DEFAULT_GOAL := help