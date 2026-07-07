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
	mux.HandleFunc("GET /api/items/{itemUpc}", cfg.handleGetItem)
	mux.HandleFunc("POST /api/items", cfg.handleCreateItem)

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

func (cfg *apiConfig) handleGetItem(w http.ResponseWriter, req *http.Request) {
	upc := req.PathValue("itemUpc")
	if upc == "" {
		// no item found
	}

	// upc64, err := strconv.ParseInt(upcString, 10, 32)
	// if err != nil {
	// 	respondWithError(w, http.StatusInternalServerError, "Error parsing upc")
	// }
	// upc := int32(upc64)

	item, err := cfg.dbQueries.GetItemByUpc(req.Context(), upc)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Falied to get item")
	}

	respondWithJSON(w, 200, item)
}

func (cfg *apiConfig) handleCreateItem(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Upc         string  `json:"upc"`
		ItemName    string  `json:"item_name"`
		ItemDesc    string  `json:"item_desc"`
		ItemRetail  float64 `json:"item_retail"`
		ItemCost    float64 `json:"item_cost"`
		ItemPicture string  `json:"item_picture"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		fmt.Printf("Error decoding parameters: %v\n", err)
		respondWithError(w, http.StatusInternalServerError, "Error decoding parameters")
		return
	}

	var itemDescNull sql.NullString
	if params.ItemDesc == "" {
		itemDescNull = sql.NullString{Valid: false}
	} else {
		itemDescNull = sql.NullString{String: params.ItemDesc, Valid: true}
	}
	var pictureNull sql.NullString
	if params.ItemPicture == "" {
		pictureNull = sql.NullString{Valid: false}
	} else {
		pictureNull = sql.NullString{String: params.ItemPicture, Valid: true}
	}

	itemParams := database.CreateItemParams{
		Upc:         params.Upc,
		ItemName:    params.ItemName,
		ItemDesc:    itemDescNull,
		ItemRetail:  params.ItemRetail,
		ItemCost:    params.ItemCost,
		ItemPicture: pictureNull,
	}

	item, err := cfg.dbQueries.CreateItem(req.Context(), itemParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating item")
		return
	}

	respondWithJSON(w, 201, item)
}
