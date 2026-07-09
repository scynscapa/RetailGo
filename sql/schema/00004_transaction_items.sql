-- +goose Up
CREATE TABLE transaction_items (
    item_id INT PRIMARY KEY,
    transaction_id INT NOT NULL,
    quantity INT NOT NULL,
    price DOUBLE PRECISION NOT NULL,
    CONSTRAINT fk_transaction_id
        FOREIGN KEY(transaction_id) 
        REFERENCES transactions(transaction_id)
);

-- +goose Down
DROP TABLE transaction_items;