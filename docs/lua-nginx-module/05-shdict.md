# lua-nginx-module 共享内存字典 (shdict)

本文档详细说明 lua-nginx-module 的共享内存字典功能。

---

## 一、核心文件

| 文件 | 说明 |
|------|------|
| `src/ngx_http_lua_shdict.c` | shdict 核心实现 |
| `src/ngx_http_lua_shdict.h` | 数据结构定义 |
| `src/ngx_http_lua_directive.c` | 配置指令处理 |

---

## 二、数据结构

### 2.1 节点结构

```c
typedef struct {
    u_char       color;         /* 红黑树颜色 */
    uint8_t      value_type;    /* 值类型 */
    u_short      key_len;       /* key 长度 */
    uint32_t     value_len;     /* value 长度 */
    uint64_t     expires;       /* 过期时间 (毫秒) */
    ngx_queue_t  queue;         /* LRU 队列节点 */
    uint32_t     user_flags;    /* 用户自定义标志 */
    u_char       data[1];       /* key + value 数据 */
} ngx_http_lua_shdict_node_t;
```

### 2.2 值类型枚举

```c
enum {
    SHDICT_TNIL     = 0,  /* 空值 */
    SHDICT_TBOOLEAN = 1,  /* 布尔值 */
    SHDICT_TNUMBER  = 3,  /* 数字 */
    SHDICT_TSTRING  = 4,  /* 字符串 */
    SHDICT_TLIST    = 5,  /* 列表 */
};
```

### 2.3 共享上下文

```c
typedef struct {
    ngx_rbtree_t       rbtree;     /* 红黑树根 */
    ngx_rbtree_node_t  sentinel;   /* 红黑树哨兵 */
    ngx_queue_t        lru_queue;  /* LRU 队列头 */
} ngx_http_lua_shdict_shctx_t;

typedef struct {
    ngx_http_lua_shdict_shctx_t *sh;      /* 共享上下文 */
    ngx_slab_pool_t             *shpool;  /* slab 分配器 */
    ngx_str_t                    name;    /* 字典名称 */
} ngx_http_lua_shdict_ctx_t;
```

---

## 三、配置

### `lua_shared_dict <name> <size>`

定义共享内存字典。

- **参数**:
  - `name`: 字典名称
  - `size`: 内存大小 (最小 8KB)
- **配置层级**: main
- **示例**:
  ```nginx
  lua_shared_dict cache 10m;
  lua_shared_dict sessions 100m;
  lua_shared_dict counters 1m;
  ```

---

## 四、Lua API

### 4.1 获取字典对象

```lua
local dict = ngx.shared.cache
```

### 4.2 基础操作

#### `dict:get(key)`

获取值。

- **参数**: `key` (string)
- **返回值**: value, err 或 nil
- **示例**:
  ```lua
  local val, err = ngx.shared.cache:get("user:123")
  if val == nil and err then
      ngx.log(ngx.ERR, "get failed: ", err)
  end
  ```

#### `dict:get_stale(key)`

获取值（允许过期数据）。

#### `dict:set(key, value, exptime?, flags?)`

设置值。

- **参数**:
  - `key` (string)
  - `value` (string/number/boolean/nil)
  - `exptime` (number, 可选): 过期时间秒数
  - `flags` (number, 可选): 用户标志
- **返回值**: success, err, forcible
- **示例**:
  ```lua
  local dict = ngx.shared.cache
  local ok, err, forcible = dict:set("key", "value", 60)
  if forcible then
      ngx.log(ngx.WARN, "forced to evict old items")
  end
  ```

#### `dict:safe_set(key, value, exptime?, flags?)`

安全设置值（不会淘汰有效项）。

#### `dict:add(key, value, exptime?, flags?)`

仅在 key 不存在时添加。

#### `dict:safe_add(key, value, exptime?, flags?)`

安全添加（不会淘汰有效项）。

#### `dict:replace(key, value, exptime?, flags?)`

仅在 key 存在时替换。

#### `dict:delete(key)`

删除 key。

### 4.3 数值操作

#### `dict:incr(key, value, init?)`

原子递增。

- **参数**:
  - `key` (string)
  - `value` (number): 增量
  - `init` (number, 可选): key 不存在时的初始值
- **返回值**: new_value, err 或 nil
- **示例**:
  ```lua
  local dict = ngx.shared.counters
  local new_val, err = dict:incr("hits", 1, 0)
  ngx.say("Hits: ", new_val)
  ```

### 4.4 列表操作

#### `dict:lpush(key, value)`

左侧推入列表。

#### `dict:rpush(key, value)`

右侧推入列表。

#### `dict:lpop(key)`

左侧弹出列表。

#### `dict:rpop(key)`

右侧弹出列表。

#### `dict:llen(key)`

获取列表长度。

- **示例**:
  ```lua
  local dict = ngx.shared.queue
  dict:rpush("jobs", "job1")
  dict:rpush("jobs", "job2")
  local job = dict:lpop("jobs")  -- "job1"
  local len = dict:llen("jobs")  -- 1
  ```

### 4.5 过期管理

#### `dict:ttl(key)`

获取剩余 TTL。

- **返回值**: ttl, err 或 nil

#### `dict:expire(key, exptime)`

设置过期时间。

- **参数**:
  - `key` (string)
  - `exptime` (number): 秒数
- **返回值**: success, err

#### `dict:flush_all()`

标记所有项为过期。

#### `dict:flush_expired(max_count?)`

清除过期项。

- **参数**: `max_count` (number, 可选)

### 4.6 容量信息

#### `dict:capacity()`

获取总容量。

- **返回值**: number (bytes)

#### `dict:free_space()`

获取空闲空间。

- **返回值**: number (bytes)

#### `dict:get_keys(max_count?)`

获取所有 key。

- **参数**: `max_count` (number, 可选, 默认 1024)
- **返回值**: table of keys

---

## 五、内部实现

### 5.1 Slab 分配器

使用 Nginx 的 slab 分配器管理共享内存：

- `ngx_slab_alloc_locked()` / `ngx_slab_alloc()`
- `ngx_slab_free_locked()` / `ngx_slab_free()`

### 5.2 红黑树索引

用于快速查找：

- Key 哈希 → 红黑树节点
- 冲突处理：`ngx_memn2cmp` 比较实际 key

### 5.3 LRU 队列

实现最近最少使用淘汰：

- 新访问项移到队列头
- 淘汰时从队列尾移除

### 5.4 并发控制

使用互斥锁保护共享数据：

```c
ngx_shmtx_lock(&ctx->shpool->mutex);
// 操作共享数据
ngx_shmtx_unlock(&ctx->shpool->mutex);
```

---

## 六、使用示例

### 6.1 简单缓存

```lua
local function get_with_cache(key, fetch_func, ttl)
    local dict = ngx.shared.cache
    local value = dict:get(key)

    if value then
        return value
    end

    value = fetch_func()
    dict:set(key, value, ttl)
    return value
end
```

### 6.2 分布式锁

```lua
local function acquire_lock(lock_name, timeout, expiry)
    local dict = ngx.shared.locks
    local ok, err = dict:add(lock_name, 1, expiry)
    if ok then
        return true
    end
    return false, err
end

local function release_lock(lock_name)
    local dict = ngx.shared.locks
    dict:delete(lock_name)
end
```

### 6.3 限流计数

```lua
local function rate_limit(key, limit, window)
    local dict = ngx.shared.limits
    local count, err = dict:incr(key, 1, 0, window)

    if count > limit then
        return false  -- 超限
    end
    return true
end
```