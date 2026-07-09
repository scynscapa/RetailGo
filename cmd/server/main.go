package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/scynscapa/RetailGo/internal/database"
)

type AccessLevel string

const (
	AccessUser            AccessLevel = "USER"
	AccessAssistant       AccessLevel = "ASSISTANT"
	AccessManager         AccessLevel = "MANAGER"
	AccessDistrictManager AccessLevel = "DISTRICT"
	AccessOperations      AccessLevel = "OPS"
	AccessDev             AccessLevel = "DEV"
)

type apiConfig struct {
	dbQueries *database.Queries
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

var jwtKey = []byte("skdhfowieulkjhggfdtytyiiubjbkncl")

func main() {
	fmt.Println("Starting RetailGo server...")

	godotenv.Load()
	dbURL := os.Getenv("DB_URL")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database: %s", err)
	}

	mux := mux.NewRouter()
	// mux.Use(AuthMiddleware)

	publicMux := mux.PathPrefix("/public").Subrouter()
	privateMux := mux.PathPrefix("/api/v1").Subrouter()
	privateMux.Use(AuthMiddleware)

	cfg := apiConfig{}
	cfg.dbQueries = database.New(db)

	privateMux.HandleFunc("/users", cfg.HandleGetUsers).Methods("GET")
	privateMux.HandleFunc("/items/{itemUpc}", cfg.HandleGetItem).Methods("GET")
	privateMux.HandleFunc("/items", cfg.HandleGetItems).Methods("GET")
	privateMux.HandleFunc("/items", cfg.HandleCreateItem).Methods("POST")
	privateMux.HandleFunc("/users", cfg.HandleCreateUser).Methods("POST")

	publicMux.HandleFunc("/login", cfg.LoginUser).Methods("POST")

	http.ListenAndServe(":8080", mux)
}

func (acc AccessLevel) accessValid() bool {
	// check if an AccessLevel is valid for user creation
	switch acc {
	case AccessUser, AccessAssistant, AccessManager, AccessDistrictManager, AccessOperations, AccessDev:
		return true
	}
	return false
}

func RespondWithError(w http.ResponseWriter, code int, msg string, errorReceived error) {
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

	if errorReceived != nil {
		log.Printf("%s: %v", msg, errorReceived)
	}
}

func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
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

func (cfg *apiConfig) HandleGetUsers(w http.ResponseWriter, req *http.Request) {
	var users []database.User

	users, err := cfg.dbQueries.GetUsers(req.Context())

	// hide password from result
	for i := range len(users) {
		users[i].PasswordHash = ""
	}

	if err != nil {
		log.Printf("Error getting users: %v", err)
		w.WriteHeader(500)
		return
	}

	RespondWithJSON(w, 200, users)
}

func (cfg *apiConfig) HandleCreateUser(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		AccessLevel string `json:"access_level"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		Password    string `json:"password"`
		Active      bool   `json:"active"`
		Username    string `json:"user_name"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error decoding user", err)
		return
	}

	// check if access level input is valid
	accLevel := AccessLevel(params.AccessLevel)
	if accLevel.accessValid() != true {
		RespondWithError(w, http.StatusBadRequest, "Invalid Access Level", nil)
		return
	}

	// create hashed password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(params.Password), bcrypt.DefaultCost)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error hashing password", err)
		return
	}

	userParams := database.CreateUserParams{
		AccessLevel:  params.AccessLevel,
		FirstName:    params.FirstName,
		LastName:     params.LastName,
		PasswordHash: string(hashedPassword),
		Active:       params.Active,
		UserName:     params.Username,
	}

	user, err := cfg.dbQueries.CreateUser(req.Context(), userParams)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error creating user", err)
		return
	}

	// hide password hash from creation return
	user.PasswordHash = ""

	RespondWithJSON(w, 201, user)
}

func (cfg *apiConfig) HandleGetItems(w http.ResponseWriter, req *http.Request) {
	var items []database.Item

	items, err := cfg.dbQueries.GetItems(req.Context())
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error getting items", err)
		return
	}

	RespondWithJSON(w, 200, items)
}

func (cfg *apiConfig) HandleGetItem(w http.ResponseWriter, req *http.Request) {
	upc := req.PathValue("itemUpc")
	if upc == "" {
		// no item found
	}

	item, err := cfg.dbQueries.GetItemByUpc(req.Context(), upc)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Falied to get item", err)
	}

	RespondWithJSON(w, 200, item)
}

func (cfg *apiConfig) HandleCreateItem(w http.ResponseWriter, req *http.Request) {
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
		RespondWithError(w, http.StatusInternalServerError, "Error decoding parameters", err)
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
		RespondWithError(w, http.StatusInternalServerError, "Error creating item", err)
		return
	}

	RespondWithJSON(w, 201, item)
}

func (cfg *apiConfig) LoginUser(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error decoding login", err)
		return
	}

	user, err := cfg.dbQueries.GetUserByUserName(r.Context(), creds.Username)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error retrieving user", err)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(creds.Password))
	if err != nil {
		RespondWithError(w, http.StatusUnauthorized, "Invalid password", nil)
		return
	}

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: creds.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Could not create token", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenString,
	})
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			RespondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil {
			if err == jwt.ErrSignatureInvalid {
				RespondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
				return
			}
			RespondWithError(w, http.StatusBadRequest, "Bad Request", nil)
			return
		}
		if !token.Valid {
			RespondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}
