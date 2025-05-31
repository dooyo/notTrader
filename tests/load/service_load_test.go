package load

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"flash-sale-backend/internal/handlers"
	"flash-sale-backend/internal/models"
	"flash-sale-backend/tests/unit"
)

// ServiceLoadConfig holds configuration for service-only load testing
type ServiceLoadConfig struct {
	NumUsers           int
	RequestsPerUser    int
	ConcurrentRequests int
}

// BenchmarkServiceCheckoutPerformance tests checkout handler performance with mocked dependencies
func BenchmarkServiceCheckoutPerformance(b *testing.B) {
	// Setup with mocks (no real databases)
	mockSaleService := unit.NewMockSaleService()
	mockSaleService.SetCurrentSale(&models.Sale{
		ID:        1,
		StartTime: time.Now().Add(-time.Minute),
		EndTime:   time.Now().Add(time.Hour),
		Active:    true,
	})
	
	mockItemService := unit.NewMockItemService()
	mockItemService.AddItem("item1", &models.Item{
		ID:    "item1", 
		Name:  "Test Item", 
		Price: 99.99,
	})
	
	mockDB := unit.NewMockDatabase()
	mockRedis := unit.NewMockRedis()

	handler := handlers.NewCheckoutHandler(mockSaleService, mockItemService, mockDB, mockRedis)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		userID := 0
		for pb.Next() {
			userID++
			req := httptest.NewRequest("POST", 
				fmt.Sprintf("/checkout?user_id=user%d&item_id=item1", userID), nil)
			w := httptest.NewRecorder()
			
			handler.HandleCheckout(w, req)
			
			if w.Code != http.StatusOK {
				b.Errorf("Checkout failed with status %d", w.Code)
			}
		}
	})
}

// BenchmarkServicePurchasePerformance tests purchase handler performance with mocked dependencies
func BenchmarkServicePurchasePerformance(b *testing.B) {
	// Setup with mocks
	mockSaleService := unit.NewMockSaleService()
	mockSaleService.SetCurrentSale(&models.Sale{
		ID:        1,
		StartTime: time.Now().Add(-time.Minute),
		EndTime:   time.Now().Add(time.Hour),
		Active:    true,
	})
	
	mockItemService := unit.NewMockItemService()
	mockItemService.AddItem("item1", &models.Item{
		ID:    "item1", 
		Name:  "Test Item", 
		Price: 99.99,
	})
	
	mockDB := unit.NewMockDatabase()
	mockRedis := unit.NewMockRedis()

	handler := handlers.NewPurchaseHandler(mockSaleService, mockItemService, mockDB, mockRedis)

	// Pre-create checkout codes in mock database
	checkoutCodes := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		code := fmt.Sprintf("CHK_bench_%d", i)
		checkout := &models.CheckoutAttempt{
			Code:      code,
			SaleID:    1,
			UserID:    fmt.Sprintf("user%d", i),
			ItemID:    "item1",
			Status:    "pending",
			ExpiresAt: time.Now().Add(10 * time.Minute),
			CreatedAt: time.Now(),
		}
		mockDB.CreateCheckoutAttempt(context.Background(), checkout)
		checkoutCodes[i] = code
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		index := 0
		for pb.Next() {
			if index >= len(checkoutCodes) {
				break
			}

			purchaseBody := map[string]string{
				"checkout_code": checkoutCodes[index],
			}
			jsonBody, _ := json.Marshal(purchaseBody)

			req := httptest.NewRequest("POST", "/purchase", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			handler.HandlePurchase(w, req)
			
			// Purchases may fail due to limits, but should not error
			if w.Code >= 500 {
				b.Errorf("Purchase failed with server error %d", w.Code)
			}
			
			index++
		}
	})
}

