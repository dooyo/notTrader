package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"flash-sale-backend/internal/models"
	"flash-sale-backend/internal/interfaces"
)

// RedisClient implements RedisInterface
type RedisClient struct {
	client *redis.Client

	// Lua scripts for atomic operations
	atomicPurchaseScript *redis.Script
	validateCodeScript   *redis.Script
	setupSaleScript      *redis.Script
}

// Lua script for atomic purchase with inventory and user limit checks
const atomicPurchaseLua = `
	local sale_key = "sale:" .. ARGV[1] .. ":sold"
	local user_key = "user:" .. ARGV[2] .. ":sale:" .. ARGV[1] .. ":count"
	local max_items = tonumber(ARGV[3])
	local max_user_items = tonumber(ARGV[4])
	
	-- Get current values
	local sold = tonumber(redis.call('GET', sale_key) or 0)
	local user_count = tonumber(redis.call('GET', user_key) or 0)
	
	-- Check global inventory limit
	if sold >= max_items then
		return {0, "sale_sold_out", sold, user_count}
	end
	
	-- Check user purchase limit
	if user_count >= max_user_items then
		return {0, "user_limit_exceeded", sold, user_count}
	end
	
	-- Atomic increment both counters
	local new_sold = redis.call('INCR', sale_key)
	local new_user_count = redis.call('INCR', user_key)
	
	-- Set expiration for both keys (24 hours)
	redis.call('EXPIRE', sale_key, 86400)
	redis.call('EXPIRE', user_key, 86400)
	
	return {1, "success", new_sold, new_user_count}
`

// Lua script for validating and consuming checkout codes
const validateCodeLua = `
	local code_key = "checkout:" .. ARGV[1]
	local exists = redis.call('EXISTS', code_key)
	
	if exists == 0 then
		return {0, "code_not_found", "", "", ""}
	end
	
	-- Get checkout data
	local data = redis.call('HMGET', code_key, 'sale_id', 'user_id', 'item_id', 'used')
	local sale_id = data[1]
	local user_id = data[2]  
	local item_id = data[3]
	local used = data[4]
	
	if used == "true" then
		return {0, "code_already_used", sale_id, user_id, item_id}
	end
	
	-- Mark as used
	redis.call('HSET', code_key, 'used', 'true')
	
	return {1, "success", sale_id, user_id, item_id}
`

// Lua script for setting up sale counters
const setupSaleLua = `
	local sale_id = ARGV[1]
	local items_available = tonumber(ARGV[2])
	
	-- Set up sale counters
	redis.call('SET', "sale:" .. sale_id .. ":sold", 0)
	redis.call('SET', "sale:" .. sale_id .. ":available", items_available)
	redis.call('SET', "active_sale_id", sale_id)
	
	-- Set expiration (24 hours)
	redis.call('EXPIRE', "sale:" .. sale_id .. ":sold", 86400)
	redis.call('EXPIRE', "sale:" .. sale_id .. ":available", 86400)
	redis.call('EXPIRE', "active_sale_id", 86400)
	
	-- Cache sale info
	redis.call('HMSET', "sale:" .. sale_id .. ":cache", 
		"id", sale_id,
		"available", items_available,
		"sold", 0,
		"active", "true")
	redis.call('EXPIRE', "sale:" .. sale_id .. ":cache", 3600)
	
	return "OK"
`

// NewRedisClient creates a new Redis client connection
func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		
		// High-performance settings for flash sale load
		PoolSize:     100,              // Match PostgreSQL pool size
		MinIdleConns: 25,               // Keep connections warm
		MaxRetries:   3,                // Automatic retry on failure
		DialTimeout:  5 * time.Second,  // Connection timeout
		ReadTimeout:  2 * time.Second,  // Read timeout
		WriteTimeout: 2 * time.Second,  // Write timeout
		PoolTimeout:  4 * time.Second,  // Pool get timeout
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	redisClient := &RedisClient{
		client: client,
		atomicPurchaseScript: redis.NewScript(atomicPurchaseLua),
		validateCodeScript:   redis.NewScript(validateCodeLua),
		setupSaleScript:      redis.NewScript(setupSaleLua),
	}

	return redisClient, nil
}

// Connection management
func (r *RedisClient) Close() error {
	return r.client.Close()
}

