# Lua Engine API Reference

## Timer Callback Limitations

### No Upvalue Capture

Timer callbacks cannot capture local variables (upvalues/closure variables). Attempting to register a callback with captured variables will fail with:

```
timer callback cannot capture upvalues (closure variables); use shared dict instead
```

**Reason**: The timer callback executes in a dedicated scheduler LState after the source coroutine has died. Captured values would reference dead coroutine memory.

**Workaround**: Use shared dict to pass data:

```lua
-- WRONG: captures local variable
local request_id = ngx.var.request_id
ngx.timer.at(5, function()
    ngx.log(ngx.INFO, request_id)  -- sees nil, not captured value
end)

-- RIGHT: use shared dict
local dict = ngx.shared.timer_data
dict:set("request_id", ngx.var.request_id)
ngx.timer.at(5, function()
    local dict = ngx.shared.timer_data
    ngx.log(ngx.INFO, dict:get("request_id"))
end)
```

## Safe vs Unsafe API Scope

### Safe APIs (available in timer callbacks)

These APIs can be called from timer callbacks without restriction:

| API | Description |
|-----|-------------|
| `ngx.shared.DICT.*` | Shared memory operations (get/set/add/incr/delete/flush) |
| `ngx.log(level, ...)` | Logging without request context |
| `ngx.timer.at()` | Create new timers (nesting supported) |
| `ngx.timer.running_count()` | Get active timer count |

### Unsafe APIs (blocked in timer callbacks)

These APIs require request context and raise errors when called in timer callbacks:

| API | Error Message |
|-----|---------------|
| `ngx.req.*` | `API ngx.req.X not available in timer callback context` |
| `ngx.resp.*` | `API ngx.resp.X not available in timer callback context` |
| `ngx.var.*` | `API ngx.var not available in timer callback context` |
| `ngx.ctx.*` | `API ngx.ctx not available in timer callback context` |
| `ngx.location.capture()` | `API ngx.location.capture not available in timer callback context` |
| `ngx.say/print/flush` | `API ngx.say not available in timer callback context` |
| `ngx.exit/redirect` | `API ngx.exit not available in timer callback context` |

## Shared Dictionary API

### Creating Shared Dicts

```go
// In Go code
engine.CreateSharedDict("my_dict", 1000)  // max 1000 items
```

### Lua Usage

```lua
local dict = ngx.shared.my_dict

-- Basic operations
dict:set("key", "value", 3600)  -- TTL: 3600 seconds
dict:add("key", "value", 3600)  -- Add only if not exists
dict:replace("key", "value", 3600)  -- Replace only if exists

local value = dict:get("key")
local value, err = dict:get_stale("key")  -- Get expired items

dict:delete("key")

-- Increment
local new_val, err = dict:incr("counter", 1)
local new_val, err = dict:incr("counter", 1, 0)  -- Init value if not exists

-- Flush all
local count = dict:flush_all()
local count = dict:flush_expired(100)  -- Flush expired, max 100
```

## Timer API

### Creating Timers

```lua
-- Basic timer
local ok, err = ngx.timer.at(5, function()
    ngx.log(ngx.INFO, "timer fired")
end)

-- With arguments
local ok, err = ngx.timer.at(5, function(premature, arg1, arg2)
    ngx.log(ngx.INFO, "timer args:", arg1, arg2)
end, "value1", "value2")

-- Check running count
local count = ngx.timer.running_count()
```

### Canceling Timers

```lua
local timer, err = ngx.timer.at(10, callback)
if timer then
    timer:cancel()  -- Cancel before it fires
end
```

## Subrequest API

### ngx.location.capture

```lua
local res = ngx.location.capture("/internal/path")

-- Result structure
-- res.status: HTTP status code
-- res.body: Response body
-- res.headers: Response headers table

-- With options
local res = ngx.location.capture("/api", {
    method = "POST",
    body = '{"data": "value"}',
    headers = {
        ["Content-Type"] = "application/json"
    }
})
```

## Best Practices

### Timer Callbacks

1. **Always use shared dict** for passing data to timer callbacks
2. **Keep callbacks short** - avoid long-running operations
3. **Handle errors gracefully** - use `pcall` for error handling
4. **Avoid blocking operations** - no network calls in timer context (use cosocket in request context instead)

### Shared Dictionary

1. **Set appropriate TTLs** - avoid memory leaks from stale data
2. **Use atomic operations** - `add`, `replace`, `incr` for concurrent safety
3. **Handle errors** - check return values for `err`

### Performance

1. **Minimize timer count** - use `running_count()` to monitor
2. **Batch operations** - use `flush_expired` periodically
3. **Reuse shared dicts** - create once at engine startup

## Thread Safety

The Lua engine uses a dedicated scheduler goroutine for timer callback execution. All LState operations are single-threaded:

- Request handlers execute in their own goroutine with per-request LState
- Timer callbacks execute in the scheduler goroutine with dedicated LState
- Shared dicts are thread-safe and can be accessed from both contexts

## Graceful Shutdown

On engine close:

1. New timers are rejected (`stopping` flag)
2. The callback queue channel is closed
3. The scheduler goroutine drains remaining callbacks
4. A 5-second timeout is enforced
5. Remaining callbacks are abandoned and logged

Example shutdown log:
```
[lua] shutdown timeout: 3 callbacks abandoned
```

## Known Limitations

| Limitation | Reason | Workaround |
|------------|--------|------------|
| Timer callbacks cannot capture upvalues | gopher-lua `NewFunctionFromProto` does not preserve upvalue closures | Use `ngx.shared.DICT` to pass data |
| Request-scoped APIs unavailable in timer callbacks | Timer callbacks have no `RequestCtx` | Use shared dict for data sharing |
| Subrequests use deep-copied parent request data | Cannot access coroutine's live `RequestCtx` from API function | Parent data is copied at capture time |

## Troubleshooting

### "timer callback cannot capture upvalues"

Your callback captured a local variable. Use shared dict instead:

```lua
-- Instead of:
local data = ngx.var.request_id
ngx.timer.at(5, function() ngx.log(ngx.INFO, data) end)

-- Use:
ngx.shared.timer_data:set("key", ngx.var.request_id)
ngx.timer.at(5, function()
    ngx.log(ngx.INFO, ngx.shared.timer_data:get("key"))
end)
```

### "API ngx.X not available in timer callback context"

The API requires request context. Timer callbacks run in an isolated scheduler LState.
Restructure your code to pass needed data via shared dict before creating the timer.

### High timer callback latency

- Check callback queue depth (default: 1024)
- Long-running callbacks delay subsequent ones
- Consider reducing callback work or splitting into multiple timers