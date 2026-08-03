CREATE TABLE IF NOT EXISTS clothes (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    brand VARCHAR(100) NOT NULL,
    category VARCHAR(50) NOT NULL, -- e.g., T-Shirts, Hoodies, Pants, Jackets
    size VARCHAR(20) NOT NULL,     -- e.g., S, M, L, XL, XXL
    color VARCHAR(50),             -- e.g., Black, White, Blue
    price NUMERIC(10, 2) NOT NULL CHECK (price >= 0),
    stock INT NOT NULL DEFAULT 1 CHECK (stock >= 0),
    image_url TEXT,
    description TEXT,
    in_stock BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_clothes_category ON clothes(category);