package web

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/EdwarHercules/golem/config"
	"github.com/EdwarHercules/golem/internal/agent"
	"github.com/EdwarHercules/golem/internal/executor"
	"github.com/EdwarHercules/golem/internal/llm"
)

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientState
	limit   int
	window  time.Duration
}

type clientState struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		clients: make(map[string]*clientState),
		limit:   limit,
		window:  window,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	state, exists := rl.clients[ip]

	if !exists || now.After(state.resetAt) {
		rl.clients[ip] = &clientState{
			count:   1,
			resetAt: now.Add(rl.window),
		}
		return true
	}

	if state.count >= rl.limit {
		return false
	}

	state.count++
	return true
}

// Server bundles all dependencies for HTTP handlers in one place.
// Grouping them here avoids global state and makes testing straightforward —
// swap any field with a mock and exercise the handler directly.
type Server struct {
	port     string
	agent    *agent.Agent
	llm      llm.LLMClient
	executor executor.Executor
	cfg      *config.Config
}

// NewServer constructs a Server by wiring all internal dependencies.
// Failing early (API key missing, etc.) is intentional: better a clear startup
// error than a cryptic failure on the first real request.
//
// cfg is a pointer to match how config.Load() returns it; pass its result directly.
// NewServer wires all dependencies and returns a ready-to-start Server.
// port is exposed as a parameter so callers (e.g. the CLI's --port flag) can
// override the default without reaching into unexported struct fields.
func NewServer(cfg *config.Config, port string) (*Server, error) {
	// AnthropicClient validates the API key inside its constructor, so any
	// misconfiguration surfaces immediately rather than at the first request.
	llmClient, err := llm.NewAnthropicClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
	if err != nil {
		return nil, fmt.Errorf("init llm client: %w", err)
	}

	exec := executor.New()

	// One shared agent instance kept as a fallback reference. Individual
	// handlers create fresh agents per request with route-specific AgentOptions
	// (different system prompts, step limits, etc.).
	ag := agent.NewAgent(llmClient, exec, cfg, agent.AgentOptions{})

	return &Server{
		port:     port,
		agent:    ag,
		llm:      llmClient,
		executor: exec,
		cfg:      cfg,
	}, nil
}

// Start registers all routes, attaches middleware, and blocks serving traffic.
// Returns only when the listener exits (error or OS signal).
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Go 1.22+ method-prefixed patterns handle 405 Method Not Allowed
	// automatically — no manual method checks needed inside each handler.
	mux.HandleFunc("POST /analyze", s.handleAnalyze)
	mux.HandleFunc("POST /security", s.handleSecurity)
	mux.HandleFunc("POST /fix", s.handleFix)
	mux.HandleFunc("GET /health", s.handleHealth)

	// Serve the SPA shell and static assets from web/static/.
	// The path is relative to the working directory, which is the project root
	// when running via `go run ./cmd/golem` or the compiled binary from root.
	// For a self-contained production binary, replace http.Dir with go:embed.
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("GET /", fs)

	limiter := newRateLimiter(10, 60*time.Second)
	// Apply CORS once at the mux level so every existing and future route
	// is covered without per-handler boilerplate.
	handler := rateLimitMiddleware(limiter, corsMiddleware(mux))

	return http.ListenAndServe(":"+s.port, handler)
}

