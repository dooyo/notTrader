package unit

import (
	"context"
	"testing"
	"time"

	"flash-sale-backend/internal/models"
	"flash-sale-backend/internal/services"
)

func TestSaleService_CreateHourlySale(t *testing.T) {
	mockDB := NewMockDatabase()
	mockRedis := NewMockRedis()
	saleService := services.NewSaleService(mockDB, mockRedis)

	ctx := context.Background()

	// Test successful sale creation
	sale, err := saleService.CreateHourlySale(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if sale == nil {
		t.Fatal("Expected sale to be created, got nil")
	}

	if sale.ItemsAvailable != 10000 {
		t.Errorf("Expected 10000 items available, got: %d", sale.ItemsAvailable)
	}

	if sale.ItemsSold != 0 {
		t.Errorf("Expected 0 items sold, got: %d", sale.ItemsSold)
	}

	if !sale.Active {
		t.Error("Expected sale to be active")
	}

	// Verify sale duration is approximately 1 hour
	duration := sale.EndTime.Sub(sale.StartTime)
	expectedDuration := time.Hour
	if duration < expectedDuration-time.Minute || duration > expectedDuration+time.Minute {
		t.Errorf("Expected sale duration ~1 hour, got: %v", duration)
	}
}

func TestSaleService_GetCurrentActiveSale(t *testing.T) {
	mockDB := NewMockDatabase()
	mockRedis := NewMockRedis()
	saleService := services.NewSaleService(mockDB, mockRedis)

	ctx := context.Background()

	// Test when no active sale exists
	sale, err := saleService.GetCurrentActiveSale(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if sale != nil {
		t.Error("Expected no active sale, got sale")
	}

	// Create an active sale
	activeSale := &models.Sale{
		ID:             1,
		StartTime:      time.Now().Add(-time.Minute),
		EndTime:        time.Now().Add(time.Hour),
		ItemsAvailable: 10000,
		ItemsSold:      100,
		Active:         true,
	}
	mockDB.sales[1] = activeSale

	// Test getting active sale
	sale, err = saleService.GetCurrentActiveSale(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if sale == nil {
		t.Fatal("Expected active sale, got nil")
	}
	if sale.ID != 1 {
		t.Errorf("Expected sale ID 1, got: %d", sale.ID)
	}
}

func TestSaleService_ErrorHandling(t *testing.T) {
	mockDB := NewMockDatabase()
	mockDB.shouldError = true
	mockRedis := NewMockRedis()
	saleService := services.NewSaleService(mockDB, mockRedis)

	ctx := context.Background()

	// Test database error handling
	_, err := saleService.CreateHourlySale(ctx)
	if err == nil {
		t.Error("Expected error when database fails, got nil")
	}

	_, err = saleService.GetCurrentActiveSale(ctx)
	if err == nil {
		t.Error("Expected error when database fails, got nil")
	}
}

func TestSaleService_ConcurrentSaleCreation(t *testing.T) {
	mockDB := NewMockDatabase()
	mockRedis := NewMockRedis()
	saleService := services.NewSaleService(mockDB, mockRedis)

	ctx := context.Background()
	numGoroutines := 10
	results := make(chan error, numGoroutines)

	// Test concurrent sale creation
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := saleService.CreateHourlySale(ctx)
			results <- err
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		if err != nil {
			t.Errorf("Concurrent sale creation failed: %v", err)
		}
	}

	// Verify all sales were created
	if len(mockDB.sales) != numGoroutines {
		t.Errorf("Expected %d sales created, got: %d", numGoroutines, len(mockDB.sales))
	}
} 