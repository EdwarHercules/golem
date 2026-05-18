package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/EdwarHercules/golem/config"
	"github.com/EdwarHercules/golem/internal/agent"
	"github.com/EdwarHercules/golem/internal/executor"
	"github.com/EdwarHercules/golem/internal/llm"
	"github.com/EdwarHercules/golem/internal/ui"
	"github.com/EdwarHercules/golem/web"
)

// main es el punto de entrada. Su única responsabilidad:
// llamar a run() y manejar si algo salió mal.
// Nunca tiene lógica de negocio — solo decide el exit code.
func main() {
	if err := run(); err != nil {
		// Aquí SÍ está bien terminar — es main, es el tope de la cadena
		// Pero mostramos el error de forma amigable, no como log.Fatal
		fmt.Fprintf(os.Stderr, "\n❌ Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "\nEscribe 'golem help' para ver los comandos disponibles.")
		os.Exit(1)
	}
}

// run contiene toda la lógica de arranque.
// Retorna error en lugar de llamar log.Fatal — así main() controla el exit.
func run() error {
	if len(os.Args) == 1 {
		return runREPL()
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuración inválida: %w\n💡 Asegúrate de tener ANTHROPIC_API_KEY en tu archivo .env", err)
	}

	llmClient, err := llm.NewAnthropicClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
	if err != nil {
		return fmt.Errorf("no se pudo conectar al LLM: %w", err)
	}

	ex := executor.New()

	switch os.Args[1] {
	case "analyze":
		return runAnalyze(os.Args[2:], llmClient, ex, cfg)
	case "security":
		return runSecurity(os.Args[2:], llmClient, ex, cfg)
	case "serve":
		// llmClient and ex are already validated above, so API-key errors
		// surface before the server binds its port — fail fast, not at
		// the first HTTP request.
		_ = llmClient
		_ = ex
		return runServe(os.Args[2:], cfg)
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("subcomando desconocido: %q", os.Args[1])
	}
}

// runREPL inicia el modo interactivo. cfg, llmClient y ex se crean una sola vez
// y se reutilizan en cada comando del loop.
func runREPL() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuración inválida: %w\n💡 Asegúrate de tener ANTHROPIC_API_KEY en tu archivo .env", err)
	}

	llmClient, err := llm.NewAnthropicClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
	if err != nil {
		return fmt.Errorf("no se pudo conectar al LLM: %w", err)
	}

	ex := executor.New()

	fmt.Println("🗿 GOLEM v1.0 — Agente CodeAct")
	fmt.Println("Escribe 'help' para ver comandos")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("golem> ")
		if !scanner.Scan() {
			// EOF (Ctrl+Z en Windows) — salida limpia
			if err := scanner.Err(); err != nil {
				fmt.Println()
				return fmt.Errorf("error leyendo stdin: %w", err)
			}
			fmt.Println("\n👋 Hasta luego")
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		tokens := strings.Fields(line)
		switch tokens[0] {
		case "analyze":
			if err := runAnalyze(tokens[1:], llmClient, ex, cfg); err != nil {
				fmt.Printf("❌ Error: %v\n", err)
			}
		case "security":
			if err := runSecurity(tokens[1:], llmClient, ex, cfg); err != nil {
				fmt.Printf("❌ Error: %v\n", err)
			}
		case "help":
			printUsage()
		case "exit", "quit":
			fmt.Println("👋 Hasta luego")
			return nil
		default:
			fmt.Printf("❌ Comando desconocido: %q — escribe 'help' para ver los disponibles\n", tokens[0])
		}
	}
}

