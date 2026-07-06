package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/scynscapa/RetailGo/internal/database"
)

type apiConfig struct {
	dbQueries *database.Queries
}

func main() {
	fmt.Println("Starting RetailGo server...")

	godotenv.Load()
	dbURL := os.Getenv("DB_URL")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database: %s", err)
	}

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	cfg := apiConfig{}
	cfg.dbQueries = database.New(db)

	mux.HandleFunc("GET /api/users", cfg.handleGetUsers)

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorReturn struct {
		Error string `json:"error"`
	}
	resBody := errorReturn{
		Error: msg,
	}
	data, err := json.Marshal(resBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func (cfg *apiConfig) handleGetUsers(w http.ResponseWriter, req *http.Request) {
	var users []database.User

	users, err := cfg.dbQueries.GetUsers(req.Context())
	if err != nil {
		log.Printf("Error getting users: %v", err)
		w.WriteHeader(500)
		return
	}

	usersJson := make([]database.User, len(users))
	var userJson database.User

	for i, user := range users {
		userJson.ID = user.ID
		userJson.CreatedAt = user.CreatedAt
		userJson.UpdatedAt = user.UpdatedAt

		usersJson[i] = userJson
	}

	respondWithJSON(w, 200, usersJson)
}
