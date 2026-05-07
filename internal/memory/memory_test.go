package memory

import "testing"

func TestNew_CreatesEmptyMemory(t *testing.T) {
	m := New()
	if m.Len() != 0 {
		t.Errorf("Memory nueva debería tener 0 mensajes, tiene %d", m.Len())
	}
}

func TestAddUserMessage_AddsCorrectRole(t *testing.T) {
	m := New()
	m.AddUserMessage("analiza este código")

	msgs := m.Messages()
	if len(msgs) != 1 {
		t.Fatalf("esperaba 1 mensaje, obtuve %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("esperaba role 'user', obtuve '%s'", msgs[0].Role)
	}
	if msgs[0].Content != "analiza este código" {
		t.Errorf("contenido incorrecto: '%s'", msgs[0].Content)
	}
}

func TestAddAssistantMessage_AddsCorrectRole(t *testing.T) {
	m := New()
	m.AddAssistantMessage("encontré 2 problemas")

	msgs := m.Messages()
	if msgs[0].Role != "assistant" {
		t.Errorf("esperaba role 'assistant', obtuve '%s'", msgs[0].Role)
	}
}

func TestMessages_PreservesOrder(t *testing.T) {
	m := New()
	m.AddUserMessage("primer mensaje")
	m.AddAssistantMessage("primera respuesta")
	m.AddUserMessage("segundo mensaje")

	msgs := m.Messages()
	if len(msgs) != 3 {
		t.Fatalf("esperaba 3 mensajes, obtuve %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" || msgs[2].Role != "user" {
		t.Error("el orden de roles no es correcto")
	}
}

func TestClear_ResetsHistory(t *testing.T) {
	m := New()
	m.AddUserMessage("mensaje")
	m.AddAssistantMessage("respuesta")

	m.Clear()

	if m.Len() != 0 {
		t.Errorf("después de Clear() debería haber 0 mensajes, hay %d", m.Len())
	}
}
