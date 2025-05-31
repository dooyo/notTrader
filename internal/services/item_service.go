package services

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"flash-sale-backend/internal/models"

	"github.com/google/uuid"
)

// ItemServiceImpl implements interfaces.ItemService
type ItemServiceImpl struct {
	// In-memory cache of generated items for performance
	itemCache map[string]*models.Item
}

// NewItemService creates a new item service
func NewItemService() *ItemServiceImpl {
	return &ItemServiceImpl{
		itemCache: make(map[string]*models.Item),
	}
}

// GenerateItems creates a specified number of items at runtime
func (i *ItemServiceImpl) GenerateItems(ctx context.Context, count int) ([]models.Item, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive, got %d", count)
	}

	if count > 100000 { // Reasonable upper limit
		return nil, fmt.Errorf("count too large, maximum 100000, got %d", count)
	}

	items := make([]models.Item, count)
	now := time.Now()

	// Predefined item templates for variety
	itemTemplates := []struct {
		namePrefix  string
		description string
		basePrice   float64
	}{
		{"Flash Electronics", "High-tech gadget at incredible price", 299.99},
		{"Designer Fashion", "Premium clothing item with limited availability", 149.99},
		{"Home Essential", "Must-have household item for modern living", 79.99},
		{"Sports Gear", "Professional quality sports equipment", 199.99},
		{"Beauty Product", "Premium skincare and cosmetic item", 89.99},
		{"Kitchen Tool", "Essential cooking equipment for every chef", 59.99},
		{"Gaming Accessory", "Professional gaming equipment", 129.99},
		{"Health Supplement", "Premium wellness and health product", 49.99},
		{"Book Collection", "Bestselling books and educational materials", 29.99},
		{"Art Supply", "Professional quality creative materials", 39.99},
	}

	for idx := 0; idx < count; idx++ {
		// Generate unique item ID
		itemID := fmt.Sprintf("item_%s", uuid.New().String()[:8])
		
		// Select random template
		template := itemTemplates[rand.Intn(len(itemTemplates))]
		
		// Create unique name with variant number
		itemName := fmt.Sprintf("%s #%d", template.namePrefix, idx+1)
		
		// Add some price variation (±20%)
		priceVariation := 1.0 + (rand.Float64()-0.5)*0.4 // Random between 0.8 and 1.2
		finalPrice := template.basePrice * priceVariation
		
		// Round to 2 decimal places
		finalPrice = float64(int(finalPrice*100)) / 100

		item := models.Item{
			ID:          itemID,
			Name:        itemName,
			Description: template.description,
			Price:       finalPrice,
			CreatedAt:   now,
		}

		items[idx] = item
		
		// Cache the item for quick lookup
		i.itemCache[itemID] = &item
	}

	return items, nil
}

// GetItemByID returns a specific item by its ID
func (i *ItemServiceImpl) GetItemByID(ctx context.Context, itemID string) (*models.Item, error) {
	if err := i.ValidateItemID(itemID); err != nil {
		return nil, err
	}

	// Check cache first
	if item, exists := i.itemCache[itemID]; exists {
		return item, nil
	}

	// If not in cache, it might be a valid format but not generated yet
	// For flash sale, we generate items on-demand if they don't exist
	return i.generateSingleItem(ctx, itemID)
}

// GetAvailableItems returns all available items (from cache)
func (i *ItemServiceImpl) GetAvailableItems(ctx context.Context) ([]models.Item, error) {
	items := make([]models.Item, 0, len(i.itemCache))
	
	for _, item := range i.itemCache {
		items = append(items, *item)
	}

	// If no items are cached, generate a default set
	if len(items) == 0 {
		generatedItems, err := i.GenerateItems(ctx, 1000) // Generate 1000 default items
		if err != nil {
			return nil, fmt.Errorf("failed to generate default items: %w", err)
		}
		return generatedItems, nil
	}

	return items, nil
}

