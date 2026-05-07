package main

import "fmt"

// processData tiene alta complejidad ciclomática — intencional para demo
func processData(data []int, mode string) string {
	result := ""

	for _, v := range data { // +1
		if v > 100 { // +1
			if mode == "strict" { // +1
				result += "ALTO"
			} else if mode == "normal" { // +1
				result += "medio-alto"
			} else {
				result += "?"
			}
		} else if v > 50 { // +1
			if mode == "strict" { // +1
				result += "MEDIO"
			} else {
				result += "bajo-medio"
			}
		} else if v > 10 { // +1
			result += "BAJO"
		} else if v > 0 { // +1
			result += "MUY_BAJO"
		} else {
			result += "NEGATIVO"
		}
	}

	if result == "" { // +1
		return "sin datos"
	}

	return result
} // Complejidad: 10 — justo en el límite

// unusedVariable declara una variable que nunca usa — intencional
func unusedVariable() {
	mensaje := "esto nunca se imprime" // variable sin usar
	fmt.Println("función ejecutada")
	_ = mensaje // necesario para que compile — pero es un smell
}

func main() {
	datos := []int{5, 25, 75, 150}
	resultado := processData(datos, "strict")
	fmt.Println("Resultado:", resultado)
	unusedVariable()
}
