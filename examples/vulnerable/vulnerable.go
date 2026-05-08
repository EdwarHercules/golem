package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

// VULNERABILIDAD 1 — SQL Injection
// Concatena input del usuario directamente en la query — nunca hacer esto
func getUserByID(db *sql.DB, userID string) {
	query := "SELECT * FROM users WHERE id=" + userID
	rows, err := db.Query(query)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer rows.Close()

	search := "SELECT name FROM products WHERE category='" + userID + "'"
	rows2, err := db.Query(search)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer rows2.Close()
}

// VULNERABILIDAD 2 — Credenciales hardcodeadas
func connectToServices() {
	password := "supersecret123"
	apiKey := "sk-prod-abc123xyz789"
	token := "eyJhbGciOiJIUzI1NiJ9.hardcoded"
	dbSecret := "postgres://admin:root1234@localhost/prod"

	fmt.Println(password, apiKey, token, dbSecret)
}

// VULNERABILIDAD 3 — Input sin sanitizar
func handleRequest(w http.ResponseWriter, r *http.Request) {
	filename := r.FormValue("file")
	exec.Command("cat", filename).Run()

	path := r.URL.Query().Get("path")
	os.Open(path)
}

func main() {
	fmt.Println("servidor vulnerable iniciado")
}