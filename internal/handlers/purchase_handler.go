package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"flash-sale-backend/internal/interfaces"
	"flash-sale-backend/internal/models"
)

// PurchaseHandler handles purchase-related HTTP requests
type PurchaseHandler struct {
	saleService interfaces.SaleService
	itemService interfaces.ItemService
	db          interfaces.DatabaseInterface
	redis       interfaces.RedisInterface
}

// NewPurchaseHandler creates a new purchase handler
func NewPurchaseHandler(
	saleService interfaces.SaleService,
	itemService interfaces.ItemService,
	db interfaces.DatabaseInterface,
	redis interfaces.RedisInterface,
) *PurchaseHandler {
	return &PurchaseHandler{
		saleService: saleService,
		itemService: itemService,
		db:          db,
		redis:       redis,
	}
}

// PurchaseRequest represents the purchase request structure
type PurchaseRequest struct {
	CheckoutCode string `json:"checkout_code"`
}

// PurchaseResponse represents the purchase response structure
type PurchaseResponse struct {
	Success       bool           `json:"success"`
	PurchaseID    int           `json:"purchase_id,omitempty"`
	Message       string        `json:"message,omitempty"`
	Item          *models.Item  `json:"item,omitempty"`
	TotalPrice    float64       `json:"total_price,omitempty"`
	PurchasedAt   time.Time     `json:"purchased_at,omitempty"`
	UserPurchases int           `json:"user_purchases,omitempty"` // How many items user has purchased in this sale
	Error         string        `json:"error,omitempty"`
}

// HandlePurchase processes POST /purchase requests
func (ph *PurchaseHandler) HandlePurchase(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		ph.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Parse request
	var req PurchaseRequest
	
	// Check if request has JSON body
	if r.Header.Get("Content-Type") == "application/json" {
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&req); err != nil {
			ph.sendErrorResponse(w, http.StatusBadRequest, "Invalid JSON format")
			return
		}
	} else {
		// Parse from query parameters for easier testing
		req.CheckoutCode = r.URL.Query().Get("code")
	}

	// Validate request
	if err := ph.validatePurchaseRequest(&req); err != nil {
		ph.sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// Process purchase
	ctx := context.Background()
	response, statusCode := ph.processPurchase(ctx, &req)
	
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// validatePurchaseRequest validates the purchase request parameters
func (ph *PurchaseHandler) validatePurchaseRequest(req *PurchaseRequest) error {
	if req.CheckoutCode == "" {
		return fmt.Errorf("checkout_code is required")
	}

	// Basic format validation for checkout code
	if len(req.CheckoutCode) < 5 || len(req.CheckoutCode) > 50 {
		return fmt.Errorf("invalid checkout_code format")
	}

	return nil
}

// processPurchase handles the core purchase logic with atomic operations
func (ph *PurchaseHandler) processPurchase(ctx context.Context, req *PurchaseRequest) (*PurchaseResponse, int) {
	// 1. Verify checkout code and get checkout details
	checkout, err := ph.verifyCheckoutCode(ctx, req.CheckoutCode)
	if err != nil {
		log.Printf("Checkout code verification failed: %v", err)
		return &PurchaseResponse{
			Success: false,
			Message: "Invalid or expired checkout code",
		}, http.StatusBadRequest
	}

	// 2. Check if checkout has already been used
	if checkout.Status != "pending" {
		return &PurchaseResponse{
			Success: false,
			Message: "Checkout code has already been used",
		}, http.StatusBadRequest
	}

	// 3. Check if checkout has expired
	if time.Now().After(checkout.ExpiresAt) {
		return &PurchaseResponse{
			Success: false,
			Message: "Checkout code has expired",
		}, http.StatusBadRequest
	}

	// 4. Get the associated sale and verify it's still active
	sale, err := ph.saleService.GetCurrentActiveSale(ctx)
	if err != nil || sale == nil || sale.ID != checkout.SaleID {
		return &PurchaseResponse{
			Success: false,
			Message: "Sale is no longer active",
		}, http.StatusBadRequest
	}

	// 5. Get item details
	item, err := ph.itemService.GetItemByID(ctx, checkout.ItemID)
	if err != nil {
		log.Printf("Error getting item %s: %v", checkout.ItemID, err)
		return &PurchaseResponse{
			Success: false,
			Error: "Item not found",
		}, http.StatusBadRequest
	}

	// 6. Perform atomic purchase operation using Redis Lua script
	purchaseResult, err := ph.redis.AttemptPurchase(ctx, sale.ID, checkout.UserID, checkout.ItemID)
	if err != nil {
		log.Printf("Purchase attempt failed: %v", err)
		return &PurchaseResponse{
			Success: false,
			Error: "Purchase failed",
		}, http.StatusInternalServerError
	}

	// 7. Check purchase result
	switch purchaseResult.Status {
	case "success":
		// Purchase successful, create purchase record in database
		return ph.completePurchase(ctx, checkout, item, purchaseResult)
		
	case "sold_out":
		return &PurchaseResponse{
			Success: false,
			Message: "Sorry, this item is sold out",
		}, http.StatusConflict
		
	case "user_limit_exceeded":
		return &PurchaseResponse{
			Success: false,
			Message: fmt.Sprintf("Purchase limit exceeded. You can only purchase %d items per sale", 10),
			UserPurchases: purchaseResult.UserPurchases,
		}, http.StatusConflict
		
	case "sale_not_active":
		return &PurchaseResponse{
			Success: false,
			Message: "Sale is not currently active",
		}, http.StatusBadRequest
		
	default:
		return &PurchaseResponse{
			Success: false,
			Error: "Unknown purchase error",
		}, http.StatusInternalServerError
	}
}

// verifyCheckoutCode verifies and retrieves checkout details
func (ph *PurchaseHandler) verifyCheckoutCode(ctx context.Context, code string) (*models.Checkout, error) {
	// Use database lookup directly since Redis doesn't store ExpiresAt field
	// This ensures we get the complete checkout record including expiration time
	checkout, err := ph.db.GetCheckoutByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to get checkout from database: %w", err)
	}

	if checkout == nil {
		return nil, fmt.Errorf("checkout code not found")
	}

	return checkout, nil
}