// ValidateItemID checks if an item ID has a valid format
func (i *ItemServiceImpl) ValidateItemID(itemID string) error {
	if itemID == "" {
		return fmt.Errorf("item ID cannot be empty")
	}

	if len(itemID) < 3 || len(itemID) > 50 {
		return fmt.Errorf("item ID length must be between 3 and 50 characters")
	}

	// Check for valid characters (alphanumeric, underscore, hyphen)
	for _, char := range itemID {
		if !((char >= 'a' && char <= 'z') || 
			 (char >= 'A' && char <= 'Z') || 
			 (char >= '0' && char <= '9') || 
			 char == '_' || char == '-') {
			return fmt.Errorf("item ID contains invalid characters: %s", itemID)
		}
	}

	return nil
}

// generateSingleItem creates a single item with the given ID
func (i *ItemServiceImpl) generateSingleItem(ctx context.Context, itemID string) (*models.Item, error) {
	// Extract number from item ID if it follows our pattern
	var itemNumber int = 1
	if strings.HasPrefix(itemID, "item_") {
		// Use hash of ID to generate consistent properties
		hash := simpleHash(itemID)
		itemNumber = int(hash % 10000) // Keep within reasonable range
	}

	// Use item number to select template consistently
	itemTemplates := []struct {
		namePrefix  string
		description string
		basePrice   float64
	}{
		{"Flash Electronics", "High-tech gadget at incredible price", 299.99},
		{"Designer Fashion", "Premium clothing item with limited availability", 149.99},
		{"Home Essential", "Must-have household item for modern living", 79.99},
		{"Sports Gear", "Professional quality sports equipment", 199.99},
		{"Beauty Product", "Premium skincare and cosmetic item", 89.99},
		{"Kitchen Tool", "Essential cooking equipment for every chef", 59.99},
		{"Gaming Accessory", "Professional gaming equipment", 129.99},
		{"Health Supplement", "Premium wellness and health product", 49.99},
		{"Book Collection", "Bestselling books and educational materials", 29.99},
		{"Art Supply", "Professional quality creative materials", 39.99},
	}

	template := itemTemplates[itemNumber%len(itemTemplates)]
	
	// Generate consistent price variation based on item ID
	hash := simpleHash(itemID)
	priceVariation := 0.8 + (float64(hash%40)/100.0) // Between 0.8 and 1.2
	finalPrice := template.basePrice * priceVariation
	finalPrice = float64(int(finalPrice*100)) / 100

	item := &models.Item{
		ID:          itemID,
		Name:        fmt.Sprintf("%s #%d", template.namePrefix, itemNumber),
		Description: template.description,
		Price:       finalPrice,
		CreatedAt:   time.Now(),
	}

	// Cache the generated item
	i.itemCache[itemID] = item

	return item, nil
}

// simpleHash creates a simple hash of a string for consistent randomization
func simpleHash(s string) uint32 {
	var hash uint32 = 0
	for _, char := range s {
		hash = hash*31 + uint32(char)
	}
	return hash
}

// PreloadCommonItems generates and caches commonly requested items
func (i *ItemServiceImpl) PreloadCommonItems(ctx context.Context) error {
	// Generate common item IDs that might be used in testing
	commonItems := []string{
		"item1", "item2", "item3", "item4", "item5",
		"test_item", "demo_item", "sample_item",
		"product_a", "product_b", "product_c",
	}

	for _, itemID := range commonItems {
		if _, exists := i.itemCache[itemID]; !exists {
			_, err := i.generateSingleItem(ctx, itemID)
			if err != nil {
				return fmt.Errorf("failed to preload item %s: %w", itemID, err)
			}
		}
	}

	return nil
}

// GetCacheStats returns statistics about the item cache
func (i *ItemServiceImpl) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"cached_items":    len(i.itemCache),
		"memory_estimate": len(i.itemCache) * 200, // Rough estimate: 200 bytes per item
	}
}

// ClearCache clears the item cache (useful for testing)
func (i *ItemServiceImpl) ClearCache() {
	i.itemCache = make(map[string]*models.Item)
} 