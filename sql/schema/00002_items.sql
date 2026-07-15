-- +goose Up
CREATE TABLE items (
    upc             TEXT UNIQUE PRIMARY KEY,
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL,
    item_name       TEXT NOT NULL,
    item_desc       TEXT,
    item_retail     DOUBLE PRECISION NOT NULL,
    item_cost       DOUBLE PRECISION NOT NULL,
    item_picture    TEXT,
    item_id         SERIAL NOT NULL
);


-- +goose Down
DROP TABLE items;