// corsMiddleware adds permissive CORS headers suitable for local development.
// The wildcard origin is intentional; for production, check r.Header.Get("Origin")
// against an explicit allowlist instead of returning "*".
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// OPTIONS is a browser preflight probe. Answer it and stop —
		// passing it downstream would hit the 405 handler for most routes.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func rateLimitMiddleware(limiter *rateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.Header.Get("X-Forwarder-For")
		if ip == "" {
			ip, _, _ = net.SplitHostPort(r.RemoteAddr)
		}

		if !limiter.allow(ip) {
			http.Error(w,
				`{"error": "demasiados requests, intenta en un momento"}`,
				http.StatusTooManyRequests,
			)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- SSE types --------------------------------------------------------------

// agentEvent is the internal message type that bridges the agent goroutine
// and the SSE write loop. Keeping it internal means handlers don't leak agent
// internals into the HTTP layer — only the serialized JSON shape is public.
type agentEvent struct {
	eventType     string             // "progress" | "done" | "error" | "code_generated"
	message       string             // human-readable text for progress/error frames
	result        *agent.AgentResult // non-nil for regular analysis done events
	GeneratedCode string             // non-empty when eventType == "code_generated"
	FixedCode     string             // non-empty for fix done events
	Patches       []webPatch         // non-nil for fix done events
}

// webPatch is the wire representation of a single code fix sent to the browser.
type webPatch struct {
	Original string `json:"original"`
	Fixed    string `json:"fixed"`
}

// --- Handlers ---------------------------------------------------------------

// handleAnalyze streams analysis results back to the client using
// Server-Sent Events (SSE). The agent can take 10-20 s, so we run it in a
// goroutine and forward progress to the browser in real time instead of
// making the client wait for a single JSON response.
func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	// 1. Parse the request body.
	var req struct {
		Code string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		msg := `{"error":"missing or invalid 'code' field"}`
		if err.Error() == "http: request body too large" {
			msg = `{"error":"payload excede el límite de 1MB"}`
		}
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, `{"error":"missing or invalid 'code' field"}`, http.StatusBadRequest)
		return
	}

	// 2. Verify the ResponseWriter supports incremental flushing.
	//    Plain http.ResponseWriter buffers writes until the handler returns;
	//    http.Flusher lets us push bytes to the client between writes.
	//    net/http always provides this, but third-party middleware may not.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	// 3. Set SSE headers BEFORE writing any body.
	//    Once the first byte is written the status code is locked in.
	//
	//    text/event-stream  — tells the browser's EventSource to treat this as
	//                         an SSE stream instead of buffering the whole body.
	//    Cache-Control: no-cache — prevents proxies from caching the stream.
	//    Connection: keep-alive — asks the TCP connection to stay open for the
	//                             full duration of the stream.
	//    X-Accel-Buffering: no  — tells nginx (and similar) not to buffer the
	//                             response; without this, events batch up in the
	//                             proxy and arrive all at once at the end.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// 4. Channel that decouples the agent goroutine from the SSE write loop.
	//    Why goroutine + channel:
	//      - agent.Run() is synchronous and blocks for 10-20 s.
	//      - If we called it directly in the handler, the HTTP response would
	//        be empty until Run() returned — proxies and browsers time out.
	//      - Running it in a goroutine lets this goroutine own the write loop
	//        and push each event to the client the moment it arrives.
	//    Buffer of 4: the goroutine sends at most 2 events (progress + done/error),
	//    so it never stalls waiting for the reader, even if the client disconnects.
	events := make(chan agentEvent, 32)

	go func() {
		// Closing the channel is the signal to the for-select below to stop.
		// It must happen exactly once, so we defer it here.
		defer close(events)

		events <- agentEvent{eventType: "progress", message: "Iniciando análisis..."}

		// Create a fresh agent per request so each call has isolated memory.
		// Reusing s.agent across concurrent requests would mix conversation
		// history from different users/sessions.
		ag := agent.NewAgent(s.llm, s.executor, s.cfg, agent.AgentOptions{
			SystemPrompt: agent.SystemPrompt,
			ProgressCallback: func(evt, msg string) {
				select {
				case events <- agentEvent{eventType: evt, message: msg}:
				default:
				}
			},
			CodeCallback: func(_ int, code string) {
				select {
				case events <- agentEvent{eventType: "code_generated", GeneratedCode: code}:
				default:
				}
			},
		})
		task := buildWebAnalyzeTask(req.Code)

		result, err := ag.Run(r.Context(), task)
		if err != nil {
			events <- agentEvent{eventType: "error", message: err.Error()}
			return
		}

		events <- agentEvent{eventType: "done", result: result}
	}()

	// 5. Forward each event to the client as it arrives.
	//    We select on both the events channel and the request context so that
	//    a client disconnect (context cancellation) exits the loop promptly
	//    instead of waiting for the goroutine to finish.
	//    The goroutine itself will also return early because ag.Run receives
	//    r.Context() and respects cancellation.
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return // channel closed → agent finished
			}
			// 6. Write the SSE frame and flush immediately.
			//    Why flush after every event:
			//      ResponseWriter accumulates writes in a buffer for efficiency.
			//      Flush() forces the buffered bytes to the TCP stack right now,
			//      so the browser receives each event as it happens rather than
			//      in one burst at the end of the stream.
			writeSSEEvent(w, ev)
			flusher.Flush()
		}
	}
}

