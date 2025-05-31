-- Flash Sale Backend Database Schema
-- Optimized for high-performance concurrent operations

-- Sales table for tracking flash sale sessions
CREATE TABLE sales (
    id SERIAL PRIMARY KEY,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    items_available INTEGER NOT NULL DEFAULT 10000,
    items_sold INTEGER NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT chk_items_available CHECK (items_available > 0),
    CONSTRAINT chk_items_sold CHECK (items_sold >= 0),
    CONSTRAINT chk_items_sold_limit CHECK (items_sold <= items_available),
    CONSTRAINT chk_sale_times CHECK (end_time > start_time)
);

-- Checkout attempts table for tracking all checkout requests
CREATE TABLE checkout_attempts (
    id SERIAL PRIMARY KEY,
    sale_id INTEGER NOT NULL REFERENCES sales(id),
    user_id VARCHAR(50) NOT NULL,
    item_id VARCHAR(50) NOT NULL,
    code VARCHAR(100) NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMP WITH TIME ZONE,
    purchased BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT chk_user_id_format CHECK (LENGTH(user_id) > 0),
    CONSTRAINT chk_item_id_format CHECK (LENGTH(item_id) > 0),
    CONSTRAINT chk_checkout_status CHECK (status IN ('pending', 'used', 'expired'))
);

-- Purchases table for completed transactions
CREATE TABLE purchases (
    id SERIAL PRIMARY KEY,
    sale_id INTEGER NOT NULL REFERENCES sales(id),
    user_id VARCHAR(50) NOT NULL,
    item_id VARCHAR(50) NOT NULL,
    code VARCHAR(100) NOT NULL,
    checkout_id INTEGER REFERENCES checkout_attempts(id),
    price DECIMAL(10,2) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'completed',
    purchased_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT chk_price CHECK (price >= 0),
    CONSTRAINT chk_purchase_status CHECK (status IN ('completed', 'refunded', 'cancelled'))
);

-- Performance indexes
-- Index for finding active sales quickly
CREATE INDEX CONCURRENTLY idx_sales_active_time ON sales(active, start_time DESC) WHERE active = true;

-- Index for checkout code lookups (most frequent operation)
CREATE UNIQUE INDEX CONCURRENTLY idx_checkout_code ON checkout_attempts(code);

-- Index for checkout attempts by sale and user (for analytics)
CREATE INDEX CONCURRENTLY idx_checkout_sale_user ON checkout_attempts(sale_id, user_id);

-- Partial index for unpurchased checkout attempts (for cleanup)
CREATE INDEX CONCURRENTLY idx_checkout_unpurchased ON checkout_attempts(sale_id, created_at) 
    WHERE purchased = false;

-- Index for finding checkout attempts by sale (for admin queries)
CREATE INDEX CONCURRENTLY idx_checkout_sale_created ON checkout_attempts(sale_id, created_at DESC);

-- Indexes for purchases table
CREATE INDEX CONCURRENTLY idx_purchases_sale_user ON purchases(sale_id, user_id);
CREATE INDEX CONCURRENTLY idx_purchases_user_created ON purchases(user_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_purchases_sale_created ON purchases(sale_id, created_at DESC);

-- Trigger function to update updated_at timestamps
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for automatic timestamp updates
CREATE TRIGGER update_sales_updated_at 
    BEFORE UPDATE ON sales
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_checkout_attempts_updated_at 
    BEFORE UPDATE ON checkout_attempts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
