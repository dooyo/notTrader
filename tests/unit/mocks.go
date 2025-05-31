package unit

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"flash-sale-backend/internal/interfaces"
	"flash-sale-backend/internal/models"
)

// MockSaleService implements interfaces.SaleService
type MockSaleService struct {
	currentSale *models.Sale
	shouldError bool
	sales       map[int]*models.Sale
	nextSaleID  int
	mu          sync.RWMutex
}

func NewMockSaleService() *MockSaleService {
	return &MockSaleService{
		sales:      make(map[int]*models.Sale),
		nextSaleID: 1,
	}
}

func (m *MockSaleService) CreateHourlySale(ctx context.Context) (*models.Sale, error) {
	if m.shouldError {
		return nil, errors.New("mock sale service error")
	}
	sale := &models.Sale{
		ID:             m.nextSaleID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
		ItemsAvailable: 10000,
		ItemsSold:      0,
		Active:         true,
		CreatedAt:      time.Now(),
	}
	m.sales[sale.ID] = sale
	m.nextSaleID++
	return sale, nil
}

func (m *MockSaleService) GetCurrentActiveSale(ctx context.Context) (*models.Sale, error) {
	if m.shouldError {
		return nil, errors.New("mock sale service error")
	}
	return m.currentSale, nil
}

func (m *MockSaleService) ActivateSale(ctx context.Context, saleID int) error {
	if m.shouldError {
		return errors.New("mock sale service error")
	}
	if sale, exists := m.sales[saleID]; exists {
		sale.Active = true
	}
	return nil
}

func (m *MockSaleService) DeactivateSale(ctx context.Context, saleID int) error {
	if m.shouldError {
		return errors.New("mock sale service error")
	}
	if sale, exists := m.sales[saleID]; exists {
		sale.Active = false
	}
	return nil
}

func (m *MockSaleService) GetSaleStatus(ctx context.Context, saleID int) (*models.Sale, error) {
	if m.shouldError {
		return nil, errors.New("mock sale service error")
	}
	sale, exists := m.sales[saleID]
	if !exists {
		return nil, nil
	}
	return sale, nil
}

func (m *MockSaleService) GetSaleItemsSold(ctx context.Context, saleID int) (int, error) {
	if m.shouldError {
		return 0, errors.New("mock sale service error")
	}
	if sale, exists := m.sales[saleID]; exists {
		return sale.ItemsSold, nil
	}
	return 0, nil
}

// Helper method for load testing
func (m *MockSaleService) SetCurrentSale(sale *models.Sale) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentSale = sale
}

// MockItemService implements interfaces.ItemService
type MockItemService struct {
	items       map[string]*models.Item
	shouldError bool
}

func NewMockItemService() *MockItemService {
	return &MockItemService{
		items: make(map[string]*models.Item),
	}
}

func (m *MockItemService) GenerateItems(ctx context.Context, count int) ([]models.Item, error) {
	if m.shouldError {
		return nil, errors.New("mock item service error")
	}
	items := make([]models.Item, count)
	for i := 0; i < count; i++ {
		items[i] = models.Item{
			ID:          "item" + string(rune('1'+i)),
			Name:        "Test Item " + string(rune('1'+i)),
			Description: "Test Description",
			Price:       99.99,
			CreatedAt:   time.Now(),
		}
	}
	return items, nil
}

func (m *MockItemService) GetItemByID(ctx context.Context, itemID string) (*models.Item, error) {
	if m.shouldError {
		return nil, errors.New("mock item service error")
	}
	item, exists := m.items[itemID]
	if !exists {
		return nil, errors.New("item not found")
	}
	return item, nil
}

func (m *MockItemService) GetAvailableItems(ctx context.Context) ([]models.Item, error) {
	if m.shouldError {
		return nil, errors.New("mock item service error")
	}
	items := make([]models.Item, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, *item)
	}
	return items, nil
}

