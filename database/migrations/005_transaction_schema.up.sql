CREATE TABLE IF NOT EXISTS transactions (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255),
    user_name VARCHAR(100),
    price NUMERIC(10, 2),
    quantity INT DEFAULT 1,
    total_amount NUMERIC(10, 2),
    payment_method VARCHAR(50),
    status VARCHAR(50),
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);