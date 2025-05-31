package interfaces

import (
	"context"
	"database/sql"

	"flash-sale-backend/internal/models"
)

// PurchaseResult represents the result of a purchase operation
type PurchaseResult struct {
	Status        string `json:"status"`         // "success", "sold_out", "user_limit_exceeded", "sale_not_active"
	UserPurchases int    `json:"user_purchases"` // How many items user has purchased in this sale
	TotalSold     int    `json:"total_sold"`     // Total items sold in this sale
	ItemID        string `json:"item_id"`        // The item that was purchased
}

// DatabaseInterface defines the contract for database operations
type DatabaseInterface interface {
	// Connection management
	Close() error
	Ping(ctx context.Context) error
	Stats() sql.DBStats

	// Sale operations
	CreateSale(ctx context.Context, sale *models.Sale) error
	GetActiveSale(ctx context.Context) (*models.Sale, error)
	GetSaleByID(ctx context.Context, id int) (*models.Sale, error)
	UpdateSaleItemsSold(ctx context.Context, saleID int, itemsSold int) error
	DeactivateSale(ctx context.Context, saleID int) error

	// Checkout operations
	CreateCheckoutAttempt(ctx context.Context, attempt *models.CheckoutAttempt) error
	GetCheckoutAttemptByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error)
	UpdateCheckoutAttemptPurchased(ctx context.Context, code string) error

	// Checkout operations (compatibility aliases)
	CreateCheckout(ctx context.Context, attempt *models.CheckoutAttempt) error
	GetCheckoutByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error)

	// User purchase tracking
	GetUserSaleCount(ctx context.Context, userID string, saleID int) (*models.UserSaleCount, error)
	IncrementUserSaleCount(ctx context.Context, userID string, saleID int) error
	CreateUserSaleCount(ctx context.Context, userID string, saleID int) error

	// Purchase operations
	CreatePurchase(ctx context.Context, purchase *models.Purchase) error
	UpdateCheckout(ctx context.Context, checkout *models.Checkout) error

	// Transaction support
	BeginTx(ctx context.Context) (TxInterface, error)
	BeginTransaction(ctx context.Context) (TxInterface, error) // Alias for compatibility
}

// TxInterface defines the contract for database transactions
type TxInterface interface {
	// Transaction control
	Commit() error
	Rollback() error

	// Same operations as DatabaseInterface but within transaction context
	CreateCheckoutAttempt(ctx context.Context, attempt *models.CheckoutAttempt) error
	GetCheckoutAttemptByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error)
	UpdateCheckoutAttemptPurchased(ctx context.Context, code string) error
	GetUserSaleCount(ctx context.Context, userID string, saleID int) (*models.UserSaleCount, error)
	IncrementUserSaleCount(ctx context.Context, userID string, saleID int) error
}

// RedisInterface defines the contract for Redis operations
type RedisInterface interface {
	// Connection management
	Close() error
	Ping(ctx context.Context) error

	// Atomic sale operations
	AtomicPurchase(ctx context.Context, saleID int, userID string, maxItems, maxUserItems int) (bool, string, int, int, error)
	GetSoldItems(ctx context.Context, saleID int) (int, error)
	GetUserPurchaseCount(ctx context.Context, userID string, saleID int) (int, error)

	// Sale management
	SetupSale(ctx context.Context, saleID int, itemsAvailable int) error
	GetActiveSaleID(ctx context.Context) (int, error)
	SetActiveSaleID(ctx context.Context, saleID int) error

	// Checkout code management
	CacheCheckoutCode(ctx context.Context, code string, saleID int, userID string, itemID string) error
	GetCheckoutData(ctx context.Context, code string) (saleID int, userID string, itemID string, err error)
	InvalidateCheckoutCode(ctx context.Context, code string) error

	// Checkout code management (compatibility aliases)
	SetCheckoutCode(ctx context.Context, code string, saleID int, userID string, itemID string) error
	GetCheckoutCode(ctx context.Context, code string) (*models.Checkout, error)
	AttemptPurchase(ctx context.Context, saleID int, userID string, itemID string) (*PurchaseResult, error)

	// Performance metrics
	GetConnectionStats() interface{}
} 