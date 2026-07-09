-- +goose Up
CREATE TABLE transactions (
    transaction_id SERIAL PRIMARY KEY,
    customer_id INT NOT NULL,
    transaction_date TIMESTAMP NOT NULL
);


-- +goose Down
DROP TABLE transactions;