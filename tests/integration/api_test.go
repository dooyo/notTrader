package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"flash-sale-backend/internal/database"
	"flash-sale-backend/internal/handlers"
	"flash-sale-backend/internal/services"
)

// TestFullAPIFlow tests the complete checkout -> purchase flow
func TestFullAPIFlow(t *testing.T) {
	// Skip if no database available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Define configuration inline
	databaseURL := "postgres://postgres:password@localhost:5432/flashsale?sslmode=disable"
	redisURL := "localhost:6379"

	// Initialize database connections
	db, err := database.NewPostgresDB(databaseURL)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
	}
	defer db.Close()

	redisClient, err := database.NewRedisClient(redisURL, "", 0)
	if err != nil {
		t.Skipf("Could not connect to test Redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize services
	saleService := services.NewSaleService(db, redisClient)
	itemService := services.NewItemService()

	// Initialize handlers
	checkoutHandler := handlers.NewCheckoutHandler(saleService, itemService, db, redisClient)
	purchaseHandler := handlers.NewPurchaseHandler(saleService, itemService, db, redisClient)

	// Create a test sale
	sale, err := saleService.CreateHourlySale(context.Background())
	if err != nil {
		t.Fatalf("Failed to create test sale: %v", err)
	}

	// Test 1: Checkout
	checkoutReq := httptest.NewRequest("POST", "/checkout?user_id=testuser&item_id=item1", nil)
	checkoutW := httptest.NewRecorder()

	checkoutHandler.HandleCheckout(checkoutW, checkoutReq)

	if checkoutW.Code != http.StatusOK {
		t.Fatalf("Checkout failed with status %d: %s", checkoutW.Code, checkoutW.Body.String())
	}

	var checkoutResponse map[string]interface{}
	err = json.Unmarshal(checkoutW.Body.Bytes(), &checkoutResponse)
	if err != nil {
		t.Fatalf("Failed to parse checkout response: %v", err)
	}

	checkoutCode, ok := checkoutResponse["checkout_code"].(string)
	if !ok {
		t.Fatal("No checkout code in response")
	}

	// Test 2: Purchase
	purchaseBody := map[string]string{
		"checkout_code": checkoutCode,
	}
	purchaseJSON, _ := json.Marshal(purchaseBody)

	purchaseReq := httptest.NewRequest("POST", "/purchase", bytes.NewBuffer(purchaseJSON))
	purchaseReq.Header.Set("Content-Type", "application/json")
	purchaseW := httptest.NewRecorder()

	purchaseHandler.HandlePurchase(purchaseW, purchaseReq)

	if purchaseW.Code != http.StatusOK {
		t.Fatalf("Purchase failed with status %d: %s", purchaseW.Code, purchaseW.Body.String())
	}

	var purchaseResponse map[string]interface{}
	err = json.Unmarshal(purchaseW.Body.Bytes(), &purchaseResponse)
	if err != nil {
		t.Fatalf("Failed to parse purchase response: %v", err)
	}

	if success, ok := purchaseResponse["success"].(bool); !ok || !success {
		t.Error("Purchase was not successful")
	}

	// Test 3: Verify purchase was recorded
	if purchaseID, ok := purchaseResponse["purchase_id"].(float64); !ok || purchaseID <= 0 {
		t.Error("Invalid purchase ID in response")
	}

	t.Logf("Successfully completed checkout -> purchase flow for sale %d", sale.ID)
}

// TestConcurrentCheckouts tests multiple users checking out simultaneously
func TestConcurrentCheckouts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup (same as above)
	databaseURL := "postgres://postgres:password@localhost:5432/flashsale?sslmode=disable"
	redisURL := "localhost:6379"

	db, err := database.NewPostgresDB(databaseURL)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
	}
	defer db.Close()

	redisClient, err := database.NewRedisClient(redisURL, "", 0)
	if err != nil {
		t.Skipf("Could not connect to test Redis: %v", err)
	}
	defer redisClient.Close()

	saleService := services.NewSaleService(db, redisClient)
	itemService := services.NewItemService()
	checkoutHandler := handlers.NewCheckoutHandler(saleService, itemService, db, redisClient)

	// Create a test sale
	sale, err := saleService.CreateHourlySale(context.Background())
	if err != nil {
		t.Fatalf("Failed to create test sale: %v", err)
	}

	// Test concurrent checkouts
	numUsers := 50
	results := make(chan int, numUsers)

	for i := 0; i < numUsers; i++ {
		go func(userID int) {
			req := httptest.NewRequest("POST", 
				fmt.Sprintf("/checkout?user_id=user%d&item_id=item1", userID), nil)
			w := httptest.NewRecorder()
			
			checkoutHandler.HandleCheckout(w, req)
			results <- w.Code
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numUsers; i++ {
		code := <-results
		if code == http.StatusOK {
			successCount++
		}
	}

	if successCount != numUsers {
		t.Errorf("Expected %d successful checkouts, got: %d", numUsers, successCount)
	}

	t.Logf("Successfully processed %d concurrent checkouts for sale %d", successCount, sale.ID)
}

// TestUserPurchaseLimit tests that a user cannot purchase more than 10 items per sale
func TestUserPurchaseLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	databaseURL := "postgres://postgres:password@localhost:5432/flashsale?sslmode=disable"
	redisURL := "localhost:6379"

	db, err := database.NewPostgresDB(databaseURL)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
	}
	defer db.Close()

	redisClient, err := database.NewRedisClient(redisURL, "", 0)
	if err != nil {
		t.Skipf("Could not connect to test Redis: %v", err)
	}
	defer redisClient.Close()

	saleService := services.NewSaleService(db, redisClient)
	itemService := services.NewItemService()
	checkoutHandler := handlers.NewCheckoutHandler(saleService, itemService, db, redisClient)
	purchaseHandler := handlers.NewPurchaseHandler(saleService, itemService, db, redisClient)

	// Create a test sale
	sale, err := saleService.CreateHourlySale(context.Background())
	if err != nil {
		t.Fatalf("Failed to create test sale: %v", err)
	}

	userID := "limit_test_user"

	// Test: Purchase 10 items (should all succeed)
	for i := 0; i < 10; i++ {
		// Checkout
		checkoutReq := httptest.NewRequest("POST", 
			fmt.Sprintf("/checkout?user_id=%s&item_id=item%d", userID, i+1), nil)
		checkoutW := httptest.NewRecorder()
		checkoutHandler.HandleCheckout(checkoutW, checkoutReq)

		if checkoutW.Code != http.StatusOK {
			t.Fatalf("Checkout %d failed with status %d: %s", i+1, checkoutW.Code, checkoutW.Body.String())
		}

		var checkoutResponse map[string]interface{}
		err = json.Unmarshal(checkoutW.Body.Bytes(), &checkoutResponse)
		if err != nil {
			t.Fatalf("Failed to parse checkout response %d: %v", i+1, err)
		}

		checkoutCode, ok := checkoutResponse["checkout_code"].(string)
		if !ok {
			t.Fatalf("No checkout code in response %d", i+1)
		}

		// Purchase
		purchaseBody := map[string]string{"checkout_code": checkoutCode}
		purchaseJSON, _ := json.Marshal(purchaseBody)

		purchaseReq := httptest.NewRequest("POST", "/purchase", bytes.NewBuffer(purchaseJSON))
		purchaseReq.Header.Set("Content-Type", "application/json")
		purchaseW := httptest.NewRecorder()
		purchaseHandler.HandlePurchase(purchaseW, purchaseReq)

		if purchaseW.Code != http.StatusOK {
			t.Fatalf("Purchase %d failed with status %d: %s", i+1, purchaseW.Code, purchaseW.Body.String())
		}

		var purchaseResponse map[string]interface{}
		err = json.Unmarshal(purchaseW.Body.Bytes(), &purchaseResponse)
		if err != nil {
			t.Fatalf("Failed to parse purchase response %d: %v", i+1, err)
		}

		if success, ok := purchaseResponse["success"].(bool); !ok || !success {
			t.Errorf("Purchase %d was not successful", i+1)
		}

		// Verify user purchase count
		if userPurchases, ok := purchaseResponse["user_purchases"].(float64); ok {
			if int(userPurchases) != i+1 {
				t.Errorf("Expected user purchases %d, got: %v", i+1, userPurchases)
			}
		}
	}

	// Test: 11th checkout should succeed (checkout always allowed)
	checkoutReq := httptest.NewRequest("POST", 
		fmt.Sprintf("/checkout?user_id=%s&item_id=item11", userID), nil)
	checkoutW := httptest.NewRecorder()
	checkoutHandler.HandleCheckout(checkoutW, checkoutReq)

	if checkoutW.Code != http.StatusOK {
		t.Fatalf("11th checkout failed with status %d: %s", checkoutW.Code, checkoutW.Body.String())
	}

	var checkoutResponse map[string]interface{}
	err = json.Unmarshal(checkoutW.Body.Bytes(), &checkoutResponse)
	if err != nil {
		t.Fatalf("Failed to parse 11th checkout response: %v", err)
	}

	checkoutCode, ok := checkoutResponse["checkout_code"].(string)
	if !ok {
		t.Fatal("No checkout code in 11th checkout response")
	}

	// Test: 11th purchase should FAIL due to user limit
	purchaseBody := map[string]string{"checkout_code": checkoutCode}
	purchaseJSON, _ := json.Marshal(purchaseBody)

	purchaseReq := httptest.NewRequest("POST", "/purchase", bytes.NewBuffer(purchaseJSON))
	purchaseReq.Header.Set("Content-Type", "application/json")
	purchaseW := httptest.NewRecorder()
	purchaseHandler.HandlePurchase(purchaseW, purchaseReq)

	// Purchase should return 409 Conflict for user limit exceeded
	if purchaseW.Code != http.StatusConflict {
		t.Fatalf("Expected 11th purchase to fail with 409, got status %d: %s", 
			purchaseW.Code, purchaseW.Body.String())
	}

	var purchaseResponse map[string]interface{}
	err = json.Unmarshal(purchaseW.Body.Bytes(), &purchaseResponse)
	if err != nil {
		t.Fatalf("Failed to parse 11th purchase response: %v", err)
	}

	// Verify the error response
	if success, ok := purchaseResponse["success"].(bool); !ok || success {
		t.Error("Expected 11th purchase to fail (success should be false)")
	}

	if message, ok := purchaseResponse["message"].(string); ok {
		t.Logf("11th purchase correctly failed with message: %s", message)
	}

	t.Logf("Successfully validated user purchase limit: %s made 10 purchases, 11th was rejected for sale %d", userID, sale.ID)
} 