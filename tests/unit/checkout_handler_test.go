package unit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"flash-sale-backend/internal/handlers"
	"flash-sale-backend/internal/models"
)

func TestCheckoutHandler_ValidRequest(t *testing.T) {
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

	handler := handlers.NewCheckoutHandler(mockSaleService, mockItemService, mockDB, mockRedis)

	// Test valid request with query parameters
	req := httptest.NewRequest("POST", "/checkout?user_id=user123&item_id=item1", nil)
	w := httptest.NewRecorder()

	handler.HandleCheckout(w, req)

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

	if _, ok := response["checkout_code"].(string); !ok {
		t.Error("Expected checkout_code in response")
	}

	if _, ok := response["expires_at"].(string); !ok {
		t.Error("Expected expires_at in response")
	}
}

func TestCheckoutHandler_ConcurrentRequests(t *testing.T) {
	mockSaleService := NewMockSaleService()
	mockSaleService.currentSale = &models.Sale{
		ID: 1, Active: true, 
		StartTime: time.Now().Add(-time.Minute), 
		EndTime: time.Now().Add(time.Hour),
	}
	
	mockItemService := NewMockItemService()
	mockItemService.items["item1"] = &models.Item{
		ID: "item1", Name: "Test Item", Price: 99.99,
	}
	
	mockDB := NewMockDatabase()
	mockRedis := NewMockRedis()

	handler := handlers.NewCheckoutHandler(mockSaleService, mockItemService, mockDB, mockRedis)

	numRequests := 100
	results := make(chan int, numRequests)

	// Send concurrent requests
	for i := 0; i < numRequests; i++ {
		go func(id int) {
			userID := fmt.Sprintf("user%d", id)
			url := fmt.Sprintf("/checkout?user_id=%s&item_id=item1", userID)
			req := httptest.NewRequest("POST", url, nil)
			w := httptest.NewRecorder()
			handler.HandleCheckout(w, req)
			results <- w.Code
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numRequests; i++ {
		code := <-results
		if code == http.StatusOK {
			successCount++
		}
	}

	if successCount != numRequests {
		t.Errorf("Expected %d successful requests, got: %d", numRequests, successCount)
	}

	// Verify all checkout codes were created
	if len(mockDB.checkouts) != numRequests {
		t.Errorf("Expected %d checkout records, got: %d", numRequests, len(mockDB.checkouts))
	}
} 