func (m *MockItemService) ValidateItemID(itemID string) error {
	if itemID == "" || len(itemID) > 50 {
		return errors.New("invalid item ID")
	}
	return nil
}

// Helper method for load testing
func (m *MockItemService) AddItem(id string, item *models.Item) {
	m.items[id] = item
}

// MockDatabaseInterface implements interfaces.DatabaseInterface
type MockDatabaseInterface struct {
	sales        map[int]*models.Sale
	checkouts    map[string]*models.CheckoutAttempt
	userCounts   map[string]*models.UserSaleCount
	purchases    map[int]*models.Purchase
	shouldError  bool
	nextSaleID   int
	nextPurchaseID int
	mu           sync.RWMutex
}

func NewMockDatabase() *MockDatabaseInterface {
	return &MockDatabaseInterface{
		sales:      make(map[int]*models.Sale),
		checkouts:  make(map[string]*models.CheckoutAttempt),
		userCounts: make(map[string]*models.UserSaleCount),
		purchases:  make(map[int]*models.Purchase),
		nextSaleID: 1,
		nextPurchaseID: 1,
	}
}

// Connection management
func (m *MockDatabaseInterface) Close() error { return nil }
func (m *MockDatabaseInterface) Ping(ctx context.Context) error { return nil }
func (m *MockDatabaseInterface) Stats() sql.DBStats { return sql.DBStats{} }

// Sale operations
func (m *MockDatabaseInterface) CreateSale(ctx context.Context, sale *models.Sale) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	sale.ID = m.nextSaleID
	sale.CreatedAt = time.Now()
	m.sales[sale.ID] = sale
	m.nextSaleID++
	return nil
}

func (m *MockDatabaseInterface) GetActiveSale(ctx context.Context) (*models.Sale, error) {
	if m.shouldError {
		return nil, errors.New("mock database error")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sale := range m.sales {
		if sale.Active {
			return sale, nil
		}
	}
	return nil, nil
}

func (m *MockDatabaseInterface) GetSaleByID(ctx context.Context, id int) (*models.Sale, error) {
	if m.shouldError {
		return nil, errors.New("mock database error")
	}
	sale, exists := m.sales[id]
	if !exists {
		return nil, nil
	}
	return sale, nil
}

func (m *MockDatabaseInterface) UpdateSaleItemsSold(ctx context.Context, saleID int, itemsSold int) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	if sale, exists := m.sales[saleID]; exists {
		sale.ItemsSold = itemsSold
	}
	return nil
}

func (m *MockDatabaseInterface) DeactivateSale(ctx context.Context, saleID int) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	if sale, exists := m.sales[saleID]; exists {
		sale.Active = false
	}
	return nil
}

// Checkout operations
func (m *MockDatabaseInterface) CreateCheckoutAttempt(ctx context.Context, attempt *models.CheckoutAttempt) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt.ID = len(m.checkouts) + 1
	attempt.CreatedAt = time.Now()
	m.checkouts[attempt.Code] = attempt
	return nil
}

func (m *MockDatabaseInterface) GetCheckoutAttemptByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error) {
	if m.shouldError {
		return nil, errors.New("mock database error")
	}
	checkout, exists := m.checkouts[code]
	if !exists {
		return nil, nil
	}
	return checkout, nil
}

func (m *MockDatabaseInterface) UpdateCheckoutAttemptPurchased(ctx context.Context, code string) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	if checkout, exists := m.checkouts[code]; exists {
		checkout.Purchased = true
	}
	return nil
}

// Compatibility aliases
func (m *MockDatabaseInterface) CreateCheckout(ctx context.Context, attempt *models.CheckoutAttempt) error {
	return m.CreateCheckoutAttempt(ctx, attempt)
}

func (m *MockDatabaseInterface) GetCheckoutByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error) {
	return m.GetCheckoutAttemptByCode(ctx, code)
}