// writeSSEEvent serializes ev into the SSE wire format and writes it to w.
//
// SSE wire format for a single event:
//
//	data: <json>\n\n
//
// The double newline (\n\n) marks the end of one event frame. A single \n
// would be treated as a continuation line of the same event by EventSource.
// We encode everything as JSON inside the data field so the browser-side
// handler can parse it with JSON.parse(e.data) without bespoke parsing.
func writeSSEEvent(w http.ResponseWriter, ev agentEvent) {
	var payload map[string]any

	switch ev.eventType {
	case "done":
		if ev.FixedCode != "" {
			// Fix handler done — carry patched code and diff back to browser.
			patches := ev.Patches
			if patches == nil {
				patches = []webPatch{}
			}
			payload = map[string]any{
				"type":       "done",
				"fixed_code": ev.FixedCode,
				"patches":    patches,
			}
		} else {
			findings := ev.result.Findings
			if len(findings) == 0 && ev.result.Report != "" {
				findings = parseWebFindings(ev.result.Report)
			}
			if findings == nil {
				findings = []agent.Finding{}
			}
			payload = map[string]any{
				"type":     "done",
				"report":   ev.result.Report,
				"findings": findings,
			}
		}
	case "code_generated":
		payload = map[string]any{
			"type": "code_generated",
			"code": ev.GeneratedCode,
		}
	case "error":
		payload = map[string]any{
			"type":    "error",
			"message": ev.message,
		}
	default:
		payload = map[string]any{
			"type":    ev.eventType,
			"message": ev.message,
		}
	}

	data, _ := json.Marshal(payload) // error impossible for map[string]any
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// handleSecurity streams a security audit back to the client via SSE.
// It follows the same goroutine+channel pattern as handleAnalyze; the only
// differences are the agent task string and the SecurityPrompt system prompt,
// which tunes the agent toward vulnerability detection and severity scoring.
func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		msg := `{"error":"missing or invalid 'code' field"}`
		if err.Error() == "http: request body too large" {
			msg = `{"error":"payload excede el límite de 1MB"}`
		}
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, `{"error":"missing or invalid 'code' field"}`, http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	events := make(chan agentEvent, 32)

	go func() {
		defer close(events)

		events <- agentEvent{eventType: "progress", message: "Iniciando auditoría de seguridad..."}

		// SecurityPrompt directs the agent to focus on vulnerability classes
		// (SQL injection, hardcoded credentials, unsanitized input, etc.) and
		// to emit findings with Critical/High/Medium/Low severity labels.
		// MaxSteps mirrors the three-step analysis defined in buildSecurityTask.
		ag := agent.NewAgent(s.llm, s.executor, s.cfg, agent.AgentOptions{
			SystemPrompt:              agent.SecurityPrompt,
			MaxSteps:                  3,
			TerminationSignal:         "REPORTE GOLEM",
			RequireStructuredFindings: true,
			ProgressCallback: func(evt, msg string) {
				select {
				case events <- agentEvent{eventType: evt, message: msg}:
				default:
				}
			},
			CodeCallback: func(_ int, code string) {
				select {
				case events <- agentEvent{eventType: "code_generated", GeneratedCode: code}:
				default:
				}
			},
		})
		task := buildWebSecurityTask(req.Code)

		result, err := ag.Run(r.Context(), task)
		if err != nil {
			events <- agentEvent{eventType: "error", message: err.Error()}
			return
		}

		events <- agentEvent{eventType: "done", result: result}
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			writeSSEEvent(w, ev)
			flusher.Flush()
		}
	}
}

