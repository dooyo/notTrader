package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"flash-sale-backend/internal/interfaces"
	"flash-sale-backend/internal/models"
)

// SaleServiceImpl implements interfaces.SaleService
type SaleServiceImpl struct {
	db    interfaces.DatabaseInterface
	redis interfaces.RedisInterface
}

// NewSaleService creates a new sale service
func NewSaleService(db interfaces.DatabaseInterface, redis interfaces.RedisInterface) *SaleServiceImpl {
	return &SaleServiceImpl{
		db:    db,
		redis: redis,
	}
}

// CreateHourlySale creates a new hourly flash sale
func (s *SaleServiceImpl) CreateHourlySale(ctx context.Context) (*models.Sale, error) {
	now := time.Now()
	
	// Round down to the current hour
	startTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	endTime := startTime.Add(time.Hour)

	// Create sale model
	sale := &models.Sale{
		StartTime:      startTime,
		EndTime:        endTime,
		ItemsAvailable: 10000, // Contest requirement: exactly 10,000 items
		ItemsSold:      0,
		Active:         true,
	}

	// Deactivate any existing active sales first
	if err := s.deactivateAllSales(ctx); err != nil {
		log.Printf("Warning: failed to deactivate existing sales: %v", err)
		// Continue anyway - this is not critical
	}

	// Create sale in database
	if err := s.db.CreateSale(ctx, sale); err != nil {
		return nil, fmt.Errorf("failed to create sale in database: %w", err)
	}

	// Setup sale in Redis for atomic operations
	if err := s.redis.SetupSale(ctx, sale.ID, sale.ItemsAvailable); err != nil {
		// Rollback database sale if Redis setup fails
		s.db.DeactivateSale(ctx, sale.ID)
		return nil, fmt.Errorf("failed to setup sale in Redis: %w", err)
	}

	log.Printf("Created new flash sale %d: %v to %v", sale.ID, startTime, endTime)
	return sale, nil
}

// GetCurrentActiveSale returns the currently active sale
func (s *SaleServiceImpl) GetCurrentActiveSale(ctx context.Context) (*models.Sale, error) {
	// Try Redis first for performance
	activeSaleID, err := s.redis.GetActiveSaleID(ctx)
	if err != nil {
		log.Printf("Warning: failed to get active sale from Redis: %v", err)
		// Fall back to database
	} else if activeSaleID > 0 {
		// Get sale details from database
		sale, err := s.db.GetSaleByID(ctx, activeSaleID)
		if err != nil {
			log.Printf("Warning: failed to get sale %d from database: %v", activeSaleID, err)
		} else if sale != nil && sale.Active {
			// Sync Redis counter with database if needed
			s.syncSaleCounters(ctx, sale)
			return sale, nil
		}
	}

	// Fall back to database query
	sale, err := s.db.GetActiveSale(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sale from database: %w", err)
	}

	if sale == nil {
		return nil, nil // No active sale
	}

	// Update Redis with active sale ID
	if err := s.redis.SetActiveSaleID(ctx, sale.ID); err != nil {
		log.Printf("Warning: failed to update active sale ID in Redis: %v", err)
	}

	return sale, nil
}

// ActivateSale activates a specific sale
func (s *SaleServiceImpl) ActivateSale(ctx context.Context, saleID int) error {
	// Deactivate all other sales first
	if err := s.deactivateAllSales(ctx); err != nil {
		return fmt.Errorf("failed to deactivate existing sales: %w", err)
	}

	// Get sale to activate
	sale, err := s.db.GetSaleByID(ctx, saleID)
	if err != nil {
		return fmt.Errorf("failed to get sale %d: %w", saleID, err)
	}

	if sale == nil {
		return fmt.Errorf("sale %d not found", saleID)
	}

	// Activate sale in database (this will be handled by database triggers for updated_at)
	// We'll use the database interface method for activation
	
	// Setup sale in Redis
	if err := s.redis.SetupSale(ctx, saleID, sale.ItemsAvailable); err != nil {
		return fmt.Errorf("failed to setup sale in Redis: %w", err)
	}

	log.Printf("Activated sale %d", saleID)
	return nil
}

// DeactivateSale deactivates a specific sale
func (s *SaleServiceImpl) DeactivateSale(ctx context.Context, saleID int) error {
	// Deactivate in database
	if err := s.db.DeactivateSale(ctx, saleID); err != nil {
		return fmt.Errorf("failed to deactivate sale in database: %w", err)
	}

	// Clear from Redis active sale
	activeSaleID, err := s.redis.GetActiveSaleID(ctx)
	if err == nil && activeSaleID == saleID {
		// Clear active sale ID (set to 0)
		if err := s.redis.SetActiveSaleID(ctx, 0); err != nil {
			log.Printf("Warning: failed to clear active sale ID in Redis: %v", err)
		}
	}

	log.Printf("Deactivated sale %d", saleID)
	return nil
}