// User purchase tracking
func (m *MockDatabaseInterface) GetUserSaleCount(ctx context.Context, userID string, saleID int) (*models.UserSaleCount, error) {
	if m.shouldError {
		return nil, errors.New("mock database error")
	}
	key := userID + "_" + string(rune(saleID))
	count, exists := m.userCounts[key]
	if !exists {
		return nil, nil
	}
	return count, nil
}

func (m *MockDatabaseInterface) IncrementUserSaleCount(ctx context.Context, userID string, saleID int) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	key := userID + "_" + string(rune(saleID))
	if count, exists := m.userCounts[key]; exists {
		count.PurchaseCount++
	}
	return nil
}

func (m *MockDatabaseInterface) CreateUserSaleCount(ctx context.Context, userID string, saleID int) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	key := userID + "_" + string(rune(saleID))
	m.userCounts[key] = &models.UserSaleCount{
		UserID:        userID,
		SaleID:        saleID,
		PurchaseCount: 1,
		CreatedAt:     time.Now(),
	}
	return nil
}

// Purchase operations
func (m *MockDatabaseInterface) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	purchase.ID = m.nextPurchaseID
	purchase.CreatedAt = time.Now()
	m.purchases[purchase.ID] = purchase
	m.nextPurchaseID++
	return nil
}

func (m *MockDatabaseInterface) UpdateCheckout(ctx context.Context, checkout *models.CheckoutAttempt) error {
	if m.shouldError {
		return errors.New("mock database error")
	}
	if existing, exists := m.checkouts[checkout.Code]; exists {
		existing.Status = checkout.Status
		existing.Purchased = checkout.Purchased
		existing.UpdatedAt = time.Now()
	}
	return nil
}

// Transaction support
func (m *MockDatabaseInterface) BeginTx(ctx context.Context) (interfaces.TxInterface, error) {
	if m.shouldError {
		return nil, errors.New("mock database error")
	}
	return &MockTx{db: m}, nil
}

func (m *MockDatabaseInterface) BeginTransaction(ctx context.Context) (interfaces.TxInterface, error) {
	return m.BeginTx(ctx)
}

// MockTx implements interfaces.TxInterface
type MockTx struct {
	db *MockDatabaseInterface
}

func (t *MockTx) Commit() error { return nil }
func (t *MockTx) Rollback() error { return nil }

func (t *MockTx) CreateCheckoutAttempt(ctx context.Context, attempt *models.CheckoutAttempt) error {
	return t.db.CreateCheckoutAttempt(ctx, attempt)
}

func (t *MockTx) GetCheckoutAttemptByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error) {
	return t.db.GetCheckoutAttemptByCode(ctx, code)
}

func (t *MockTx) UpdateCheckoutAttemptPurchased(ctx context.Context, code string) error {
	return t.db.UpdateCheckoutAttemptPurchased(ctx, code)
}

func (t *MockTx) GetUserSaleCount(ctx context.Context, userID string, saleID int) (*models.UserSaleCount, error) {
	return t.db.GetUserSaleCount(ctx, userID, saleID)
}

func (t *MockTx) IncrementUserSaleCount(ctx context.Context, userID string, saleID int) error {
	return t.db.IncrementUserSaleCount(ctx, userID, saleID)
}

// MockRedisInterface implements interfaces.RedisInterface
type MockRedisInterface struct {
	checkoutCodes map[string]bool
	userCounts    map[string]int
	soldItems     map[int]int
	shouldError   bool
	mu            sync.RWMutex
}

func NewMockRedis() *MockRedisInterface {
	return &MockRedisInterface{
		checkoutCodes: make(map[string]bool),
		userCounts:    make(map[string]int),
		soldItems:     make(map[int]int),
	}
}

// Connection management
func (m *MockRedisInterface) Close() error { return nil }
func (m *MockRedisInterface) Ping(ctx context.Context) error { return nil }

