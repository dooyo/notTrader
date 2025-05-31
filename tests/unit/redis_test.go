package unit

import (
	"context"
	"testing"
)

func TestRedis_AtomicPurchase_Success(t *testing.T) {
	mockRedis := NewMockRedis()
	ctx := context.Background()

	// Test successful purchase
	success, status, totalSold, userPurchases, err := mockRedis.AtomicPurchase(ctx, 1, "user1", 10000, 10)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !success {
		t.Error("Expected successful purchase")
	}

	if status != "success" {
		t.Errorf("Expected status 'success', got: %s", status)
	}

	if totalSold != 1 {
		t.Errorf("Expected total sold 1, got: %d", totalSold)
	}

	if userPurchases != 1 {
		t.Errorf("Expected user purchases 1, got: %d", userPurchases)
	}
}

func TestRedis_AtomicPurchase_UserLimit(t *testing.T) {
	mockRedis := NewMockRedis()
	ctx := context.Background()

	// Purchase 10 items (user limit)
	for i := 0; i < 10; i++ {
		success, _, _, _, err := mockRedis.AtomicPurchase(ctx, 1, "user1", 10000, 10)
		if err != nil {
			t.Fatalf("Expected no error on purchase %d, got: %v", i+1, err)
		}
		if !success {
			t.Errorf("Expected successful purchase %d", i+1)
		}
	}

	// 11th purchase should fail due to user limit
	success, status, _, userPurchases, err := mockRedis.AtomicPurchase(ctx, 1, "user1", 10000, 10)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if success {
		t.Error("Expected purchase to fail due to user limit")
	}

	if status != "user_limit_exceeded" {
		t.Errorf("Expected status 'user_limit_exceeded', got: %s", status)
	}

	if userPurchases != 10 {
		t.Errorf("Expected user purchases 10, got: %d", userPurchases)
	}
}

func TestRedis_AtomicPurchase_SoldOut(t *testing.T) {
	mockRedis := NewMockRedis()
	ctx := context.Background()

	// Purchase all available items (limit = 5 for this test)
	for i := 0; i < 5; i++ {
		userID := "user" + string(rune('1'+i))
		success, _, _, _, err := mockRedis.AtomicPurchase(ctx, 1, userID, 5, 10)
		if err != nil {
			t.Fatalf("Expected no error on purchase %d, got: %v", i+1, err)
		}
		if !success {
			t.Errorf("Expected successful purchase %d", i+1)
		}
	}

	// Next purchase should fail due to sold out
	success, status, totalSold, _, err := mockRedis.AtomicPurchase(ctx, 1, "user6", 5, 10)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if success {
		t.Error("Expected purchase to fail due to sold out")
	}

	if status != "sold_out" {
		t.Errorf("Expected status 'sold_out', got: %s", status)
	}

	if totalSold != 5 {
		t.Errorf("Expected total sold 5, got: %d", totalSold)
	}
}

func TestRedis_ConcurrentPurchases(t *testing.T) {
	mockRedis := NewMockRedis()
	ctx := context.Background()

	numGoroutines := 100
	results := make(chan bool, numGoroutines)

	// Send concurrent purchase requests
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			userID := "user" + string(rune('1'+id%50)) // 50 different users
			success, _, _, _, err := mockRedis.AtomicPurchase(ctx, 1, userID, 10000, 10)
			if err != nil {
				results <- false
				return
			}
			results <- success
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numGoroutines; i++ {
		if <-results {
			successCount++
		}
	}

	// Should have some successful purchases (exact number depends on user limits)
	if successCount == 0 {
		t.Error("Expected at least some successful purchases")
	}

	// Verify total sold items
	totalSold, err := mockRedis.GetSoldItems(ctx, 1)
	if err != nil {
		t.Fatalf("Expected no error getting sold items, got: %v", err)
	}

	if totalSold != successCount {
		t.Errorf("Expected total sold %d, got: %d", successCount, totalSold)
	}
} 