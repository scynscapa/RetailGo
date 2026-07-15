package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/scynscapa/RetailGo/internal/database"
)

// type AccessLevel string

// const (
// 	AccessUser            AccessLevel = "USER"
// 	AccessAssistant       AccessLevel = "ASSISTANT"
// 	AccessManager         AccessLevel = "MANAGER"
// 	AccessDistrictManager AccessLevel = "DISTRICT"
// 	AccessOperations      AccessLevel = "OPS"
// 	AccessDev             AccessLevel = "DEV"
// )

type AccessLevel int

const (
	AccessUser AccessLevel = iota
	AccessAssistant
	AccessManager
	AccessDistrictManager
	AccessOperations
	AccessDev
)

type apiConfig struct {
	dbQueries *database.Queries
	jwtKey    []byte
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func main() {
	fmt.Println("Starting RetailGo server...")

	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	secretKey := os.Getenv("JWT_SECRET_KEY")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database: %s", err)
	}

	jwtKey := []byte(secretKey)

	cfg := apiConfig{}
	cfg.dbQueries = database.New(db)
	cfg.jwtKey = jwtKey

	mux := mux.NewRouter()

	// allow some paths for unauthed use - mostly for logging in
	publicMux := mux.PathPrefix("/public").Subrouter()

	privateMux := mux.PathPrefix("/api/v1").Subrouter()
	privateMux.Use(cfg.AuthMiddleware)

	privateMux.HandleFunc("/users", cfg.HandleGetUsers).Methods("GET")
	privateMux.HandleFunc("/users", cfg.HandleCreateUser).Methods("POST")

	privateMux.HandleFunc("/items/{itemUpc}", cfg.HandleGetItem).Methods("GET")
	privateMux.HandleFunc("/items", cfg.HandleGetItems).Methods("GET")
	privateMux.HandleFunc("/items", cfg.HandleCreateItem).Methods("POST")

	privateMux.HandleFunc("/transactions", cfg.handleCreateTrans).Methods("POST")
	privateMux.HandleFunc("/transactions/{transId}", cfg.handleGetTransById).Methods("GET")
	privateMux.HandleFunc("/transactions/{transId}", cfg.handleAddItemTrans).Methods("POST")

	publicMux.HandleFunc("/login", cfg.LoginUser).Methods("POST")

	http.ListenAndServe(":8080", mux)
}

func (acc AccessLevel) accessLevelValid() bool {
	// check if an AccessLevel is valid for user creation
	switch acc {
	case AccessUser, AccessAssistant, AccessManager, AccessDistrictManager, AccessOperations, AccessDev:
		return true
	}
	return false
}

func (cfg *apiConfig) allowedToAccess(w http.ResponseWriter, ctx context.Context, requiredLevel AccessLevel) bool {
	user, err := cfg.dbQueries.GetUserByUserName(ctx, ctx.Value("userName").(string))
	if err != nil {
		log.Printf("Error in allowedToAccess: %v", err)
		return false
	}

	if AccessLevel(user.AccessLevel) >= requiredLevel {
		return true
	}

	RespondWithError(w, http.StatusUnauthorized, "User AccessLevel invalid", nil)
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
	if !cfg.allowedToAccess(w, req.Context(), AccessAssistant) {
		return
	}

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
	if !cfg.allowedToAccess(w, req.Context(), AccessAssistant) {
		return
	}

	type parameters struct {
		AccessLevel int    `json:"access_level"`
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
	if accLevel.accessLevelValid() != true {
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
		AccessLevel:  int32(params.AccessLevel),
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

	RespondWithJSON(w, http.StatusCreated, user)
}

func (cfg *apiConfig) HandleGetItems(w http.ResponseWriter, req *http.Request) {
	if !cfg.allowedToAccess(w, req.Context(), AccessUser) {
		return
	}

	var items []database.Item

	items, err := cfg.dbQueries.GetItems(req.Context())
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error getting items", err)
		return
	}

	RespondWithJSON(w, 200, items)
}

func (cfg *apiConfig) HandleGetItem(w http.ResponseWriter, req *http.Request) {
	if !cfg.allowedToAccess(w, req.Context(), AccessUser) {
		return
	}

	upc := req.PathValue("itemUpc")
	if upc == "" {
		// no item found
		RespondWithError(w, http.StatusNotFound, "Item not found", nil)
		return
	}

	item, err := cfg.dbQueries.GetItemByUpc(req.Context(), upc)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Falied to get item", err)
	}

	RespondWithJSON(w, 200, item)
}

func (cfg *apiConfig) HandleCreateItem(w http.ResponseWriter, req *http.Request) {
	if !cfg.allowedToAccess(w, req.Context(), AccessAssistant) {
		return
	}

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

	RespondWithJSON(w, http.StatusCreated, item)
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

	if user.Active == false {
		RespondWithError(w, http.StatusUnauthorized, "User not active", nil)
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
	tokenString, err := token.SignedString(cfg.jwtKey)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Could not create token", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenString,
	})
}

func (cfg *apiConfig) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			RespondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return cfg.jwtKey, nil
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

		parent := r.Context()
		ctx := context.WithValue(parent, "userName", claims.Username)
		req := r.WithContext(ctx)
		next.ServeHTTP(w, req)

		// next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handleGetTransById(w http.ResponseWriter, req *http.Request) {
	if !cfg.allowedToAccess(w, req.Context(), AccessAssistant) {
		return
	}

	vars := mux.Vars(req)
	transIdString := vars["transId"]
	transIdInt, err := strconv.Atoi(transIdString)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Error converting transaction ID", nil)
		return
	}
	transId := int32(transIdInt)

	transaction, err := cfg.dbQueries.GetTransById(req.Context(), transId)

	RespondWithJSON(w, 200, transaction)
}

func (cfg *apiConfig) handleCreateTrans(w http.ResponseWriter, req *http.Request) {
	if !cfg.allowedToAccess(w, req.Context(), AccessUser) {
		return
	}

	type parameters struct {
		CustomerID int32 `json:"customer_id"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error decoding transaction", err)
		return
	}

	customerId := params.CustomerID

	trans, err := cfg.dbQueries.CreateTrans(req.Context(), customerId)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error creating transaction", err)
		return
	}

	RespondWithJSON(w, http.StatusCreated, trans)
}

func (cfg *apiConfig) handleAddItemTrans(w http.ResponseWriter, req *http.Request) {
	if !cfg.allowedToAccess(w, req.Context(), AccessUser) {
		return
	}

	vars := mux.Vars(req)
	transIdString := vars["transId"]
	transIdInt, err := strconv.Atoi(transIdString)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Error converting transaction ID", nil)
		return
	}
	transId := int32(transIdInt)

	type parameters struct {
		Upc      string `json:"upc"`
		Quantity int32  `json:"quantity"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}

	err = decoder.Decode(&params)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error decoding transaction add", err)
		return
	}

	itemToAdd, err := cfg.dbQueries.GetItemByUpc(req.Context(), params.Upc)
	totalPrice := itemToAdd.ItemRetail * float64(params.Quantity)

	transParams := database.AddItemTransParams{
		ItemID:        itemToAdd.ItemID,
		TransactionID: transId,
		Quantity:      params.Quantity,
		Price:         totalPrice,
	}

	_, err = cfg.dbQueries.AddItemTrans(req.Context(), transParams)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error adding items", err)
		return
	}

	transaction, err := cfg.dbQueries.GetTransById(req.Context(), transId)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error retrieving transaction", err)
		return
	}

	RespondWithJSON(w, http.StatusCreated, transaction)
}
