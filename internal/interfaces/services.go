package interfaces

import (
	"context"

	"flash-sale-backend/internal/models"
)

// SaleService defines the contract for sale management operations
type SaleService interface {
	// Sale lifecycle
	CreateHourlySale(ctx context.Context) (*models.Sale, error)
	GetCurrentActiveSale(ctx context.Context) (*models.Sale, error)
	ActivateSale(ctx context.Context, saleID int) error
	DeactivateSale(ctx context.Context, saleID int) error

	// Sale status
	GetSaleStatus(ctx context.Context, saleID int) (*models.Sale, error)
	GetSaleItemsSold(ctx context.Context, saleID int) (int, error)
}

// CheckoutService defines the contract for checkout operations
type CheckoutService interface {
	// Checkout process
	ProcessCheckout(ctx context.Context, userID, itemID string) (string, error)
	ValidateCheckoutRequest(userID, itemID string) error

	// Checkout verification
	GetCheckoutAttempt(ctx context.Context, code string) (*models.CheckoutAttempt, error)
}

// PurchaseService defines the contract for purchase operations
type PurchaseService interface {
	// Purchase process
	ProcessPurchase(ctx context.Context, code string) (bool, string, error)
	ValidatePurchaseRequest(code string) error

	// Purchase verification
	GetUserPurchaseCount(ctx context.Context, userID string, saleID int) (int, error)
	CanUserPurchase(ctx context.Context, userID string, saleID int) (bool, error)
}

// ItemService defines the contract for item management
type ItemService interface {
	// Item generation
	GenerateItems(ctx context.Context, count int) ([]models.Item, error)
	GetItemByID(ctx context.Context, itemID string) (*models.Item, error)
	GetAvailableItems(ctx context.Context) ([]models.Item, error)

	// Item validation
	ValidateItemID(itemID string) error
}

// HealthService defines the contract for health monitoring
type HealthService interface {
	// Health checks
	CheckDatabaseHealth(ctx context.Context) error
	CheckRedisHealth(ctx context.Context) error
	GetOverallHealth(ctx context.Context) (*models.HealthResponse, error)

	// Performance metrics
	GetPerformanceMetrics(ctx context.Context) map[string]interface{}
} 