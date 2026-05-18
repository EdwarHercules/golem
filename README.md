# GOLEM — CodeAct Agent

> A Go-based AI agent that uses the **CodeAct paradigm**: instead of reasoning alone, it writes and executes real Go code to analyze codebases and produce actionable results.

---

## What is CodeAct?

Traditional AI agents choose from a fixed set of predefined tools (function calling). GOLEM uses **CodeAct**: the LLM generates arbitrary Go programs, compiles and runs them, observes the real stdout/stderr output, and self-corrects if they fail — all in an autonomous loop.

```
User Request → LLM generates Go code → Executor compiles & runs → Output fed back to LLM → Final Report
```

This gives the agent extreme flexibility: it can write any analysis program it needs, not just call pre-wired functions.

---

## Features

- **Security Analysis** — detects SQL injection, hardcoded credentials, unsanitized inputs, and more
- **Code Quality Analysis** — cyclomatic complexity via AST parsing, `go vet`, `go build` checks
- **Auto-Fix Engine** — generates corrected code for detected issues and verifies it compiles
- **CLI Interface** — interactive REPL and direct file analysis
- **Web Interface** — browser-based UI with real-time SSE streaming of agent progress
- **ReAct Loop** — autonomous Reason → Act → Observe cycle with configurable retries

---

## Architecture

```
┌─ CLI / Web Layer ──────────────────────────────────────┐
│  cmd/golem/main.go  →  run*()  →  subcommands          │
│  web/server.go      →  HTTP handlers + SSE streaming   │
├─ Core Agent Layer ─────────────────────────────────────┤
│  internal/agent/agent.go  →  ReAct loop (CodeAct)      │
│  internal/agent/prompt.go →  system prompt builder     │
├─ Infrastructure Layer ─────────────────────────────────┤
│  internal/llm/      →  LLMClient interface + Anthropic │
│  internal/executor/ →  compiles & runs generated code  │
│  internal/memory/   →  conversation history for LLM    │
├─ Config Layer ─────────────────────────────────────────┤
│  config/config.go   →  env vars, fail-fast validation  │
└────────────────────────────────────────────────────────┘
```

**Key design decisions:**
- `LLMClient` is an interface — swapping providers requires zero agent changes
- Each web request gets a fresh agent instance — no shared state between users
- The agent doesn't know about HTTP; the server doesn't know about the ReAct loop
- All dependencies injected via constructor: `NewAgent(llmClient, executor, config)`

---

## Project Structure

```
golem/
├── cmd/golem/
│   └── main.go              # Entry point, CLI subcommand routing
├── internal/
│   ├── agent/
│   │   ├── agent.go         # ReAct loop orchestrator
│   │   ├── agent_test.go
│   │   └── prompt.go        # System prompt construction
│   ├── llm/
│   │   ├── client.go        # LLMClient interface
│   │   └── anthropic.go     # Claude API implementation
│   ├── executor/
│   │   ├── executor.go      # Executor interface
│   │   └── goexec.go        # Compiles & runs Go code in tempdir
│   └── memory/
│       └── memory.go        # Conversation history
├── web/
│   ├── server.go            # net/http server + SSE handlers
│   └── static/
│       └── app.js           # Vanilla JS SPA, reads SSE events
├── config/
│   └── config.go            # Env var loading with fail-fast
├── examples/
│   └── sample.go            # Example files for demos
├── .env.example             # Template — copy to .env and fill
├── go.mod
└── README.md
```

---

## Setup

### Prerequisites

- Go 1.21+
- Anthropic API key

### Installation

```bash
git clone https://github.com/EdwarHercules/golem
cd golem
```

Create your `.env` file:

```bash
cp .env.example .env
# Edit .env and add your key:
# ANTHROPIC_API_KEY=sk-ant-...
# ANTHROPIC_MODEL=claude-haiku-4-5-20251001
# LLM_PROVIDER=anthropic
```

---

## Usage

### Web Interface (recommended)

```bash
go run ./cmd/golem web --port 8080
# Open http://localhost:8080
```

Paste any Go code into the editor, choose Security or Quality analysis, and watch the agent work in real time via SSE streaming.

### CLI — Analyze a file

```bash
go run ./cmd/golem analyze --file examples/sample.go
```

### CLI — Security scan with auto-fix

```bash
go run ./cmd/golem security --file examples/sample.go --fix
```

### CLI — Interactive REPL

```bash
go run ./cmd/golem
```

### Run tests

```bash
go test ./...
```

---

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `ANTHROPIC_API_KEY` | ✅ | — | Your Anthropic API key |
| `ANTHROPIC_MODEL` | ❌ | `claude-haiku-4-5-20251001` | Model to use |
| `LLM_PROVIDER` | ❌ | `anthropic` | LLM provider |
| `MAX_RETRIES` | ❌ | `3` | Max ReAct loop iterations |
| `EXECUTION_TIMEOUT` | ❌ | `30` | Code execution timeout (seconds) |

---

## How the ReAct Loop Works

```
1. User sends code for analysis
2. Agent builds a system prompt describing the task
3. LLM generates a Go program to perform the analysis
4. Executor compiles the program in an isolated tempdir
5. If compilation fails → error fed back to LLM → retry
6. If execution fails → stderr fed back to LLM → retry  
7. LLM observes the real output and generates next step or final report
8. Loop continues until terminal report or max retries reached
```

The agent uses SSE (Server-Sent Events) to stream each step to the browser in real time, so users see the agent thinking and correcting itself.

---

## Why These Technology Choices?

| Choice | Reason |
|---|---|
| **Go** | Compiled, fast, strong concurrency primitives, single binary deployment |
| **`net/http` (no framework)** | Demonstrates understanding of HTTP fundamentals; no unnecessary dependencies |
| **SSE over WebSocket** | Data flows only server → client; SSE is simpler, HTTP-native, auto-reconnects |
| **Anthropic Claude** | Best-in-class code generation quality for the analysis tasks |
| **Vanilla JS** | No build step, no bundler, keeps the focus on the Go backend |
| **Interface-based LLM client** | Swap providers (Ollama, OpenAI) by changing one env var |

---

## Security Considerations

- Generated code runs with the same OS permissions as the GOLEM process — no sandbox isolation (known limitation, documented for production hardening)
- API keys are read from environment variables only — never hardcoded
- Request body size is limited to prevent memory exhaustion
- Each request gets an isolated temporary directory for code execution

---

## Design Patterns Applied

- **Command Pattern** — CLI subcommands in `main.go`
- **Strategy Pattern** — `LLMClient` interface allows provider swap
- **Repository Pattern** — `memory.Memory` abstracts conversation state
- **Dependency Injection** — all dependencies passed via constructors
- **Observer Pattern** — SSE streaming from agent events to browser

---

*Built in Go as a technical challenge. The agent uses CodeAct — executable code as the action mechanism.*