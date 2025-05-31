# Flash Sale Backend

A high-performance flash sale system built in Go for selling exactly 10,000 items per hour with atomic operations and race condition prevention.

## 🚀 Quick Start

### Prerequisites
- Go 1.20+
- Docker Desktop
- Git

### 1. Setup Databases

```bash
# Start PostgreSQL and Redis
docker-compose up -d

# Wait for databases to be ready (check with)
docker ps
```

### 2. Start the Server

```bash
go run cmd/server/main.go
```

You should see output like:
```
2025/05/31 14:58:53 Initializing PostgreSQL connection...
2025/05/31 14:58:53 Initializing Redis connection...
2025/05/31 14:58:53 Starting background sale manager...
2025/05/31 14:58:53 Flash sale server starting on :8080
2025/05/31 14:58:53 Found active sale 1
```

### 3. Test the API

**Health Check**
```bash
curl http://localhost:8080/health
```

**API Information**
```bash
curl http://localhost:8080/
```

**Create Checkout Code**
```bash
curl -X POST "http://localhost:8080/checkout?user_id=user1&item_id=item1"
```

**Complete Purchase**
```bash
curl -X POST "http://localhost:8080/purchase" \
  -H "Content-Type: application/json" \
  -d '{"checkout_code": "CHK_219b6c54_5175"}'
```

## 📖 API Documentation

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/` | API information |
| POST | `/checkout` | Create checkout code |
| POST | `/purchase` | Complete purchase |

### POST /checkout

Creates a checkout code for a user to purchase an item.

**Query Parameters:**
- `user_id` (string, required): Unique user identifier
- `item_id` (string, required): Item identifier to purchase

**Response:**
```json
{
  "success": true,
  "checkout_code": "CHK_abc123_456",
  "message": "Checkout code generated successfully",
  "expires_at": "2025-05-31T15:09:05Z",
  "item": {
    "id": "item1",
    "name": "Item Name",
    "description": "Item description",
    "price": 99.99
  }
}
```

### POST /purchase

Completes a purchase using a checkout code.

**Request Body:**
```json
{
  "checkout_code": "CHK_abc123_456"
}
```

**Response:**
```json
{
  "success": true,
  "purchase_id": 123,
  "message": "Purchase completed successfully",
  "item": {
    "id": "item1",
    "name": "Item Name",
    "price": 99.99
  },
  "total_price": 99.99,
  "purchased_at": "2025-05-31T15:09:05Z",
  "user_purchases": 1
}
```

## 🏗️ Architecture

### System Components

- **Go HTTP Server**: Standard library only, optimized for performance
- **PostgreSQL**: Data persistence with optimized indexes
- **Redis**: Atomic operations and caching with Lua scripts
- **Background Sale Manager**: Automatic hourly sale creation

### Key Features

- ✅ **Exactly 10,000 items per sale** (enforced atomically)
- ✅ **Maximum 10 items per user** (enforced per sale)
- ✅ **Race condition prevention** (Redis Lua scripts)
- ✅ **All checkout attempts persisted**
- ✅ **Graceful degradation** (works without databases for testing)
- ✅ **Minimal dependencies** (only 3 external packages)

### Database Schema

**Sales Table:**
- Tracks hourly flash sale sessions
- Automatic activation/deactivation
- Performance indexes for active sale lookups

**Checkout Attempts Table:**
- Persists all checkout requests
- Unique code generation with UUID
- Expiration tracking (10 minutes)

**User Sale Counts Table:**
- Enforces per-user purchase limits
- Real-time tracking with Redis backup

**Purchases Table:**
- Completed transaction records
- Price and status tracking

## 🔧 Configuration

### Database Connections

**PostgreSQL:**
```
Host: localhost:5432
Database: flashsale
User: postgres
Password: password
```

**Redis:**
```
Host: localhost:6379
Database: 0 (default)
No authentication
```

### Environment Variables

The application currently uses hardcoded connection strings for simplicity. In production, use environment variables:

```bash
export POSTGRES_URL="postgres://user:password@host:port/database"
export REDIS_URL="redis://host:port"
```

## 🧪 Testing

### Load Testing

```bash
# Install Artillery.js
npm install -g artillery

# Create basic load test
cat > load-test.yml << EOF
config:
  target: 'http://localhost:8080'
  phases:
    - duration: 60
      arrivalRate: 10
scenarios:
  - name: "Checkout flow"
    flow:
      - post:
          url: "/checkout?user_id={{ $randomInt(1, 1000) }}&item_id=item{{ $randomInt(1, 100) }}"
EOF

# Run load test
artillery run load-test.yml
```

### Database Testing

**Check active sales:**
```bash
docker exec flashsale-postgres psql -U postgres -d flashsale -c "SELECT * FROM sales WHERE active = true;"
```

**Check checkout attempts:**
```bash
docker exec flashsale-postgres psql -U postgres -d flashsale -c "SELECT COUNT(*) FROM checkout_attempts;"
```

**Check Redis data:**
```bash
docker exec flashsale-redis redis-cli KEYS "*"
```

## 🐛 Troubleshooting

### Common Issues

**1. "Sale is not currently active"**
- The background sale manager creates sales every hour on the hour
- Sales run from XX:00:00 to XX:59:59
- Check server logs for sale creation messages
- Ensure server has been running for a few minutes

**2. "Database connection failed"**
- Ensure Docker containers are running: `docker ps`
- Check container health: `docker-compose ps`
- Restart containers: `docker-compose restart`

**3. "Checkout code has expired"**
- Checkout codes expire after 10 minutes
- Generate a new checkout code
- Check system time synchronization

**4. "User purchase limit exceeded"**
- Each user can only purchase 10 items per sale
- Use different user_id for testing
- Wait for next hourly sale

### Debug Commands

**Check container status:**
```bash
docker ps
docker logs flashsale-postgres
docker logs flashsale-redis
```

**Check server logs:**
The server outputs detailed logs including:
- Database connection status
- Sale creation/activation
- Purchase attempts
- Error details

**Reset everything:**
```bash
# Stop server (Ctrl+C)
# Reset databases
docker-compose down -v
docker-compose up -d
# Restart server
go run cmd/server/main.go
```

## 📊 Performance Targets

- **Throughput**: >1,000 requests/second
- **Checkout Response**: <50ms
- **Purchase Response**: <100ms  
- **Memory Usage**: <500MB under load
- **Concurrency**: 1,000+ concurrent users

## 🛑 Stopping Services

**Stop the server:** `Ctrl+C`

**Stop databases:**
```bash
docker-compose down
```

**Remove all data:**
```bash
docker-compose down -v
```

## 🏆 Contest Optimization

This system is optimized for programming contests with:
- **Minimal dependencies** (only 3 external packages)
- **Standard library HTTP** (no frameworks)
- **Atomic Redis operations** (race condition free)
- **Prepared SQL statements** (high performance)
- **Connection pooling** (optimized for load)

Perfect for demonstrating Go expertise and system design skills! 🚀 