// runAnalyze ahora retorna error en lugar de llamar os.Exit directamente.
func runAnalyze(args []string, llmClient llm.LLMClient, ex executor.Executor, cfg *config.Config) error {
	cmd := flag.NewFlagSet("analyze", flag.ContinueOnError) // ContinueOnError → no llama os.Exit
	fileFlag    := cmd.String("file", "", "ruta al archivo Go a analizar")
	verboseFlag := cmd.Bool("verbose", false, "mostrar código generado y detalles de ejecución")
	providerFlag := cmd.String("provider", "", "sobreescribir proveedor LLM (anthropic/ollama)")

	if err := cmd.Parse(args); err != nil {
		return fmt.Errorf("flags inválidos en analyze: %w", err)
	}

	filePath, err := validateFile(fileFlag, "analyze")
	if err != nil {
		return err // el error ya tiene mensaje descriptivo
	}

	if *providerFlag != "" {
		cfg.LLMProvider = *providerFlag
		fmt.Printf("ℹ  Proveedor: %s\n", *providerFlag)
	}

	ag := agent.NewAgent(llmClient, ex, cfg, agent.AgentOptions{
		SystemPrompt:      agent.SystemPrompt,
		MaxSteps:          3,
		TerminationSignal: "REPORTE GOLEM",
		Verbose:           *verboseFlag,
	})

	var spinner *ui.Spinner
	if !*verboseFlag {
		spinner = ui.NewSpinner("Analizando código con Claude...")
		spinner.Start()
	} else {
		fmt.Println("🔍 Modo verbose — mostrando todos los pasos")
	}

	task := buildAnalyzeTask(filePath)

	if spinner != nil {
		spinner.Stop("")
		spinner = nil
	}
	result, err := runAgent(ag, task)
	if err != nil {
		return fmt.Errorf("análisis falló: %w", err)
	}
	fmt.Println(ui.Green + "✓ Análisis completado" + ui.Reset)

	printResult(result)
	return nil
}

// runSecurity ahora retorna error.
func runSecurity(args []string, llmClient llm.LLMClient, ex executor.Executor, cfg *config.Config) error {
	cmd := flag.NewFlagSet("security", flag.ContinueOnError)
	fileFlag     := cmd.String("file", "", "ruta al archivo Go a analizar")
	fixFlag      := cmd.Bool("fix", false, "aplicar fixes automáticos")
	verboseFlag  := cmd.Bool("verbose", false, "mostrar código generado y detalles de ejecución")
	providerFlag := cmd.String("provider", "", "sobreescribir proveedor LLM (anthropic/ollama)")

	if err := cmd.Parse(args); err != nil {
		return fmt.Errorf("flags inválidos en security: %w", err)
	}

	filePath, err := validateFile(fileFlag, "security")
	if err != nil {
		return err
	}

	if *providerFlag != "" {
		cfg.LLMProvider = *providerFlag
		fmt.Printf("ℹ  Proveedor: %s\n", *providerFlag)
	}

	ag := agent.NewAgent(llmClient, ex, cfg, agent.AgentOptions{
		SystemPrompt:      agent.SecurityPrompt,
		MaxSteps:          3,
		TerminationSignal: "REPORTE GOLEM",
		Verbose:           *verboseFlag,
	})

	var spinner *ui.Spinner
	if !*verboseFlag {
		spinner = ui.NewSpinner("Analizando seguridad con Claude...")
		spinner.Start()
	} else {
		fmt.Println("🔒 Modo verbose — mostrando todos los pasos")
	}

	task := buildSecurityTask(filePath)

	if spinner != nil {
		spinner.Stop("")
		spinner = nil
	}
	result, err := runAgent(ag, task)
	if err != nil {
		return fmt.Errorf("análisis de seguridad falló: %w", err)
	}
	fmt.Println(ui.Green + "✓ Análisis de seguridad completado" + ui.Reset)

	printResult(result)

	if !*fixFlag || len(result.Findings) == 0 {
		return nil
	}

	return runFix(result.Findings, filePath, llmClient, ex, cfg)
}

