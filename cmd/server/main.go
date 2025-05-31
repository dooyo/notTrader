package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flash-sale-backend/internal/database"
	"flash-sale-backend/internal/handlers"
	"flash-sale-backend/internal/services"
)

// getEnv returns environment variable value or default if not set
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	ctx := context.Background()
	
	// Configuration from environment variables
	postgresURL := getEnv("POSTGRES_URL", "postgres://postgres:password@localhost:5432/flashsale?sslmode=disable")
	redisURL := getEnv("REDIS_URL", "localhost:6379")
	serverPort := getEnv("SERVER_PORT", "8080")
	
	log.Printf("Starting with configuration:")
	log.Printf("  PostgreSQL: %s", postgresURL)
	log.Printf("  Redis: %s", redisURL)
	log.Printf("  Server Port: %s", serverPort)
	
	// Initialize database connections
	log.Println("Initializing PostgreSQL connection...")
	pgDB, err := database.NewPostgresDB(postgresURL)
	if err != nil {
		log.Printf("Warning: PostgreSQL connection failed: %v", err)
		log.Println("Server will start but database operations will fail until PostgreSQL is available")
		// Don't exit - allow server to start for basic testing
	}
	
	log.Println("Initializing Redis connection...")
	redisClient, err := database.NewRedisClient(redisURL, "", 0)
	if err != nil {
		log.Printf("Warning: Redis connection failed: %v", err)
		log.Println("Server will start but Redis operations will fail until Redis is available")
		// Don't exit - allow server to start for basic testing
	}

	// Initialize services
	log.Println("Initializing services...")
	saleService := services.NewSaleService(pgDB, redisClient)
	itemService := services.NewItemService()
	
	// Preload common items for testing
	if err := itemService.PreloadCommonItems(ctx); err != nil {
		log.Printf("Warning: Failed to preload common items: %v", err)
	}

	// Initialize handlers
	log.Println("Initializing handlers...")
	healthHandler := handlers.NewHealthHandler()
	checkoutHandler := handlers.NewCheckoutHandler(saleService, itemService, pgDB, redisClient)
	purchaseHandler := handlers.NewPurchaseHandler(saleService, itemService, pgDB, redisClient)

	// Setup HTTP routes
	mux := http.NewServeMux()
	
	// Health check endpoint
	mux.HandleFunc("/health", healthHandler.HandleHealth)
	
	// API endpoints
	mux.HandleFunc("/checkout", checkoutHandler.HandleCheckout)
	mux.HandleFunc("/purchase", purchaseHandler.HandlePurchase)
	
	// Root endpoint with API information
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"service": "flash-sale-backend",
			"version": "1.0.0",
			"endpoints": {
				"health": "GET /health",
				"checkout": "POST /checkout",
				"purchase": "POST /purchase"
			},
			"status": "running"
		}`))
	})

	// Configure HTTP server
	server := &http.Server{
		Addr:              ":" + serverPort,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// Start background sale manager if database is available
	if pgDB != nil && redisClient != nil {
		log.Println("Starting background sale manager...")
		saleManager := services.NewBackgroundSaleManager(saleService)
		go saleManager.Start(ctx)
		
		// Ensure manager stops when server shuts down
		defer saleManager.Stop()
	} else {
		log.Println("Skipping background sale manager (database not available)")
	}

	// Start server in goroutine
	go func() {
		log.Printf("Flash sale server starting on :%s", serverPort)
		log.Println("Available endpoints:")
		log.Println("  GET  /        - API information")
		log.Println("  GET  /health  - Health check")
		log.Println("  POST /checkout - Create checkout code")
		log.Println("  POST /purchase - Complete purchase")
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with 30 second timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	// Close database connections
	if pgDB != nil {
		log.Println("Closing PostgreSQL connection...")
		pgDB.Close()
	}
	
	if redisClient != nil {
		log.Println("Closing Redis connection...")
		redisClient.Close()
	}

	log.Println("Server exited")
} 