// handleHealth is a liveness probe for load balancers and monitoring tools.
// The fixed version and agent name let callers confirm they're talking to the
// right service without parsing logs.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": "1.0.0",
		"agent":   "golem",
	})
}

// --- Web task builders ------------------------------------------------------

// buildWebAnalyzeTask constructs the agent task for web-originated analyze
// requests. Unlike buildAnalyzeTask (which works with a file path), the code
// arrives as plain text, so the task embeds it between sentinels and instructs
// the agent to materialize input.go before running any shell commands.
func buildWebAnalyzeTask(code string) string {
	return fmt.Sprintf(`El código a analizar está embebido abajo entre ---CODE_START--- y ---CODE_END---.
Escríbelo en un archivo input.go antes de ejecutar cualquier comando.

IMPORTANTE — reglas para el código que generes:
- Embebe el contenido del código como raw string literal en cada programa
- Antes de go build o go vet escribe: os.WriteFile("input.go", []byte(codeContent), 0644)
- En el PASO 3 usa: parser.ParseFile(fset, "input.go", []byte(codeContent), 0)
  pasando los bytes como segundo argumento para leer desde memoria, no desde disco
- NO hardcodees ningún path absoluto — usa solo "input.go"
- Genera programas SIMPLES de máximo 60 líneas cada uno
- Cada programa hace UNA sola cosa

Haz el análisis en este orden, un programa a la vez:

PASO 1: Verifica compilación
Escribe el código en input.go con os.WriteFile, luego ejecuta go build input.go.
Imprime si compila o los errores encontrados.

PASO 2: Ejecuta go vet
Escribe el código en input.go con os.WriteFile, luego ejecuta go vet input.go.
Imprime si hay warnings o "sin problemas".

PASO 3: Mide complejidad
Usa parser.ParseFile(fset, "input.go", []byte(codeContent), 0) con el código embebido
para recorrer las funciones y contar: if, for, range, switch case, &&, ||
Imprime el nombre de cada función y su complejidad.

Cuando termines los 3 pasos, escribe el REPORTE GOLEM en texto.

---CODE_START---
%s
---CODE_END---`, code)
}

// buildWebSecurityTask constructs the agent task for web-originated security
// requests. Same embedding pattern as buildWebAnalyzeTask but focused on
// vulnerability detection (SQL injection, hardcoded credentials, unsanitized input).
func buildWebSecurityTask(code string) string {
	return fmt.Sprintf(`El código a analizar está embebido abajo entre ---CODE_START--- y ---CODE_END---.
Escríbelo en un archivo input.go antes de ejecutar cualquier comando.

IMPORTANTE — reglas para el código que generes:
- Embebe el contenido del código como raw string literal en cada programa
- Antes de cualquier análisis escribe: os.WriteFile("input.go", []byte(codeContent), 0644)
- En análisis de AST usa: parser.ParseFile(fset, "input.go", []byte(codeContent), 0)
  pasando los bytes como segundo argumento para leer desde memoria, no desde disco
- NO hardcodees ningún path absoluto — usa solo "input.go"
- Genera programas SIMPLES de máximo 60 líneas cada uno
- Cada programa hace UNA sola cosa

Analiza el código en busca de vulnerabilidades de seguridad en este orden:

PASO 1: Detecta SQL Injection
Usa parser.ParseFile con los bytes del código en memoria para buscar llamadas
a funciones de base de datos con concatenación de strings.
Imprime cada hallazgo con línea y descripción.

PASO 2: Detecta credenciales hardcodeadas
Busca en el AST constantes o strings asignados a variables con nombres como:
password, secret, key, token, apiKey, passwd.
Imprime cada hallazgo con línea y descripción.

PASO 3: Detecta inputs sin sanitizar
Busca entradas de usuario (os.Args, http.Request) que se usen directamente
en operaciones sensibles sin validación previa.
Imprime cada hallazgo con línea y descripción.

OBLIGATORIO AL TERMINAR LOS 3 PASOS:
Primero imprime cada hallazgo como JSON (un objeto por línea).
Luego imprime EXACTAMENTE esta línea sola: ---FINDINGS_END---
Luego escribe el REPORTE GOLEM en texto.

---CODE_START---
%s
---CODE_END---`, code)
}

