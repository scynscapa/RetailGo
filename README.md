# RetailGo
A Retail management system written in Go


## Server

### Installation
Prerequisites:
- Go
- Postgres server with DB_URL in .env file 
    1. Run createdb.sh or manually create database
    2. Add to .env in root of project:
`DB_URL=postgres://localhost:5432/retailgo?sslmode=disable`
- SQLC, sample sqlc.yaml 
    ```version: "2"
    sql:
    - schema: "sql/schema"
        queries: "sql/queries"
        engine: "postgresql"
        gen:
        go:
            out: "internal/database"
    ```
- Goose `go install github.com/pressly/goose/v3/cmd/goose@latest`
- Add a secret key for token creation in .env: `JWT_SECRET_KEY=randomstringofcharactershere`

---

### Usage
- Starting server: `go run ./cmd/server` from root directory
- Access Levels for users

    0. User
    1. Assistant
    2. Manager
    3. District Manager
    4. Operations
    5. Developer

- Endpoints - All endpoints use JSON for input and output
    - POST /public/login - Log in with username and password
    ```
        {
            "username": "johndoe",
            "password": "12345"
        }
    ```

    - GET /api/v1/users - Retrieve list of users
    - POST /api/v1/users - Create a user
    ```
        {
            "access_level": 0,
            "first_name": "John",
            "last_name": "Doe",
            "password": "12345",
            "active": true,
            "user_name": "johndoe"
        }
    ```

    - GET /api/v1/items - Retrieve list of items
    - POST /api/v1/items - Create an item
    ```
        {
            "upc": "891627009022",
            "item_name": "Evol ChickenEnchilada",
            "item_desc": "Evol ChickenEnchilada Bake 9oz",
            "item_retail": 5.49,
            "item_cost": 3.29,
            "item_picture": "path_to_picture"
        }
    ```
    - GET /api/v1/items/*item_UPC*

    - POST /api/v1/transactions - Create a transaction
    ```
        {
            "customer_id": 1
        }
    ```
    - GET /api/v1/transactions/*transaction_ID* - Retrieve information about a transaction
    - POST /api/v1/transactions/*transaction_ID* - Add an item to a transaction
    ```
        {
            "upc": "891627009022",
            "quantity": 2
        }
    ```

---

## Client
Not yet implemented