func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Atomic sale operations
func (r *RedisClient) AtomicPurchase(ctx context.Context, saleID int, userID string, maxItems, maxUserItems int) (bool, string, int, int, error) {
	result, err := r.atomicPurchaseScript.Run(ctx, r.client, 
		[]string{}, saleID, userID, maxItems, maxUserItems).Result()
	
	if err != nil {
		return false, "", 0, 0, fmt.Errorf("atomic purchase script failed: %w", err)
	}

	res := result.([]interface{})
	success := res[0].(int64) == 1
	message := res[1].(string)
	sold := int(res[2].(int64))
	userCount := int(res[3].(int64))

	return success, message, sold, userCount, nil
}

func (r *RedisClient) GetSoldItems(ctx context.Context, saleID int) (int, error) {
	key := fmt.Sprintf("sale:%d:sold", saleID)
	result, err := r.client.Get(ctx, key).Result()
	
	if err != nil {
		if err == redis.Nil {
			return 0, nil // No items sold yet
		}
		return 0, fmt.Errorf("failed to get sold items: %w", err)
	}

	sold, err := strconv.Atoi(result)
	if err != nil {
		return 0, fmt.Errorf("invalid sold items value: %w", err)
	}

	return sold, nil
}

func (r *RedisClient) GetUserPurchaseCount(ctx context.Context, userID string, saleID int) (int, error) {
	key := fmt.Sprintf("user:%s:sale:%d:count", userID, saleID)
	result, err := r.client.Get(ctx, key).Result()
	
	if err != nil {
		if err == redis.Nil {
			return 0, nil // User has no purchases
		}
		return 0, fmt.Errorf("failed to get user purchase count: %w", err)
	}

	count, err := strconv.Atoi(result)
	if err != nil {
		return 0, fmt.Errorf("invalid user purchase count: %w", err)
	}

	return count, nil
}

// Sale management
func (r *RedisClient) SetupSale(ctx context.Context, saleID int, itemsAvailable int) error {
	_, err := r.setupSaleScript.Run(ctx, r.client, 
		[]string{}, saleID, itemsAvailable).Result()
	
	if err != nil {
		return fmt.Errorf("setup sale script failed: %w", err)
	}

	return nil
}

func (r *RedisClient) GetActiveSaleID(ctx context.Context) (int, error) {
	result, err := r.client.Get(ctx, "active_sale_id").Result()
	
	if err != nil {
		if err == redis.Nil {
			return 0, nil // No active sale
		}
		return 0, fmt.Errorf("failed to get active sale ID: %w", err)
	}

	saleID, err := strconv.Atoi(result)
	if err != nil {
		return 0, fmt.Errorf("invalid active sale ID: %w", err)
	}

	return saleID, nil
}

func (r *RedisClient) SetActiveSaleID(ctx context.Context, saleID int) error {
	err := r.client.Set(ctx, "active_sale_id", saleID, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to set active sale ID: %w", err)
	}

	return nil
}

// Checkout code management
func (r *RedisClient) CacheCheckoutCode(ctx context.Context, code string, saleID int, userID string, itemID string) error {
	key := fmt.Sprintf("checkout:%s", code)
	
	// Store checkout data as hash
	data := map[string]interface{}{
		"sale_id": saleID,
		"user_id": userID,
		"item_id": itemID,
		"used":    "false",
		"created": time.Now().Unix(),
	}

	err := r.client.HMSet(ctx, key, data).Err()
	if err != nil {
		return fmt.Errorf("failed to cache checkout code: %w", err)
	}

	// Set expiration (1 hour)
	err = r.client.Expire(ctx, key, time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to set checkout code expiration: %w", err)
	}

	return nil
}

func (r *RedisClient) GetCheckoutData(ctx context.Context, code string) (saleID int, userID string, itemID string, err error) {
	key := fmt.Sprintf("checkout:%s", code)
	
	result, err := r.client.HMGet(ctx, key, "sale_id", "user_id", "item_id", "used").Result()
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to get checkout data: %w", err)
	}

	// Check if key exists
	if result[0] == nil {
		return 0, "", "", fmt.Errorf("checkout code not found")
	}

	// Check if already used
	if result[3] == "true" {
		return 0, "", "", fmt.Errorf("checkout code already used")
	}

	// Parse sale ID
	saleIDStr, ok := result[0].(string)
	if !ok {
		return 0, "", "", fmt.Errorf("invalid sale ID in checkout data")
	}
	
	saleID, err = strconv.Atoi(saleIDStr)
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to parse sale ID: %w", err)
	}

	// Get user ID and item ID
	userID, ok = result[1].(string)
	if !ok {
		return 0, "", "", fmt.Errorf("invalid user ID in checkout data")
	}

	itemID, ok = result[2].(string)
	if !ok {
		return 0, "", "", fmt.Errorf("invalid item ID in checkout data")
	}

	return saleID, userID, itemID, nil
}