// runFix ahora retorna error.
func runFix(findings []agent.Finding, filePath string,
	llmClient llm.LLMClient, ex executor.Executor, cfg *config.Config) error {

	original, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("no se pudo leer %s: %w", filePath, err)
	}

	findingsJSON, _ := json.Marshal(findings)

	fixAgent := agent.NewAgent(llmClient, ex, cfg, agent.AgentOptions{
		SystemPrompt:      agent.FixPrompt,
		MaxSteps:          len(findings),
		TerminationSignal: "---FIX_END---",
	})

	task := fmt.Sprintf(`Tienes el siguiente archivo Go con vulnerabilidades confirmadas:

Archivo: %s

Vulnerabilidades encontradas:
%s

Contenido del archivo:
%s

Para cada vulnerabilidad, genera ÚNICAMENTE el fragmento corregido.
Imprime en este formato exacto por cada fix:
---FIX_START---
ORIGINAL:
<línea original problemática>
FIXED:
<línea o líneas corregidas>
---FIX_END---`,
		filePath, string(findingsJSON), string(original))

	result, err := fixAgent.Run(context.Background(), task)
	if err != nil {
		return fmt.Errorf("error generando fixes: %w", err)
	}

	patches := parsePatches(result.Report)
	if len(patches) == 0 {
		fmt.Println("⚠  No se generaron fixes aplicables")
		return nil // no es error fatal — simplemente no había nada que parchear
	}

	fmt.Println("\n--- Fixes propuestos ---")
	for _, p := range patches {
		fmt.Printf("\n- %s\n+ %s\n", p.Original, p.Fixed)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\n¿Aplicar estos cambios? [s/N]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input != "s" {
		fmt.Println("Cambios no aplicados.")
		return nil
	}

	fixed := string(original)
	for _, p := range patches {
		fixed = strings.Replace(fixed, p.Original, p.Fixed, 1)
	}

	verifyFix(fixed, filePath) // advertencia, no bloqueador

	if err := os.WriteFile(filePath, []byte(fixed), 0644); err != nil {
		return fmt.Errorf("error guardando fixes: %w", err)
	}

	fmt.Printf("✅ Fixes aplicados en: %s\n", filePath)
	fmt.Println("⚠  Revisa los fixes de SQL Injection — pueden requerir ajuste de imports.")
	return nil
}

// validateFile ahora retorna (string, error) en lugar de llamar os.Exit.
// Quien la llama decide qué hacer con el error.
func validateFile(fileFlag *string, subcommand string) (string, error) {
	if *fileFlag == "" {
		return "", fmt.Errorf("--file es requerido\n   Uso: golem %s --file <archivo.go>", subcommand)
	}
	if _, err := os.Stat(*fileFlag); os.IsNotExist(err) {
		return "", fmt.Errorf("archivo no encontrado: %s", *fileFlag)
	}
	return *fileFlag, nil
}

// runAgent ejecuta el agente y retorna el resultado o un error.
// Ya no llama os.Exit — deja que el caller decida.
func runAgent(ag *agent.Agent, task string) (*agent.AgentResult, error) {
	result, err := ag.Run(context.Background(), task)
	if err != nil {
		return nil, err // propagamos el error limpiamente
	}
	return result, nil
}

// printResult muestra el resultado final de forma consistente.
func printResult(result *agent.AgentResult) {
	fmt.Println("\n" + strings.Repeat("═", 40))
	fmt.Println("RESULTADO FINAL:")
	fmt.Println(strings.Repeat("═", 40))
	fmt.Println(result.Report)
}

// runServe starts the GOLEM web server, prints the listening address, and
// blocks until SIGINT (Ctrl+C) or SIGTERM arrives. The server is launched in
// a goroutine so this goroutine can own the signal-wait loop cleanly.
func runServe(args []string, cfg *config.Config) error {
	cmd := flag.NewFlagSet("serve", flag.ContinueOnError)
	portFlag := cmd.String("port", "8080", "puerto del servidor web")

	if err := cmd.Parse(args); err != nil {
		return fmt.Errorf("flags inválidos en serve: %w", err)
	}

	srv, err := web.NewServer(cfg, *portFlag)
	if err != nil {
		return fmt.Errorf("no se pudo iniciar el servidor: %w", err)
	}

	fmt.Printf("GOLEM web server running on http://localhost:%s\n", *portFlag)

	// serverErr carries any fatal error from ListenAndServe so we can
	// distinguish a clean shutdown (signal) from an unexpected failure.
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Start()
	}()

	// quit receives OS signals; buffer of 1 ensures the sender never blocks
	// if this goroutine is momentarily busy handling an error branch.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("servidor terminó inesperadamente: %w", err)
	case sig := <-quit:
		fmt.Printf("\nSeñal %v recibida — apagando servidor...\n", sig)
		return nil
	}
}

func printUsage() {
	fmt.Println("🗿 GOLEM — Agente CodeAct en Go")
	fmt.Println()
	fmt.Println("Uso:")
	fmt.Println("  golem analyze  --file <archivo.go>          Analiza calidad del código")
	fmt.Println("  golem security --file <archivo.go>          Detecta vulnerabilidades")
	fmt.Println("  golem security --file <archivo.go> --fix    Detecta y aplica fixes")
	fmt.Println("  golem serve    --port 8080                  Inicia el servidor web")
	fmt.Println()
	fmt.Println("Flags globales:")
	fmt.Println("  --verbose      Muestra código generado y pasos del agente")
	fmt.Println("  --provider     Sobreescribe proveedor LLM (anthropic/ollama)")
}

