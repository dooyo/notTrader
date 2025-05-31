package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"flash-sale-backend/internal/interfaces"
	"flash-sale-backend/internal/models"

	_ "github.com/lib/pq"
)

// PostgresDB implements DatabaseInterface
type PostgresDB struct {
	db *sql.DB

	// Prepared statements for performance
	getActiveSaleStmt              *sql.Stmt
	createCheckoutAttemptStmt      *sql.Stmt
	getCheckoutByCodeStmt          *sql.Stmt
	updateCheckoutPurchasedStmt    *sql.Stmt
}

// NewPostgresDB creates a new PostgreSQL database connection
func NewPostgresDB(connectionString string) (*PostgresDB, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for high performance
	db.SetMaxOpenConns(100)           // Match expected concurrent requests
	db.SetMaxIdleConns(25)            // Keep connections warm
	db.SetConnMaxLifetime(time.Hour)  // Prevent stale connections
	db.SetConnMaxIdleTime(15 * time.Minute) // Close unused connections

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	pgDB := &PostgresDB{db: db}

	// Prepare statements for performance
	if err := pgDB.prepareStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to prepare statements: %w", err)
	}

	return pgDB, nil
}

func (p *PostgresDB) prepareStatements() error {
	var err error

	// Get active sale statement
	p.getActiveSaleStmt, err = p.db.Prepare(`
		SELECT id, start_time, end_time, items_available, items_sold, active, created_at, updated_at 
		FROM sales 
		WHERE active = true 
		ORDER BY start_time DESC 
		LIMIT 1`)
	if err != nil {
		return fmt.Errorf("failed to prepare getActiveSale statement: %w", err)
	}

	// Create checkout attempt statement
	p.createCheckoutAttemptStmt, err = p.db.Prepare(`
		INSERT INTO checkout_attempts (sale_id, user_id, item_id, code, status, expires_at, created_at) 
		VALUES ($1, $2, $3, $4, $5, $6, NOW()) 
		RETURNING id, created_at`)
	if err != nil {
		return fmt.Errorf("failed to prepare createCheckoutAttempt statement: %w", err)
	}

	// Get checkout by code statement
	p.getCheckoutByCodeStmt, err = p.db.Prepare(`
		SELECT id, sale_id, user_id, item_id, code, status, expires_at, purchased, created_at, updated_at 
		FROM checkout_attempts 
		WHERE code = $1`)
	if err != nil {
		return fmt.Errorf("failed to prepare getCheckoutByCode statement: %w", err)
	}

	// Update checkout purchased statement
	p.updateCheckoutPurchasedStmt, err = p.db.Prepare(`
		UPDATE checkout_attempts 
		SET purchased = true 
		WHERE code = $1 AND purchased = false`)
	if err != nil {
		return fmt.Errorf("failed to prepare updateCheckoutPurchased statement: %w", err)
	}

	return nil
}

// Connection management
func (p *PostgresDB) Close() error {
	// Close prepared statements
	if p.getActiveSaleStmt != nil {
		p.getActiveSaleStmt.Close()
	}
	if p.createCheckoutAttemptStmt != nil {
		p.createCheckoutAttemptStmt.Close()
	}
	if p.getCheckoutByCodeStmt != nil {
		p.getCheckoutByCodeStmt.Close()
	}
	if p.updateCheckoutPurchasedStmt != nil {
		p.updateCheckoutPurchasedStmt.Close()
	}

	return p.db.Close()
}

func (p *PostgresDB) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func (p *PostgresDB) Stats() sql.DBStats {
	return p.db.Stats()
}

// Sale operations
func (p *PostgresDB) CreateSale(ctx context.Context, sale *models.Sale) error {
	query := `
		INSERT INTO sales (start_time, end_time, items_available, items_sold, active, created_at) 
		VALUES ($1, $2, $3, $4, $5, NOW()) 
		RETURNING id, created_at`

	err := p.db.QueryRowContext(ctx, query,
		sale.StartTime, sale.EndTime, sale.ItemsAvailable, sale.ItemsSold, sale.Active).
		Scan(&sale.ID, &sale.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create sale: %w", err)
	}

	return nil
}

