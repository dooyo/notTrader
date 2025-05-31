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

	"flash-sale-backend/internal/database"
	"flash-sale-backend/internal/handlers"
	"flash-sale-backend/internal/models"
	"flash-sale-backend/internal/services"
)

// LoadTestConfig holds configuration for load testing
type LoadTestConfig struct {
	NumUsers       int
	RequestsPerUser int
	DatabaseURL    string
	RedisURL       string
}

// BenchmarkCheckoutConcurrency tests checkout performance under load
func BenchmarkCheckoutConcurrency(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping load test in short mode")
	}

	config := &LoadTestConfig{
		NumUsers:       100,
		RequestsPerUser: 10,
		DatabaseURL:    "postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable",
		RedisURL:       "redis://localhost:6379",
	}

	// Setup test environment
	db, redisClient, handlers, sale := setupLoadTest(b, config)
	defer db.Close()
	defer redisClient.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		userID := 0
		for pb.Next() {
			userID++
			req := httptest.NewRequest("POST", 
				fmt.Sprintf("/checkout?user_id=user%d&item_id=item1", userID), nil)
			w := httptest.NewRecorder()
			
			handlers.checkout.HandleCheckout(w, req)
			
			if w.Code != http.StatusOK {
				b.Errorf("Checkout failed with status %d", w.Code)
			}
		}
	})

	b.Logf("Completed load test for sale %d", sale.ID)
}

// BenchmarkPurchaseConcurrency tests purchase performance under load
func BenchmarkPurchaseConcurrency(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping load test in short mode")
	}

	config := &LoadTestConfig{
		NumUsers:       50,
		RequestsPerUser: 5,
		DatabaseURL:    "postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable",
		RedisURL:       "redis://localhost:6379",
	}

	// Setup test environment
	db, redisClient, handlers, sale := setupLoadTest(b, config)
	defer db.Close()
	defer redisClient.Close()

	// Pre-create checkout codes for purchase testing
	checkoutCodes := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		checkoutCodes[i] = createCheckoutCode(b, handlers.checkout, i)
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
			
			handlers.purchase.HandlePurchase(w, req)
			
			// Purchases may fail due to limits, but should not error
			if w.Code >= 500 {
				b.Errorf("Purchase failed with server error %d", w.Code)
			}
			
			index++
		}
	})

	b.Logf("Completed purchase load test for sale %d", sale.ID)
}

// TestConcurrentUserFlow tests the complete flow under concurrent load
func TestConcurrentUserFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	config := &LoadTestConfig{
		NumUsers:       200,
		RequestsPerUser: 1,
		DatabaseURL:    "postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable",
		RedisURL:       "redis://localhost:6379",
	}

	// Setup test environment
	db, redisClient, handlers, sale := setupLoadTest(t, config)
	defer db.Close()
	defer redisClient.Close()

	var wg sync.WaitGroup
	results := make(chan TestResult, config.NumUsers)

	start := time.Now()

	// Spawn concurrent users
	for i := 0; i < config.NumUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			
			result := performUserFlow(handlers, userID)
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

	// Performance metrics
	avgCheckoutTime := totalCheckoutTime / time.Duration(successfulCheckouts)
	avgPurchaseTime := totalPurchaseTime / time.Duration(max(successfulPurchases, 1))
	requestsPerSecond := float64(config.NumUsers) / duration.Seconds()

	t.Logf("=== LOAD TEST RESULTS ===")
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

	// Performance assertions
	if requestsPerSecond < 100 {
		t.Errorf("Performance target not met: %.2f req/s < 100 req/s", requestsPerSecond)
	}

	if avgCheckoutTime > 50*time.Millisecond {
		t.Errorf("Checkout too slow: %v > 50ms", avgCheckoutTime)
	}

	if avgPurchaseTime > 100*time.Millisecond {
		t.Errorf("Purchase too slow: %v > 100ms", avgPurchaseTime)
	}

	t.Logf("Load test completed successfully for sale %d", sale.ID)
}

// TestResult holds the result of a single user flow test
type TestResult struct {
	UserID          int
	CheckoutSuccess bool
	PurchaseSuccess bool
	CheckoutTime    time.Duration
	PurchaseTime    time.Duration
	Error           error
}

// LoadTestHandlers holds all the handlers for load testing
type LoadTestHandlers struct {
	checkout *handlers.CheckoutHandler
	purchase *handlers.PurchaseHandler
}

// setupLoadTest initializes the test environment for load testing
func setupLoadTest(tb testing.TB, config *LoadTestConfig) (*database.PostgresDB, *database.RedisClient, *LoadTestHandlers, *models.Sale) {
	// Initialize database connections
	db, err := database.NewPostgresDB(config.DatabaseURL)
	if err != nil {
		tb.Skipf("Could not connect to test database: %v", err)
	}

	redisClient, err := database.NewRedisClient(config.RedisURL, "", 0)
	if err != nil {
		tb.Skipf("Could not connect to test Redis: %v", err)
	}

	// Initialize services
	saleService := services.NewSaleService(db, redisClient)
	itemService := services.NewItemService()

	// Initialize handlers
	checkoutHandler := handlers.NewCheckoutHandler(saleService, itemService, db, redisClient)
	purchaseHandler := handlers.NewPurchaseHandler(saleService, itemService, db, redisClient)

	// Create a test sale
	sale, err := saleService.CreateHourlySale(context.Background())
	if err != nil {
		tb.Fatalf("Failed to create test sale: %v", err)
	}

	return db, redisClient, &LoadTestHandlers{
		checkout: checkoutHandler,
		purchase: purchaseHandler,
	}, sale
}

// createCheckoutCode creates a checkout code for testing purposes
func createCheckoutCode(tb testing.TB, checkoutHandler *handlers.CheckoutHandler, userID int) string {
	req := httptest.NewRequest("POST", 
		fmt.Sprintf("/checkout?user_id=user%d&item_id=item1", userID), nil)
	w := httptest.NewRecorder()
	
	checkoutHandler.HandleCheckout(w, req)
	
	if w.Code != http.StatusOK {
		tb.Fatalf("Failed to create checkout code: status %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	
	code, ok := response["checkout_code"].(string)
	if !ok {
		tb.Fatal("No checkout code in response")
	}
	
	return code
}

// performUserFlow simulates a complete user flow (checkout -> purchase)
func performUserFlow(handlers *LoadTestHandlers, userID int) TestResult {
	result := TestResult{UserID: userID}

	// Step 1: Checkout
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

	// Step 2: Purchase
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

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
} 