func (r *RedisClient) InvalidateCheckoutCode(ctx context.Context, code string) error {
	key := fmt.Sprintf("checkout:%s", code)
	
	err := r.client.HSet(ctx, key, "used", "true").Err()
	if err != nil {
		return fmt.Errorf("failed to invalidate checkout code: %w", err)
	}

	return nil
}

// Performance metrics
func (r *RedisClient) GetConnectionStats() interface{} {
	return r.client.PoolStats()
}

// Additional utility methods for performance monitoring
func (r *RedisClient) FlushTestData(ctx context.Context) error {
	// Only use in testing - clears test keys
	pattern := "sale:*"
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return r.client.Del(ctx, keys...).Err()
	}

	pattern = "user:*"
	keys, err = r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return r.client.Del(ctx, keys...).Err()
	}

	pattern = "checkout:*"
	keys, err = r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return r.client.Del(ctx, keys...).Err()
	}

	return r.client.Del(ctx, "active_sale_id").Err()
}

// Pipeline operations for batch updates
func (r *RedisClient) BatchSetUserCounts(ctx context.Context, saleID int, userCounts map[string]int) error {
	pipe := r.client.Pipeline()

	for userID, count := range userCounts {
		key := fmt.Sprintf("user:%s:sale:%d:count", userID, saleID)
		pipe.Set(ctx, key, count, 24*time.Hour)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to batch set user counts: %w", err)
	}

	return nil
}

// Health check with detailed Redis info
func (r *RedisClient) HealthCheck(ctx context.Context) map[string]interface{} {
	health := make(map[string]interface{})

	// Basic connectivity
	if err := r.client.Ping(ctx).Err(); err != nil {
		health["status"] = "unhealthy"
		health["error"] = err.Error()
		return health
	}

	health["status"] = "healthy"

	// Connection pool stats
	stats := r.client.PoolStats()
	health["pool_stats"] = map[string]interface{}{
		"hits":         stats.Hits,
		"misses":       stats.Misses,
		"timeouts":     stats.Timeouts,
		"total_conns":  stats.TotalConns,
		"idle_conns":   stats.IdleConns,
		"stale_conns":  stats.StaleConns,
	}

	// Memory usage
	info, err := r.client.Info(ctx, "memory").Result()
	if err == nil {
		health["memory_info"] = info
	}

	// Key count estimation
	dbSize, err := r.client.DBSize(ctx).Result()
	if err == nil {
		health["total_keys"] = dbSize
	}

	return health
}

// SetCheckoutCode is an alias for CacheCheckoutCode for compatibility
func (r *RedisClient) SetCheckoutCode(ctx context.Context, code string, saleID int, userID string, itemID string) error {
	return r.CacheCheckoutCode(ctx, code, saleID, userID, itemID)
}

// GetCheckoutCode retrieves checkout data and returns as a Checkout model for compatibility
func (r *RedisClient) GetCheckoutCode(ctx context.Context, code string) (*models.Checkout, error) {
	saleID, userID, itemID, err := r.GetCheckoutData(ctx, code)
	if err != nil {
		return nil, err
	}
	
	// Create a checkout model from the cached data
	checkout := &models.Checkout{
		Code:   code,
		SaleID: saleID,
		UserID: userID,
		ItemID: itemID,
		Status: "pending", // Default status for cached codes
	}
	
	return checkout, nil
}

// AttemptPurchase performs an atomic purchase operation and returns a PurchaseResult
func (r *RedisClient) AttemptPurchase(ctx context.Context, saleID int, userID string, itemID string) (*interfaces.PurchaseResult, error) {
	// Use the existing AtomicPurchase method with default limits
	success, message, totalSold, userPurchases, err := r.AtomicPurchase(ctx, saleID, userID, 10000, 10)
	if err != nil {
		return nil, err
	}
	
	// Convert to PurchaseResult format
	result := &interfaces.PurchaseResult{
		Status:        message,
		UserPurchases: userPurchases,
		TotalSold:     totalSold,
		ItemID:        itemID,
	}
	
	if success {
		result.Status = "success"
	}
	
	return result, nil
} 