// completePurchase finalizes the purchase by creating database records
func (ph *PurchaseHandler) completePurchase(ctx context.Context, checkout *models.Checkout, item *models.Item, purchaseResult *interfaces.PurchaseResult) (*PurchaseResponse, int) {
	now := time.Now()

	// Create purchase record
	purchase := &models.Purchase{
		UserID:      checkout.UserID,
		ItemID:      checkout.ItemID,
		SaleID:      checkout.SaleID,
		CheckoutID:  checkout.ID,
		Price:       item.Price,
		Status:      "completed",
		PurchasedAt: now,
	}

	// Begin database transaction to ensure consistency
	tx, err := ph.db.BeginTransaction(ctx)
	if err != nil {
		log.Printf("Failed to begin transaction: %v", err)
		return &PurchaseResponse{
			Success: false,
			Error: "Transaction failed",
		}, http.StatusInternalServerError
	}
	defer tx.Rollback() // Will be no-op if transaction is committed

	// Create purchase record
	if err := ph.db.CreatePurchase(ctx, purchase); err != nil {
		log.Printf("Failed to create purchase record: %v", err)
		return &PurchaseResponse{
			Success: false,
			Error: "Failed to record purchase",
		}, http.StatusInternalServerError
	}

	// Update checkout status to 'used'
	checkout.Status = "used"
	checkout.Purchased = true
	checkout.UpdatedAt = now
	if err := ph.db.UpdateCheckout(ctx, checkout); err != nil {
		log.Printf("Failed to update checkout status: %v", err)
		return &PurchaseResponse{
			Success: false,
			Error: "Failed to update checkout",
		}, http.StatusInternalServerError
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		return &PurchaseResponse{
			Success: false,
			Error: "Transaction commit failed",
		}, http.StatusInternalServerError
	}

	// Return successful response
	return &PurchaseResponse{
		Success:       true,
		PurchaseID:    purchase.ID,
		Message:       "Purchase completed successfully",
		Item:          item,
		TotalPrice:    item.Price,
		PurchasedAt:   now,
		UserPurchases: purchaseResult.UserPurchases,
	}, http.StatusOK
}

// sendErrorResponse sends a standardized error response
func (ph *PurchaseHandler) sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := PurchaseResponse{
		Success: false,
		Error:   message,
	}
	
	json.NewEncoder(w).Encode(response)
} 