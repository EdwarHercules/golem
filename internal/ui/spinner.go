// internal/ui/spinner.go
package ui

import (
    "fmt"
    "time"
)

// Spinner representa un indicador de progreso animado
type Spinner struct {
    message string       // texto que se muestra junto al spinner
    stop    chan bool     // canal para enviar la señal de parada
    done    chan bool     // canal para confirmar que paró
}

// frames son los caracteres que rotan para dar efecto de animación
var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// New crea un nuevo Spinner con el mensaje dado
func NewSpinner(message string) *Spinner {
    return &Spinner{
        message: message,
        stop:    make(chan bool),
        done:    make(chan bool),
    }
}

// Start lanza el spinner en una goroutine separada
// Retorna inmediatamente — el spinner corre "en paralelo"
func (s *Spinner) Start() {
    go func() {
        i := 0
        for {
            select {
            case <-s.stop:
                // Limpia la línea y confirma que terminó
                fmt.Printf("\r\033[K") // \r = inicio de línea, \033[K = limpiar hasta el final
                s.done <- true
                return
            default:
                frame := frames[i%len(frames)]
                fmt.Printf("\r%s%s %s%s", Cyan, frame, s.message, Reset)
                time.Sleep(80 * time.Millisecond)
                i++
            }
        }
    }()
}

// Stop detiene el spinner y muestra un mensaje final
func (s *Spinner) Stop(finalMessage string) {
    s.stop <- true  // envía señal de parada
    <-s.done        // espera confirmación de que se limpió
    if finalMessage != "" {
        fmt.Println(finalMessage)
    }
}

// Success detiene con mensaje verde
func (s *Spinner) Success(message string) {
    s.Stop(Green + "✓ " + message + Reset)
}

// Fail detiene con mensaje rojo
func (s *Spinner) Fail(message string) {
    s.Stop(Red + "✗ " + message + Reset)
}