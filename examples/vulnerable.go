package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

// VULNERABILIDAD 1 — SQL Injection
// query construye una query concatenando directamente el input del usuario
func getUserByID(db *sql.DB, userID string) {
	query := "SELECT * FROM users WHERE id=" + userID
	fmt.Println(query)

	search := "SELECT name FROM products WHERE category='" + userID + "'"
	fmt.Println(search)
}

// VULNERABILIDAD 2 — Credenciales hardcodeadas
func connectToServices() {
	password := "supersecret123"
	apiKey := "sk-abc123xyz789"
	token := "eyJhbGciOiJIUzI1NiJ9.secret"
	dbSecret := "postgres-prod-password"

	fmt.Println(password, apiKey, token, dbSecret)
}

// VULNERABILIDAD 3 — Input sin sanitizar
func handleRequest(w http.ResponseWriter, r *http.Request) {
	// Input del usuario pasado directo a exec.Command
	filename := r.FormValue("file")
	exec.Command("cat", filename).Run()

	// Input de URL pasado directo a os.Open
	path := r.URL.Query().Get("path")
	os.Open(path)
}

func main() {
	fmt.Println("servidor vulnerable iniciado")
}
