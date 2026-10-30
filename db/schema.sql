CREATE TABLE products (
  sku TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  stock integer NOT NULL CHECK (stock >= 0)
);