// Atomic sale operations
func (m *MockRedisInterface) AtomicPurchase(ctx context.Context, saleID int, userID string, maxItems, maxUserItems int) (bool, string, int, int, error) {
	if m.shouldError {
		return false, "error", 0, 0, errors.New("mock redis error")
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	userKey := userID + "_" + string(rune(saleID))
	userCount := m.userCounts[userKey]
	
	if userCount >= maxUserItems {
		return false, "user_limit_exceeded", m.soldItems[saleID], userCount, nil
	}
	
	if m.soldItems[saleID] >= maxItems {
		return false, "sold_out", m.soldItems[saleID], userCount, nil
	}
	
	m.soldItems[saleID]++
	m.userCounts[userKey]++
	
	return true, "success", m.soldItems[saleID], m.userCounts[userKey], nil
}

func (m *MockRedisInterface) GetSoldItems(ctx context.Context, saleID int) (int, error) {
	if m.shouldError {
		return 0, errors.New("mock redis error")
	}
	return m.soldItems[saleID], nil
}

func (m *MockRedisInterface) GetUserPurchaseCount(ctx context.Context, userID string, saleID int) (int, error) {
	if m.shouldError {
		return 0, errors.New("mock redis error")
	}
	userKey := userID + "_" + string(rune(saleID))
	return m.userCounts[userKey], nil
}

// Sale management
func (m *MockRedisInterface) SetupSale(ctx context.Context, saleID int, itemsAvailable int) error {
	if m.shouldError {
		return errors.New("mock redis error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.soldItems[saleID] = 0
	return nil
}

func (m *MockRedisInterface) GetActiveSaleID(ctx context.Context) (int, error) {
	if m.shouldError {
		return 0, errors.New("mock redis error")
	}
	return 1, nil
}

func (m *MockRedisInterface) SetActiveSaleID(ctx context.Context, saleID int) error {
	if m.shouldError {
		return errors.New("mock redis error")
	}
	return nil
}

// Checkout code management
func (m *MockRedisInterface) CacheCheckoutCode(ctx context.Context, code string, saleID int, userID string, itemID string) error {
	if m.shouldError {
		return errors.New("mock redis error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkoutCodes[code] = true
	return nil
}

func (m *MockRedisInterface) GetCheckoutData(ctx context.Context, code string) (saleID int, userID string, itemID string, err error) {
	if m.shouldError {
		return 0, "", "", errors.New("mock redis error")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.checkoutCodes[code] {
		return 0, "", "", errors.New("checkout code not found")
	}
	return 1, "user1", "item1", nil
}

func (m *MockRedisInterface) InvalidateCheckoutCode(ctx context.Context, code string) error {
	if m.shouldError {
		return errors.New("mock redis error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.checkoutCodes, code)
	return nil
}

// Compatibility aliases
func (m *MockRedisInterface) SetCheckoutCode(ctx context.Context, code string, saleID int, userID string, itemID string) error {
	return m.CacheCheckoutCode(ctx, code, saleID, userID, itemID)
}

func (m *MockRedisInterface) GetCheckoutCode(ctx context.Context, code string) (*models.Checkout, error) {
	if m.shouldError {
		return nil, errors.New("mock redis error")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.checkoutCodes[code] {
		return nil, errors.New("checkout code not found")
	}
	return &models.Checkout{
		Code:   code,
		SaleID: 1,
		UserID: "user1",
		ItemID: "item1",
		Status: "pending",
	}, nil
}

func (m *MockRedisInterface) AttemptPurchase(ctx context.Context, saleID int, userID string, itemID string) (*interfaces.PurchaseResult, error) {
	if m.shouldError {
		return nil, errors.New("mock redis error")
	}
	_, status, totalSold, userPurchases, err := m.AtomicPurchase(ctx, saleID, userID, 10000, 10)
	if err != nil {
		return nil, err
	}
	
	return &interfaces.PurchaseResult{
		Status:        status,
		UserPurchases: userPurchases,
		TotalSold:     totalSold,
		ItemID:        itemID,
	}, nil
}

// Performance metrics
func (m *MockRedisInterface) GetConnectionStats() interface{} {
	return map[string]interface{}{"mock": "stats"}
} 