package unit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"flash-sale-backend/internal/handlers"
	"flash-sale-backend/internal/models"
)

func TestPurchaseHandler_ValidPurchase(t *testing.T) {
	// Setup mocks
	mockSaleService := NewMockSaleService()
	mockSaleService.currentSale = &models.Sale{
		ID:        1,
		StartTime: time.Now().Add(-time.Minute),
		EndTime:   time.Now().Add(time.Hour),
		Active:    true,
	}
	
	mockItemService := NewMockItemService()
	mockItemService.items["item1"] = &models.Item{
		ID:    "item1", 
		Name:  "Test Item", 
		Price: 99.99,
	}
	
	mockDB := NewMockDatabase()
	mockRedis := NewMockRedis()

	// Create a valid checkout first
	checkout := &models.CheckoutAttempt{
		Code:      "CHK_test_123",
		SaleID:    1,
		UserID:    "user123",
		ItemID:    "item1",
		Status:    "pending",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now(),
	}
	mockDB.checkouts[checkout.Code] = checkout

	handler := handlers.NewPurchaseHandler(mockSaleService, mockItemService, mockDB, mockRedis)

	// Test valid purchase request
	requestBody := map[string]string{
		"checkout_code": "CHK_test_123",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/purchase", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandlePurchase(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got: %d", w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || !success {
		t.Error("Expected success: true")
	}

	if _, ok := response["purchase_id"].(float64); !ok {
		t.Error("Expected purchase_id in response")
	}
}

func TestPurchaseHandler_InvalidMethod(t *testing.T) {
	handler := handlers.NewPurchaseHandler(nil, nil, nil, nil)

	req := httptest.NewRequest("GET", "/purchase", nil)
	w := httptest.NewRecorder()

	handler.HandlePurchase(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got: %d", w.Code)
	}
}

func TestPurchaseHandler_MissingCheckoutCode(t *testing.T) {
	handler := handlers.NewPurchaseHandler(nil, nil, nil, nil)

	// Test empty request body
	req := httptest.NewRequest("POST", "/purchase", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandlePurchase(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got: %d", w.Code)
	}
}

func TestPurchaseHandler_ExpiredCheckout(t *testing.T) {
	mockSaleService := NewMockSaleService()
	mockSaleService.currentSale = &models.Sale{
		ID:        1,
		StartTime: time.Now().Add(-time.Minute),
		EndTime:   time.Now().Add(time.Hour),
		Active:    true,
	}
	
	mockItemService := NewMockItemService()
	mockDB := NewMockDatabase()
	mockRedis := NewMockRedis()

	// Create an expired checkout
	checkout := &models.CheckoutAttempt{
		Code:      "CHK_expired_123",
		SaleID:    1,
		UserID:    "user123",
		ItemID:    "item1",
		Status:    "pending",
		ExpiresAt: time.Now().Add(-time.Minute), // Expired 1 minute ago
		CreatedAt: time.Now().Add(-11 * time.Minute),
	}
	mockDB.checkouts[checkout.Code] = checkout

	handler := handlers.NewPurchaseHandler(mockSaleService, mockItemService, mockDB, mockRedis)

	requestBody := map[string]string{
		"checkout_code": "CHK_expired_123",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/purchase", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandlePurchase(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got: %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	
	if message, ok := response["message"].(string); !ok || message != "Checkout code has expired" {
		t.Error("Expected 'Checkout code has expired' message")
	}
} 