// handleFix streams a code-fix session via SSE.
// It receives the original code + confirmed findings, runs the FixPrompt agent,
// parses the patch blocks from the report, applies them in-memory, and sends the
// patched code + diff back to the browser in a single "done" SSE frame.
func (s *Server) handleFix(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string          `json:"code"`
		Findings []agent.Finding `json:"findings"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		msg := `{"error":"missing or invalid 'code' field"}`
		if err.Error() == "http: request body too large" {
			msg = `{"error":"payload excede el límite de 1MB"}`
		}
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, `{"error":"missing or invalid 'code' field"}`, http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	events := make(chan agentEvent, 4)

	go func() {
		defer close(events)
		events <- agentEvent{eventType: "progress", message: "Generando fixes..."}

		maxSteps := len(req.Findings) + 1
		if maxSteps < 2 {
			maxSteps = 2
		}
		ag := agent.NewAgent(s.llm, s.executor, s.cfg, agent.AgentOptions{
			SystemPrompt:      agent.FixPrompt,
			MaxSteps:          maxSteps,
			TerminationSignal: "---FIX_END---",
		})

		result, err := ag.Run(r.Context(), buildWebFixTask(req.Code, req.Findings))
		if err != nil {
			events <- agentEvent{eventType: "error", message: err.Error()}
			return
		}

		patches := parseWebPatches(result.Report)
		fixed := req.Code
		for _, p := range patches {
			fixed = strings.Replace(fixed, p.Original, p.Fixed, 1)
		}

		events <- agentEvent{
			eventType: "done",
			FixedCode: fixed,
			Patches:   patches,
		}
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			writeSSEEvent(w, ev)
			flusher.Flush()
		}
	}
}

// buildWebFixTask constructs the agent task for web-originated fix requests.
// The code comes embedded (no file path), so the agent reads it from the task
// body rather than from disk.
func buildWebFixTask(code string, findings []agent.Finding) string {
	findingsJSON, _ := json.Marshal(findings)
	return fmt.Sprintf(`Tienes el siguiente código Go con vulnerabilidades confirmadas.
El código está embebido abajo entre ---CODE_START--- y ---CODE_END---.

Vulnerabilidades encontradas:
%s

Para cada vulnerabilidad, genera ÚNICAMENTE el fragmento corregido.
Imprime en este formato exacto por cada fix:
---FIX_START---
ORIGINAL:
<línea original problemática>
FIXED:
<línea o líneas corregidas>
---FIX_END---

---CODE_START---
%s
---CODE_END---`, string(findingsJSON), code)
}

// parseWebFindings scans the agent output for security findings.
// Each line that starts with '{' is attempted as JSON; lines that produce a
// Finding with both Severity and Type non-empty are kept. Scanning stops at
// the first ---FINDINGS_END--- marker.
func parseWebFindings(output string) []agent.Finding {
	var findings []agent.Finding
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "---FINDINGS_END---" {
			break
		}
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var f agent.Finding
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			continue
		}
		if f.Severity != "" && f.Type != "" {
			findings = append(findings, f)
		}
	}
	return findings
}

// parseWebPatches extracts ORIGINAL/FIXED patch blocks from the agent report.
// Each block is delimited by ---FIX_START--- / ---FIX_END--- markers.
func parseWebPatches(output string) []webPatch {
	var patches []webPatch
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

		original := strings.TrimSpace(block[origIdx+len("ORIGINAL:\n") : fixedIdx])
		fixed := strings.TrimSpace(block[fixedIdx+len("FIXED:\n"):])
		if original != "" && fixed != "" {
			patches = append(patches, webPatch{Original: original, Fixed: fixed})
		}
	}
	return patches
}
