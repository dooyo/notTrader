package models

import (
	"time"
)

// Sale represents a flash sale session
type Sale struct {
	ID             int       `json:"id"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	ItemsAvailable int       `json:"items_available"`
	ItemsSold      int       `json:"items_sold"`
	Active         bool      `json:"active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CheckoutAttempt represents a user's checkout attempt
type CheckoutAttempt struct {
	ID        int       `json:"id"`
	SaleID    int       `json:"sale_id"`
	UserID    string    `json:"user_id"`
	ItemID    string    `json:"item_id"`
	Code      string    `json:"code"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	Purchased bool      `json:"purchased"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Item represents a purchasable item (generated at runtime)
type Item struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	CreatedAt   time.Time `json:"created_at"`
}

// CheckoutRequest represents the request payload for checkout
type CheckoutRequest struct {
	UserID string `json:"user_id"`
	ItemID string `json:"item_id"`
}

// CheckoutResponse represents the response for successful checkout
type CheckoutResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Error   string `json:"error,omitempty"`
}

// PurchaseRequest represents the request payload for purchase
type PurchaseRequest struct {
	Code string `json:"code"`
}

// PurchaseResponse represents the response for purchase attempt
type PurchaseResponse struct {
	Success bool   `json:"success"`
	ItemID  string `json:"item_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Services  map[string]string `json:"services"`
}

// Checkout represents a simplified checkout (alias for CheckoutAttempt for compatibility)
type Checkout = CheckoutAttempt

// Purchase represents a completed purchase
type Purchase struct {
	ID          int       `json:"id"`
	SaleID      int       `json:"sale_id"`
	UserID      string    `json:"user_id"`
	ItemID      string    `json:"item_id"`
	Code        string    `json:"code"`
	CheckoutID  int       `json:"checkout_id"`
	Price       float64   `json:"price"`
	Status      string    `json:"status"`
	PurchaseAt  time.Time `json:"purchased_at"`
	PurchasedAt time.Time `json:"purchased_at"` // Alias for compatibility
	CreatedAt   time.Time `json:"created_at"`
}

// PurchaseResult represents the result of a purchase operation
type PurchaseResult struct {
	Success    bool   `json:"success"`
	ItemID     string `json:"item_id,omitempty"`
	Error      string `json:"error,omitempty"`
	UserCount  int    `json:"user_count,omitempty"`
	TotalSold  int    `json:"total_sold,omitempty"`
} 