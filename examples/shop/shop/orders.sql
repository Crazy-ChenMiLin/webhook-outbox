CREATE TABLE orders (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer   text NOT NULL,
    total      bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