func (p *PostgresDB) GetActiveSale(ctx context.Context) (*models.Sale, error) {
	sale := &models.Sale{}

	err := p.getActiveSaleStmt.QueryRowContext(ctx).Scan(
		&sale.ID, &sale.StartTime, &sale.EndTime, &sale.ItemsAvailable,
		&sale.ItemsSold, &sale.Active, &sale.CreatedAt, &sale.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No active sale
		}
		return nil, fmt.Errorf("failed to get active sale: %w", err)
	}

	return sale, nil
}

func (p *PostgresDB) GetSaleByID(ctx context.Context, id int) (*models.Sale, error) {
	query := `
		SELECT id, start_time, end_time, items_available, items_sold, active, created_at, updated_at 
		FROM sales 
		WHERE id = $1`

	sale := &models.Sale{}
	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&sale.ID, &sale.StartTime, &sale.EndTime, &sale.ItemsAvailable,
		&sale.ItemsSold, &sale.Active, &sale.CreatedAt, &sale.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Sale not found
		}
		return nil, fmt.Errorf("failed to get sale by ID: %w", err)
	}

	return sale, nil
}

func (p *PostgresDB) UpdateSaleItemsSold(ctx context.Context, saleID int, itemsSold int) error {
	query := `UPDATE sales SET items_sold = $1 WHERE id = $2`

	result, err := p.db.ExecContext(ctx, query, itemsSold, saleID)
	if err != nil {
		return fmt.Errorf("failed to update sale items sold: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("sale with ID %d not found", saleID)
	}

	return nil
}

func (p *PostgresDB) DeactivateSale(ctx context.Context, saleID int) error {
	query := `UPDATE sales SET active = false WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query, saleID)
	if err != nil {
		return fmt.Errorf("failed to deactivate sale: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("sale with ID %d not found", saleID)
	}

	return nil
}

// Checkout operations
func (p *PostgresDB) CreateCheckoutAttempt(ctx context.Context, attempt *models.CheckoutAttempt) error {
	err := p.createCheckoutAttemptStmt.QueryRowContext(ctx,
		attempt.SaleID, attempt.UserID, attempt.ItemID, attempt.Code, attempt.Status, attempt.ExpiresAt).
		Scan(&attempt.ID, &attempt.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create checkout attempt: %w", err)
	}

	return nil
}

func (p *PostgresDB) GetCheckoutAttemptByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error) {
	attempt := &models.CheckoutAttempt{}

	err := p.getCheckoutByCodeStmt.QueryRowContext(ctx, code).Scan(
		&attempt.ID, &attempt.SaleID, &attempt.UserID, &attempt.ItemID,
		&attempt.Code, &attempt.Status, &attempt.ExpiresAt, &attempt.Purchased, &attempt.CreatedAt, &attempt.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Checkout attempt not found
		}
		return nil, fmt.Errorf("failed to get checkout attempt: %w", err)
	}

	return attempt, nil
}

func (p *PostgresDB) UpdateCheckoutAttemptPurchased(ctx context.Context, code string) error {
	result, err := p.updateCheckoutPurchasedStmt.ExecContext(ctx, code)
	if err != nil {
		return fmt.Errorf("failed to update checkout attempt: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("checkout code %s not found or already purchased", code)
	}

	return nil
}

// Transaction support
func (p *PostgresDB) BeginTx(ctx context.Context) (interfaces.TxInterface, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &PostgresTx{tx: tx}, nil
}

// BeginTransaction is an alias for BeginTx for compatibility
func (p *PostgresDB) BeginTransaction(ctx context.Context) (interfaces.TxInterface, error) {
	return p.BeginTx(ctx)
}

// CreateCheckout is an alias for CreateCheckoutAttempt for compatibility
func (p *PostgresDB) CreateCheckout(ctx context.Context, attempt *models.CheckoutAttempt) error {
	return p.CreateCheckoutAttempt(ctx, attempt)
}

// GetCheckoutByCode is an alias for GetCheckoutAttemptByCode for compatibility
func (p *PostgresDB) GetCheckoutByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error) {
	return p.GetCheckoutAttemptByCode(ctx, code)
}

// CreatePurchase creates a new purchase record
func (p *PostgresDB) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
	query := `
		INSERT INTO purchases (sale_id, user_id, item_id, code, checkout_id, price, status, purchased_at, created_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW()) 
		RETURNING id, created_at`

	err := p.db.QueryRowContext(ctx, query,
		purchase.SaleID, purchase.UserID, purchase.ItemID, purchase.Code,
		purchase.CheckoutID, purchase.Price, purchase.Status, purchase.PurchasedAt).
		Scan(&purchase.ID, &purchase.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create purchase: %w", err)
	}

	return nil
}

// UpdateCheckout updates a checkout record
func (p *PostgresDB) UpdateCheckout(ctx context.Context, checkout *models.CheckoutAttempt) error {
	query := `
		UPDATE checkout_attempts 
		SET status = $2, purchased = $3, updated_at = NOW()
		WHERE id = $1`

	_, err := p.db.ExecContext(ctx, query, checkout.ID, checkout.Status, checkout.Purchased)
	if err != nil {
		return fmt.Errorf("failed to update checkout: %w", err)
	}

	return nil
}

// PostgresTx implements TxInterface
type PostgresTx struct {
	tx *sql.Tx
}

func (t *PostgresTx) Commit() error {
	return t.tx.Commit()
}

func (t *PostgresTx) Rollback() error {
	return t.tx.Rollback()
}

func (t *PostgresTx) CreateCheckoutAttempt(ctx context.Context, attempt *models.CheckoutAttempt) error {
	query := `
		INSERT INTO checkout_attempts (sale_id, user_id, item_id, code, status, expires_at, created_at) 
		VALUES ($1, $2, $3, $4, $5, $6, NOW()) 
		RETURNING id, created_at`

	err := t.tx.QueryRowContext(ctx, query,
		attempt.SaleID, attempt.UserID, attempt.ItemID, attempt.Code, attempt.Status, attempt.ExpiresAt).
		Scan(&attempt.ID, &attempt.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create checkout attempt in transaction: %w", err)
	}

	return nil
}

func (t *PostgresTx) GetCheckoutAttemptByCode(ctx context.Context, code string) (*models.CheckoutAttempt, error) {
	query := `
		SELECT id, sale_id, user_id, item_id, code, status, expires_at, purchased, created_at, updated_at 
		FROM checkout_attempts 
		WHERE code = $1 FOR UPDATE`

	attempt := &models.CheckoutAttempt{}
	err := t.tx.QueryRowContext(ctx, query, code).Scan(
		&attempt.ID, &attempt.SaleID, &attempt.UserID, &attempt.ItemID,
		&attempt.Code, &attempt.Status, &attempt.ExpiresAt, &attempt.Purchased, &attempt.CreatedAt, &attempt.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Checkout attempt not found
		}
		return nil, fmt.Errorf("failed to get checkout attempt in transaction: %w", err)
	}

	return attempt, nil
}

func (t *PostgresTx) UpdateCheckoutAttemptPurchased(ctx context.Context, code string) error {
	query := `
		UPDATE checkout_attempts 
		SET purchased = true 
		WHERE code = $1 AND purchased = false`

	result, err := t.tx.ExecContext(ctx, query, code)
	if err != nil {
		return fmt.Errorf("failed to update checkout attempt in transaction: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("checkout code %s not found or already purchased", code)
	}

	return nil
}