// buildAnalyzeTask y buildSecurityTask extraen los strings largos de task
// fuera de las funciones principales para mejorar legibilidad.
func buildAnalyzeTask(filePath string) string {
	return fmt.Sprintf(`Analiza el archivo Go en la ruta: %s

IMPORTANTE — reglas para el código que generes:
- El path del archivo es exactamente: %s
- Hardcodea ese path en el programa — NO uses os.Args ni flags
- Genera programas SIMPLES de máximo 60 líneas cada uno
- Cada programa hace UNA sola cosa

Haz el análisis en este orden, un programa a la vez:

PASO 1: Verifica compilación
Genera un programa que ejecute: exec.Command("go", "build", "%s")
Imprime si compila o los errores encontrados.

PASO 2: Ejecuta go vet
Genera un programa que ejecute: exec.Command("go", "vet", "%s")
Imprime si hay warnings o "sin problemas".

PASO 3: Mide complejidad
Genera un programa que use go/parser y go/ast para recorrer las funciones
y contar: if, for, range, switch case, &&, ||
Imprime el nombre de cada función y su complejidad.

Cuando termines los 3 pasos, escribe el REPORTE GOLEM en texto.`,
		filePath, filePath, filePath, filePath)
}

func buildSecurityTask(filePath string) string {
	return fmt.Sprintf(`Analiza el archivo Go en busca de vulnerabilidades de seguridad.
Ruta del archivo: %s

IMPORTANTE — reglas para el código que generes:
- El path del archivo es exactamente: %s
- Hardcodea ese path en el programa — NO uses os.Args ni flags
- Genera programas SIMPLES de máximo 60 líneas cada uno
- Cada programa hace UNA sola cosa

Haz el análisis en este orden, un programa a la vez:

PASO 1: Detecta SQL Injection
...
PASO 2: Detecta credenciales hardcodeadas
...
PASO 3: Detecta inputs sin sanitizar
...

OBLIGATORIO AL TERMINAR LOS 3 PASOS:
Primero imprime cada hallazgo como JSON (un objeto por línea).
Luego imprime EXACTAMENTE esta línea sola: ---FINDINGS_END---
Luego escribe el REPORTE GOLEM en texto.`, filePath, filePath)
}

func verifyFix(fixedCode, originalPath string) bool {
	tmpPath := originalPath + ".golem_tmp.go"

	if err := os.WriteFile(tmpPath, []byte(fixedCode), 0644); err != nil {
		fmt.Printf("⚠  No se pudo crear archivo temporal: %v\n", err)
		return false
	}
	defer os.Remove(tmpPath)

	out, err := exec.Command("go", "build", tmpPath).CombinedOutput()
	if err != nil {
		fmt.Printf("⚠  El código corregido tiene errores de compilación:\n%s\n", string(out))
		fmt.Println("⚠  Los fixes de SQL Injection pueden requerir ajustes adicionales.")
		return false
	}

	fmt.Println("✅ Fix verificado — el código corregido compila")
	return true
}

type Patch struct {
	Original string
	Fixed    string
}

func parsePatches(output string) []Patch {
	var patches []Patch
	blocks := strings.Split(output, "---FIX_START---")

	for _, block := range blocks[1:] {
		endIdx := strings.Index(block, "---FIX_END---")
		if endIdx == -1 {
			continue
		}
		block = block[:endIdx]

		origIdx := strings.Index(block, "ORIGINAL:\n")
		fixedIdx := strings.Index(block, "FIXED:\n")
		if origIdx == -1 || fixedIdx == -1 {
			continue
		}

		origStart := origIdx + len("ORIGINAL:\n")
		original := strings.TrimSpace(block[origStart:fixedIdx])

		fixedStart := fixedIdx + len("FIXED:\n")
		fixed := strings.TrimSpace(block[fixedStart:])

		if original != "" && fixed != "" {
			patches = append(patches, Patch{Original: original, Fixed: fixed})
		}
	}
	return patches
}