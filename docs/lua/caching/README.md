# Nginx Lua Caching Example

Demonstrates response caching using `ngx.shared.DICT` with TTL and cache invalidation support.

## Components

| File | Purpose |
|---|---|
| `nginx.conf` | Nginx configuration wiring Lua handlers |
| `cache_handler.lua` | Shared cache logic with TTL and invalidation |

## Features

- Cache responses with configurable TTL
- Manual cache invalidation via purge endpoint
- X-Cache header (HIT/MISS) for debugging
- LRU eviction when memory is full

## Usage

1. Define a shared dict in `nginx.conf`:
   ```nginx
   lua_shared_dict response_cache 10m;
   ```

2. Use `cache_handler.lua` in your location blocks:
   ```nginx
   content_by_lua_file /path/to/cache_handler.lua;
   ```

3. Purge a specific cache key:
   ```bash
   curl -X POST http://example.com/cache/purge -d '{"key": "my_key"}'
   ```

4. Purge all cached entries:
   ```bash
   curl -X POST http://example.com/cache/purge_all
   ```
