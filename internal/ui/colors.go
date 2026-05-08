// internal/ui/colors.go
package ui

import "fmt"

// Códigos ANSI — constantes para no hardcodear strings mágicos
const (
    Reset  = "\033[0m"
    Red    = "\033[31m"
    Yellow = "\033[33m"
    Green  = "\033[32m"
    Cyan   = "\033[36m"
    Bold   = "\033[1m"
    Dim    = "\033[2m"
)

// Funciones helper — envuelven texto con color y lo resetean automáticamente
func Critical(text string) string {
    return Red + Bold + text + Reset
}

func Warning(text string) string {
    return Yellow + text + Reset
}

func OK(text string) string {
    return Green + text + Reset
}

func Info(text string) string {
    return Cyan + text + Reset
}

func Muted(text string) string {
    return Dim + text + Reset
}

// PrintCritical, PrintWarning, PrintOK — imprimen directamente
func PrintCritical(format string, args ...any) {
    fmt.Printf(Red+Bold+"🔴 "+format+Reset+"\n", args...)
}

func PrintWarning(format string, args ...any) {
    fmt.Printf(Yellow+"🟡 "+format+Reset+"\n", args...)
}

func PrintOK(format string, args ...any) {
    fmt.Printf(Green+"🟢 "+format+Reset+"\n", args...)
}

func PrintInfo(format string, args ...any) {
    fmt.Printf(Cyan+"ℹ  "+format+Reset+"\n", args...)
}

func PrintHeader(title string) {
    line := "════════════════════════════════════════"
    fmt.Println(Cyan + Bold + line + Reset)
    fmt.Println(Cyan + Bold + "  🗿 " + title + Reset)
    fmt.Println(Cyan + Bold + line + Reset)
}