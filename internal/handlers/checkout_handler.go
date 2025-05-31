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

	"github.com/google/uuid"
)

// CheckoutHandler handles checkout-related HTTP requests
type CheckoutHandler struct {
	saleService interfaces.SaleService
	itemService interfaces.ItemService
	db          interfaces.DatabaseInterface
	redis       interfaces.RedisInterface
}

// NewCheckoutHandler creates a new checkout handler
func NewCheckoutHandler(
	saleService interfaces.SaleService,
	itemService interfaces.ItemService,
	db interfaces.DatabaseInterface,
	redis interfaces.RedisInterface,
) *CheckoutHandler {
	return &CheckoutHandler{
		saleService: saleService,
		itemService: itemService,
		db:          db,
		redis:       redis,
	}
}

// CheckoutRequest represents the checkout request structure
type CheckoutRequest struct {
	UserID string `json:"user_id"`
	ItemID string `json:"item_id"`
}

// CheckoutResponse represents the checkout response structure
type CheckoutResponse struct {
	Success     bool      `json:"success"`
	CheckoutCode string   `json:"checkout_code,omitempty"`
	Message     string    `json:"message,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	Item        *models.Item `json:"item,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// HandleCheckout processes POST /checkout requests
func (ch *CheckoutHandler) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		ch.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Parse request
	var req CheckoutRequest
	
	// Check if request has JSON body
	if r.Header.Get("Content-Type") == "application/json" {
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&req); err != nil {
			ch.sendErrorResponse(w, http.StatusBadRequest, "Invalid JSON format")
			return
		}
	} else {
		// Parse from query parameters for easier testing
		req.UserID = r.URL.Query().Get("user_id")
		req.ItemID = r.URL.Query().Get("item_id")
	}

	// Validate request
	if err := ch.validateCheckoutRequest(&req); err != nil {
		ch.sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// Process checkout
	ctx := context.Background()
	response, statusCode := ch.processCheckout(ctx, &req)
	
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// validateCheckoutRequest validates the checkout request parameters
func (ch *CheckoutHandler) validateCheckoutRequest(req *CheckoutRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}

	if req.ItemID == "" {
		return fmt.Errorf("item_id is required")
	}

	// Validate user ID format (basic validation)
	if len(req.UserID) < 1 || len(req.UserID) > 100 {
		return fmt.Errorf("user_id must be between 1 and 100 characters")
	}

	// Validate item ID format using item service
	if err := ch.itemService.ValidateItemID(req.ItemID); err != nil {
		return fmt.Errorf("invalid item_id: %w", err)
	}

	return nil
}

// processCheckout handles the core checkout logic
func (ch *CheckoutHandler) processCheckout(ctx context.Context, req *CheckoutRequest) (*CheckoutResponse, int) {
	// 1. Check if there's an active sale
	activeSale, err := ch.saleService.GetCurrentActiveSale(ctx)
	if err != nil {
		log.Printf("Error getting active sale: %v", err)
		return &CheckoutResponse{
			Success: false,
			Error:   "Unable to process checkout at this time",
		}, http.StatusInternalServerError
	}

	if activeSale == nil {
		return &CheckoutResponse{
			Success: false,
			Message: "No active sale at this time",
		}, http.StatusBadRequest
	}

	// 2. Check if sale is still within time window
	now := time.Now()
	if now.Before(activeSale.StartTime) || now.After(activeSale.EndTime) {
		return &CheckoutResponse{
			Success: false,
			Message: "Sale is not currently active",
		}, http.StatusBadRequest
	}

	// 3. Validate item exists
	item, err := ch.itemService.GetItemByID(ctx, req.ItemID)
	if err != nil {
		log.Printf("Error getting item %s: %v", req.ItemID, err)
		return &CheckoutResponse{
			Success: false,
			Error:   "Invalid item",
		}, http.StatusBadRequest
	}

	// 4. Generate unique checkout code
	checkoutCode := ch.generateCheckoutCode()

	// 5. Create checkout record
	checkout := &models.Checkout{
		Code:      checkoutCode,
		UserID:    req.UserID,
		ItemID:    req.ItemID,
		SaleID:    activeSale.ID,
		Status:    "pending",
		ExpiresAt: now.Add(10 * time.Minute), // 10-minute expiration
		CreatedAt: now,
	}

	// 6. Persist checkout attempt in database
	if err := ch.db.CreateCheckout(ctx, checkout); err != nil {
		log.Printf("Error creating checkout record: %v", err)
		return &CheckoutResponse{
			Success: false,
			Error:   "Unable to process checkout",
		}, http.StatusInternalServerError
	}

	// 7. Cache checkout code in Redis for fast verification (TTL: 10 minutes)
	if err := ch.redis.SetCheckoutCode(ctx, checkoutCode, activeSale.ID, req.UserID, req.ItemID); err != nil {
		log.Printf("Warning: Failed to cache checkout code in Redis: %v", err)
		// Continue anyway - database has the record
	}

	// 8. Return successful response
	return &CheckoutResponse{
		Success:      true,
		CheckoutCode: checkoutCode,
		Message:      "Checkout code generated successfully",
		ExpiresAt:    checkout.ExpiresAt,
		Item:         item,
	}, http.StatusOK
}

// generateCheckoutCode creates a unique checkout code
func (ch *CheckoutHandler) generateCheckoutCode() string {
	// Generate UUID-based code for uniqueness
	uuid := uuid.New()
	
	// Create a shorter, more user-friendly code
	// Use first 8 characters of UUID + timestamp suffix for uniqueness
	timestamp := time.Now().Unix() % 10000 // Last 4 digits of timestamp
	
	return fmt.Sprintf("CHK_%s_%d", uuid.String()[:8], timestamp)
}

// sendErrorResponse sends a standardized error response
func (ch *CheckoutHandler) sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := CheckoutResponse{
		Success: false,
		Error:   message,
	}
	
	json.NewEncoder(w).Encode(response)
} 