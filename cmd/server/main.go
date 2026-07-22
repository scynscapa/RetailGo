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
	"github.com/google/uuid"
	"github.com/gorilla/handlers"
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

type RefreshClaims struct {
	Username  string `json:"username"`
	TokenType string `json:"token_type"` // Must be "refresh"
	jwt.RegisteredClaims
}

type contextKey string

const userNameKey contextKey = "userName"

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

	privateMux.HandleFunc("/users", cfg.HandleGetUsers).Methods("GET", "OPTIONS")
	privateMux.HandleFunc("/users", cfg.HandleCreateUser).Methods("POST")

	privateMux.HandleFunc("/items/{itemUpc}", cfg.HandleGetItem).Methods("GET")
	privateMux.HandleFunc("/items", cfg.HandleGetItems).Methods("GET", "OPTIONS")
	privateMux.HandleFunc("/items", cfg.HandleCreateItem).Methods("POST")

	privateMux.HandleFunc("/transactions", cfg.handleCreateTrans).Methods("POST")
	privateMux.HandleFunc("/transactions/{transId}", cfg.handleGetTransById).Methods("GET")
	privateMux.HandleFunc("/transactions/{transId}", cfg.handleAddItemTrans).Methods("POST")

	publicMux.HandleFunc("/login", cfg.LoginUser).Methods("POST")
	publicMux.HandleFunc("/refresh", cfg.HandleRefresh).Methods("POST", "OPTIONS")

	// Needed for CORS
	corsOpts := handlers.AllowedOrigins([]string{"http://localhost:5173"})
	credentialsOk := handlers.AllowCredentials()
	methods := handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS"})
	headers := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"})

	http.ListenAndServe(":8080", handlers.CORS(corsOpts, credentialsOk, methods, headers)(mux))

	// http.ListenAndServe(":8080", mux)
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
	user, err := cfg.dbQueries.GetUserByUserName(ctx, ctx.Value(userNameKey).(string))
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

	// check if user is allowed to assign provided AccessLevel
	callingUser, err := cfg.dbQueries.GetUserByUserName(req.Context(), req.Context().Value("userName").(string))
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error creating user", err)
		return
	}
	if callingUser.AccessLevel <= int32(params.AccessLevel) {
		RespondWithError(w, http.StatusUnauthorized, "Invalid AccessLevel", nil)
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

	refreshTokenClaims := &RefreshClaims{
		Username:  user.UserName,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	refreshToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshTokenClaims).SignedString(cfg.jwtKey)

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		MaxAge:   7 * 24 * 3600,
		HttpOnly: true,
		Secure:   false, // requires https for true
		// SameSite: http.SameSiteStrictMode,  // doesn't work for http/local dev
		SameSite: http.SameSiteLaxMode,
		Path:     "/public/refresh",
	})

	w.Header().Set("Content-Type", "application/json")
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
}

func (cfg *apiConfig) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Read refresh token from secure HttpOnly cookie
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		http.Error(w, "Unauthorized: Missing refresh token", http.StatusUnauthorized)
		return
	}
	refreshToken := cookie.Value

	// 2. Validate token
	claims, err := ValidateRefreshToken(refreshToken, []byte(cfg.jwtKey))
	if err != nil {
		RespondWithError(w, http.StatusUnauthorized, "Unauthorized: Invalid token", nil)
		return
	}

	// 3. Generate new short-lived access token
	assessClaims := Claims{
		Username: claims.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute * 15)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	newAccessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, assessClaims).SignedString([]byte(cfg.jwtKey))
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Error creating new access token", err)
		return
	}

	// Optional: Rotate the refresh token by setting a new cookie here if needed

	// 4. Send the new access token back to the frontend
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TokenResponse{AccessToken: newAccessToken})
}

func ValidateRefreshToken(cookieStr string, secretKey []byte) (*RefreshClaims, error) {
	claims := &RefreshClaims{}

	// Layer 1: Verify Signature and Expiration
	token, err := jwt.ParseWithClaims(cookieStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return secretKey, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Layer 2: Explicit Type Check
	if claims.TokenType != "refresh" {
		return nil, fmt.Errorf("invalid token type")
	}

	// Layer 3: Database Blacklist / Revocation Check
	// isRevoked, err := db.IsTokenRevoked(claims.ID) // Uses jti claim
	// if err != nil || isRevoked {
	//     return nil, fmt.Errorf("token has been revoked")
	// }

	return claims, nil
}

func (cfg *apiConfig) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "auth/refresh" {
			next.ServeHTTP(w, r)
			return
		}

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
			RespondWithError(w, http.StatusBadRequest, "Bad Request", err)
			return
		}
		if !token.Valid {
			RespondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
			return
		}

		parent := r.Context()
		ctx := context.WithValue(parent, userNameKey, claims.Username)
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
