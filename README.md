# GOLEM — CodeAct Agent

A Go agent that analyzes code by **writing and executing real Go programs** — not just reasoning about them.

---

## What is CodeAct?

Instead of calling predefined tools, GOLEM generates executable Go programs, compiles and runs them, observes the real output, and self-corrects if they fail.

```
User code → LLM writes analyzer → Executor compiles & runs → Output fed back → Final report
```

---

## What it does

- **Security analysis** — SQL injection, hardcoded credentials, unsanitized inputs
- **Quality analysis** — cyclomatic complexity via AST, `go vet`, `go build`
- **Auto-fix** — generates corrected code and verifies it compiles
- **Web UI** — real-time SSE streaming of agent progress
- **CLI** — direct file analysis and interactive REPL

---

## Setup

**Prerequisites:** Go 1.21+ and an Anthropic API key.

```bash
git clone https://github.com/EdwarHercules/golem
cd golem
cp .env.example .env   # add your ANTHROPIC_API_KEY
```

---

## Usage

**Web (recommended)**
```bash
go run ./cmd/golem web --port 8080
# Open http://localhost:8080
```

**CLI**
```bash
go run ./cmd/golem analyze --file examples/sample.go
go run ./cmd/golem security --file examples/sample.go --fix
```

**Tests**
```bash
go test ./...
```

---

## Key design decisions

- **`net/http` over Gin/Echo** — demonstrates HTTP fundamentals without unnecessary dependencies
- **SSE over WebSocket** — data flows only server → client; simpler and HTTP-native
- **`LLMClient` interface** — swap providers (Anthropic, Ollama) by changing one env var, zero agent changes
- **Fresh agent per request** — no shared state between users; each request gets isolated memory