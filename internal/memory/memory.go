package memory

import "github.com/EdwarHercules/golem/internal/llm"

// Memory guarda el historial de conversación.
// El LLM es stateless — esta struct es la única memoria real del sistema.
// Guarda []llm.Message para ser agnóstica al proveedor, igual que LLMClient.
type Memory struct {
	messages []llm.Message
}

// New crea una Memory vacía lista para usar.
func New() *Memory {
	return &Memory{
		messages: []llm.Message{},
	}
}

// AddUserMessage agrega un mensaje del usuario al historial.
func (m *Memory) AddUserMessage(content string) {
	m.messages = append(m.messages, llm.Message{
		Role:    "user",
		Content: content,
	})
}

// AddAssistantMessage agrega una respuesta del LLM al historial.
func (m *Memory) AddAssistantMessage(content string) {
	m.messages = append(m.messages, llm.Message{
		Role:    "assistant",
		Content: content,
	})
}

// Messages retorna el historial completo para enviarlo al LLM.
func (m *Memory) Messages() []llm.Message {
	return m.messages
}

// Len retorna cuántos mensajes hay.
func (m *Memory) Len() int {
	return len(m.messages)
}

// Clear borra el historial — nueva conversación.
func (m *Memory) Clear() {
	m.messages = []llm.Message{}
}