// GetSaleStatus returns detailed status of a specific sale
func (s *SaleServiceImpl) GetSaleStatus(ctx context.Context, saleID int) (*models.Sale, error) {
	// Get sale from database
	sale, err := s.db.GetSaleByID(ctx, saleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sale from database: %w", err)
	}

	if sale == nil {
		return nil, nil // Sale not found
	}

	// Get real-time sold count from Redis
	soldItems, err := s.redis.GetSoldItems(ctx, saleID)
	if err != nil {
		log.Printf("Warning: failed to get sold items from Redis for sale %d: %v", saleID, err)
		// Use database value as fallback
	} else {
		// Update with Redis value for real-time accuracy
		sale.ItemsSold = soldItems
	}

	return sale, nil
}

// GetSaleItemsSold returns the number of items sold for a specific sale
func (s *SaleServiceImpl) GetSaleItemsSold(ctx context.Context, saleID int) (int, error) {
	// Try Redis first for real-time count
	soldItems, err := s.redis.GetSoldItems(ctx, saleID)
	if err != nil {
		log.Printf("Warning: failed to get sold items from Redis: %v", err)
		
		// Fall back to database
		sale, err := s.db.GetSaleByID(ctx, saleID)
		if err != nil {
			return 0, fmt.Errorf("failed to get sale from database: %w", err)
		}
		
		if sale == nil {
			return 0, fmt.Errorf("sale %d not found", saleID)
		}
		
		return sale.ItemsSold, nil
	}

	return soldItems, nil
}

// Helper methods

// deactivateAllSales deactivates all currently active sales
func (s *SaleServiceImpl) deactivateAllSales(ctx context.Context) error {
	// For now, we'll get the active sale and deactivate it
	// In a production system, you might want to batch this operation
	activeSale, err := s.db.GetActiveSale(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active sale: %w", err)
	}

	if activeSale != nil {
		if err := s.db.DeactivateSale(ctx, activeSale.ID); err != nil {
			return fmt.Errorf("failed to deactivate sale %d: %w", activeSale.ID, err)
		}
	}

	return nil
}

// syncSaleCounters ensures Redis and database counters are in sync
func (s *SaleServiceImpl) syncSaleCounters(ctx context.Context, sale *models.Sale) {
	// Get real-time count from Redis
	soldItems, err := s.redis.GetSoldItems(ctx, sale.ID)
	if err != nil {
		log.Printf("Warning: failed to get sold items from Redis for sync: %v", err)
		return
	}

	// If Redis count differs significantly from database, update database
	if abs(soldItems-sale.ItemsSold) > 10 { // Allow small discrepancy
		if err := s.db.UpdateSaleItemsSold(ctx, sale.ID, soldItems); err != nil {
			log.Printf("Warning: failed to sync database with Redis count: %v", err)
		} else {
			log.Printf("Synced database count for sale %d: %d -> %d", sale.ID, sale.ItemsSold, soldItems)
		}
	}
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// BackgroundSaleManager handles automatic sale lifecycle management
type BackgroundSaleManager struct {
	saleService interfaces.SaleService
	stopChan    chan struct{}
}

// NewBackgroundSaleManager creates a new background sale manager
func NewBackgroundSaleManager(saleService interfaces.SaleService) *BackgroundSaleManager {
	return &BackgroundSaleManager{
		saleService: saleService,
		stopChan:    make(chan struct{}),
	}
}

// Start begins the background sale management process
func (bsm *BackgroundSaleManager) Start(ctx context.Context) {
	log.Println("Starting background sale manager")
	
	// Create initial sale if none exists
	go bsm.ensureActiveSale(ctx)
	
	// Set up hourly ticker for new sales
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go bsm.createNewHourlySale(ctx)
		case <-bsm.stopChan:
			log.Println("Stopping background sale manager")
			return
		}
	}
}

// Stop stops the background sale manager
func (bsm *BackgroundSaleManager) Stop() {
	close(bsm.stopChan)
}

// ensureActiveSale creates a sale if none is currently active
func (bsm *BackgroundSaleManager) ensureActiveSale(ctx context.Context) {
	activeSale, err := bsm.saleService.GetCurrentActiveSale(ctx)
	if err != nil {
		log.Printf("Error checking for active sale: %v", err)
		return
	}

	if activeSale == nil {
		log.Println("No active sale found, creating initial sale")
		_, err := bsm.saleService.CreateHourlySale(ctx)
		if err != nil {
			log.Printf("Error creating initial sale: %v", err)
		}
	} else {
		log.Printf("Found active sale %d", activeSale.ID)
	}
}

// createNewHourlySale creates a new hourly sale (deactivating the previous one)
func (bsm *BackgroundSaleManager) createNewHourlySale(ctx context.Context) {
	log.Println("Creating new hourly sale")
	
	sale, err := bsm.saleService.CreateHourlySale(ctx)
	if err != nil {
		log.Printf("Error creating hourly sale: %v", err)
		return
	}

	log.Printf("Successfully created new sale %d", sale.ID)
} 