// TestServiceConcurrentLoad tests service performance under concurrent load using mocks
func TestServiceConcurrentLoad(t *testing.T) {
	config := &ServiceLoadConfig{
		NumUsers:           1000,  // Much higher since no DB bottleneck
		RequestsPerUser:    5,
		ConcurrentRequests: 1000,
	}

	// Setup service with mocks (fast, no I/O)
	handlers := setupServiceLoadTest(config)

	var wg sync.WaitGroup
	results := make(chan ServiceTestResult, config.NumUsers)
	start := time.Now()

	// Spawn concurrent users
	for i := 0; i < config.NumUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			
			result := performServiceUserFlow(handlers, userID)
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)

	duration := time.Since(start)

	// Collect and analyze results
	var successfulCheckouts, successfulPurchases int
	var totalCheckoutTime, totalPurchaseTime time.Duration

	for result := range results {
		if result.CheckoutSuccess {
			successfulCheckouts++
			totalCheckoutTime += result.CheckoutTime
		}
		if result.PurchaseSuccess {
			successfulPurchases++
			totalPurchaseTime += result.PurchaseTime
		}
	}

	// Performance metrics (should be much faster with mocks)
	avgCheckoutTime := totalCheckoutTime / time.Duration(successfulCheckouts)
	avgPurchaseTime := totalPurchaseTime / time.Duration(max(successfulPurchases, 1))
	requestsPerSecond := float64(config.NumUsers*2) / duration.Seconds() // checkout + purchase

	t.Logf("=== SERVICE LOAD TEST RESULTS ===")
	t.Logf("Total Users: %d", config.NumUsers)
	t.Logf("Total Duration: %v", duration)
	t.Logf("Requests/Second: %.2f", requestsPerSecond)
	t.Logf("Successful Checkouts: %d/%d (%.1f%%)", 
		successfulCheckouts, config.NumUsers, 
		float64(successfulCheckouts)/float64(config.NumUsers)*100)
	t.Logf("Successful Purchases: %d/%d (%.1f%%)", 
		successfulPurchases, config.NumUsers,
		float64(successfulPurchases)/float64(config.NumUsers)*100)
	t.Logf("Average Checkout Time: %v", avgCheckoutTime)
	t.Logf("Average Purchase Time: %v", avgPurchaseTime)

	// Much higher performance targets since no DB I/O
	if requestsPerSecond < 1000 {
		t.Errorf("Service performance target not met: %.2f req/s < 1000 req/s", requestsPerSecond)
	}

	if avgCheckoutTime > 5*time.Millisecond {
		t.Errorf("Service checkout too slow: %v > 5ms", avgCheckoutTime)
	}

	if avgPurchaseTime > 10*time.Millisecond {
		t.Errorf("Service purchase too slow: %v > 10ms", avgPurchaseTime)
	}

	t.Logf("Service load test completed successfully")
}

// ServiceTestResult holds the result of a service performance test
type ServiceTestResult struct {
	UserID          int
	CheckoutSuccess bool
	PurchaseSuccess bool
	CheckoutTime    time.Duration
	PurchaseTime    time.Duration
	Error           error
}

// ServiceLoadHandlers holds handlers configured with mocks
type ServiceLoadHandlers struct {
	checkout *handlers.CheckoutHandler
	purchase *handlers.PurchaseHandler
	mockDB   *unit.MockDatabaseInterface
	mockRedis *unit.MockRedisInterface
}

// setupServiceLoadTest initializes handlers with mock dependencies for performance testing
func setupServiceLoadTest(config *ServiceLoadConfig) *ServiceLoadHandlers {
	// Use mocks for maximum performance (no I/O)
	mockSaleService := unit.NewMockSaleService()
	mockSaleService.SetCurrentSale(&models.Sale{
		ID:        1,
		StartTime: time.Now().Add(-time.Minute),
		EndTime:   time.Now().Add(time.Hour),
		Active:    true,
	})
	
	mockItemService := unit.NewMockItemService()
	mockItemService.AddItem("item1", &models.Item{
		ID:    "item1", 
		Name:  "Test Item", 
		Price: 99.99,
	})
	
	mockDB := unit.NewMockDatabase()
	mockRedis := unit.NewMockRedis()

	// Initialize handlers with mocks
	checkoutHandler := handlers.NewCheckoutHandler(mockSaleService, mockItemService, mockDB, mockRedis)
	purchaseHandler := handlers.NewPurchaseHandler(mockSaleService, mockItemService, mockDB, mockRedis)

	return &ServiceLoadHandlers{
		checkout:  checkoutHandler,
		purchase:  purchaseHandler,
		mockDB:    mockDB,
		mockRedis: mockRedis,
	}
}

// performServiceUserFlow simulates a user flow testing only service performance
func performServiceUserFlow(handlers *ServiceLoadHandlers, userID int) ServiceTestResult {
	result := ServiceTestResult{UserID: userID}

	// Step 1: Checkout (testing service performance, not DB)
	checkoutStart := time.Now()
	checkoutReq := httptest.NewRequest("POST", 
		fmt.Sprintf("/checkout?user_id=user%d&item_id=item1", userID), nil)
	checkoutW := httptest.NewRecorder()
	
	handlers.checkout.HandleCheckout(checkoutW, checkoutReq)
	result.CheckoutTime = time.Since(checkoutStart)
	
	if checkoutW.Code != http.StatusOK {
		return result
	}
	result.CheckoutSuccess = true

	// Extract checkout code
	var checkoutResponse map[string]interface{}
	if err := json.Unmarshal(checkoutW.Body.Bytes(), &checkoutResponse); err != nil {
		result.Error = err
		return result
	}

	checkoutCode, ok := checkoutResponse["checkout_code"].(string)
	if !ok {
		return result
	}

	// Step 2: Purchase (testing service performance, not DB)
	purchaseStart := time.Now()
	purchaseBody := map[string]string{
		"checkout_code": checkoutCode,
	}
	purchaseJSON, _ := json.Marshal(purchaseBody)

	purchaseReq := httptest.NewRequest("POST", "/purchase", bytes.NewBuffer(purchaseJSON))
	purchaseReq.Header.Set("Content-Type", "application/json")
	purchaseW := httptest.NewRecorder()
	
	handlers.purchase.HandlePurchase(purchaseW, purchaseReq)
	result.PurchaseTime = time.Since(purchaseStart)
	
	if purchaseW.Code == http.StatusOK {
		result.PurchaseSuccess = true
	}

	return result
} 