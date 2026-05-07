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
	stmt, err := db.Prepare("SELECT * FROM users WHERE id=?")
	if err != nil {
		fmt.Println("Error preparing statement:", err)
		return
	}
	defer stmt.Close()
	row := stmt.QueryRow(userID)
	_ = row
	fmt.Println(query)

	stmt, err := db.Prepare("SELECT name FROM products WHERE category=?")
	if err != nil {
		fmt.Println("Error preparing statement:", err)
		return
	}
	defer stmt.Close()
	row := stmt.QueryRow(userID)
	_ = row
	fmt.Println(search)
}

// VULNERABILIDAD 2 — Credenciales hardcodeadas
func connectToServices() {
	password := os.Getenv("PASSWORD")
	apiKey := os.Getenv("API_KEY")
	token := os.Getenv("TOKEN")
	dbSecret := os.Getenv("DB_SECRET")

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
