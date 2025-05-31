# Redis Key Naming Conventions

## Key Patterns

### Sale Management
- `active_sale_id` - Current active sale ID (STRING)
- `sale:{sale_id}:sold` - Items sold count for sale (INTEGER)
- `sale:{sale_id}:available` - Items available for sale (INTEGER)
- `sale:{sale_id}:info` - Sale metadata (HASH)

### User Purchase Tracking
- `user:{user_id}:sale:{sale_id}:count` - User's purchase count for specific sale (INTEGER)
- `user:{user_id}:last_purchase` - Timestamp of user's last purchase (STRING)

### Checkout Code Management
- `checkout:{code}` - Checkout attempt details (HASH)
- `checkout:{code}:ttl` - Checkout code expiration (TTL: 1 hour)

### Performance Caching
- `sale:{sale_id}:cache` - Cached sale information (HASH)
- `items:generated` - Flag indicating if items are generated (STRING)

## TTL Settings

### Short-term (1 hour)
- Checkout codes: `checkout:{code}*`
- Active sale cache: `sale:{sale_id}:cache`

### Medium-term (24 hours)  
- User purchase counts: `user:{user_id}:sale:{sale_id}:count`
- Sale counters: `sale:{sale_id}:sold`, `sale:{sale_id}:available`

### Long-term (7 days)
- Sale metadata: `sale:{sale_id}:info`

## Atomic Operations

### Lua Scripts
1. **atomic_purchase.lua** - Atomic inventory decrement with user limit check
2. **validate_checkout.lua** - Validate and consume checkout code
3. **cleanup_expired.lua** - Clean up expired checkout codes

### Redis Commands Used
- `INCR` - Atomic counter increment
- `GET/SET` - Simple key-value operations
- `HGET/HSET` - Hash field operations
- `EXPIRE` - Set key expiration
- `EVAL` - Execute Lua scripts
- `PIPELINE` - Batch operations

## Key Expiration Strategy

```
checkout:{code}           -> 3600s (1 hour)
sale:{sale_id}:sold       -> 86400s (24 hours)
sale:{sale_id}:available  -> 86400s (24 hours)
user:*:sale:*:count      -> 86400s (24 hours)
sale:{sale_id}:cache     -> 3600s (1 hour)
```

## Memory Optimization

- Use INTEGER for counters instead of STRING where possible
- Set appropriate TTLs to prevent memory bloat
- Use HASH for multi-field data instead of multiple keys
- Pipeline operations to reduce network overhead 