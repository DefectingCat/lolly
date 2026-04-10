# lua-nginx-module 定时器和用户线程 API

本文档详细说明 lua-nginx-module 的定时器和用户线程功能。

---

## 一、Timer API

### 核心文件

- `src/ngx_http_lua_timer.c` - 定时器核心实现
- `src/ngx_http_lua_timer.h` - 定时器头文件

### 1.1 `ngx.timer.at(delay, callback, ...)`

创建一次性定时器。

- **参数**:
  - `delay` (number): 延迟秒数，支持小数
  - `callback` (function): 回调函数
  - `...`: 传递给回调的额外参数
- **返回值**: timer_id (成功) 或 nil, err
- **回调参数**: `premature`, `...` (额外参数)
- **示例**:
  ```lua
  local function handler(premature, data)
      if premature then
          return  -- Worker 正在退出
      end
      ngx.log(ngx.INFO, "Timer fired: ", data)
  end

  local ok, err = ngx.timer.at(5, handler, "hello")
  if not ok then
      ngx.log(ngx.ERR, "failed to create timer: ", err)
  end
  ```

### 1.2 `ngx.timer.every(delay, callback, ...)`

创建周期性定时器。

- **参数**: 同 `ngx.timer.at`
- **返回值**: timer_id 或 nil, err
- **示例**:
  ```lua
  local function heartbeat()
      -- 定期执行任务
      local redis = require "resty.redis"
      -- ...
  end

  ngx.timer.every(30, heartbeat)  -- 每 30 秒执行
  ```

### 1.3 `ngx.timer.running_count()`

获取当前正在执行的定时器数量。

- **返回值**: number

### 1.4 `ngx.timer.pending_count()`

获取等待执行的定时器数量。

- **返回值**: number

### 1.5 定时器限制

| 配置指令 | 默认值 | 说明 |
|---------|--------|------|
| `lua_max_pending_timers` | 1024 | 最大挂起定时器数 |
| `lua_max_running_timers` | 256 | 最大运行定时器数 |

### 1.6 内部实现

**数据结构**:

```c
typedef struct {
    void        **main_conf;
    void        **srv_conf;
    void        **loc_conf;
    lua_State    *co;              // 定时器回调协程
    ngx_pool_t   *pool;
    int           co_ref;
    unsigned      delay:31;        // 周期性定时器的延迟
    unsigned      premature:1;     // 是否被提前终止
} ngx_http_lua_timer_ctx_t;
```

**执行机制**:

1. 创建 `ngx_event_t` + `ngx_http_lua_timer_ctx_t`
2. 使用 `ngx_add_timer(ev, delay)` 加入 Nginx 红黑树定时器
3. 定时器到期时，`ngx_http_lua_timer_handler` 执行
4. 创建 fake connection/request 模拟请求上下文

---

## 二、Coroutine API

### 核心文件

- `src/ngx_http_lua_coroutine.c` - 协程核心实现
- `src/ngx_http_lua_coroutine.h` - 协程头文件

### 2.1 协程状态

```c
typedef enum {
    NGX_HTTP_LUA_CO_RUNNING   = 0,  // 运行中
    NGX_HTTP_LUA_CO_SUSPENDED = 1,  // 挂起
    NGX_HTTP_LUA_CO_NORMAL    = 2,  // 正常（被 resume 的协程）
    NGX_HTTP_LUA_CO_DEAD      = 3,  // 已结束
    NGX_HTTP_LUA_CO_ZOMBIE    = 4   // 僵尸（父协程已结束）
} ngx_http_lua_co_status_e;
```

### 2.2 标准 Coroutine API

lua-nginx-module 扩展了 Lua 标准 coroutine 库：

| API | 说明 |
|-----|------|
| `coroutine.create(func)` | 创建新协程 |
| `coroutine.wrap(func)` | 创建包装协程 |
| `coroutine.resume(co, ...)` | 恢复协程执行 |
| `coroutine.yield(...)` | 挂起当前协程 |
| `coroutine.status(co)` | 获取协程状态 |

### 2.3 设计特点

- 所有用户协程都在主协程中创建
- 确保总是 yield 到主 Lua 线程
- 使用 `ngx_http_lua_co_ctx_t` 管理协程上下文

---

## 三、User Thread (Light Thread) API

### 核心文件

- `src/ngx_http_lua_uthread.c` - 用户线程实现
- `src/ngx_http_lua_uthread.h` - 用户线程头文件

### 3.1 概念说明

Light Thread (uthread) 是一种特殊的协程：
- 通过 `ngx.thread.*` API 操作
- 父子关系严格：只有父协程可以 wait/kill 子线程
- 使用 `is_uthread` 标记区分

### 3.2 API

#### `ngx.thread.spawn(func, ...)`

创建轻量级线程。

- **参数**:
  - `func` (function): 线程函数
  - `...`: 传递给函数的参数
- **返回值**: thread 对象
- **示例**:
  ```lua
  local function fetch(url)
      local sock = ngx.socket.tcp()
      -- ... 网络操作
      return result
  end

  local thread1 = ngx.thread.spawn(fetch, "http://api1")
  local thread2 = ngx.thread.spawn(fetch, "http://api2")
  ```

#### `ngx.thread.wait(thread1, thread2, ...)`

等待线程结束。

- **参数**: 一个或多个 thread 对象
- **返回值**: success, res_or_err, thread
- **示例**:
  ```lua
  local ok, res = ngx.thread.wait(thread1, thread2)
  if ok then
      ngx.say("Result: ", res)
  end
  ```

#### `ngx.thread.kill(thread)`

终止线程。

- **参数**: thread 对象
- **返回值**: 1 或 nil, err

### 3.3 判断宏

```c
#define ngx_http_lua_is_thread(ctx) \
    ((ctx)->cur_co_ctx->is_uthread || (ctx)->cur_co_ctx == &(ctx)->entry_co_ctx)

#define ngx_http_lua_is_entry_thread(ctx) \
    ((ctx)->cur_co_ctx == &(ctx)->entry_co_ctx)

#define ngx_http_lua_coroutine_alive(coctx) \
    ((coctx)->co_status != NGX_HTTP_LUA_CO_DEAD \
     && (coctx)->co_status != NGX_HTTP_LUA_CO_ZOMBIE)
```

---

## 四、Semaphore API

### 核心文件

- `src/ngx_http_lua_semaphore.c` - 信号量实现
- `src/ngx_http_lua_semaphore.h` - 信号量头文件

### 4.1 数据结构

```c
typedef struct ngx_http_lua_sema_s {
    ngx_queue_t     wait_queue;      // 等待队列
    int             resource_count;  // 资源计数
    unsigned        wait_count;      // 等待计数
} ngx_http_lua_sema_t;
```

### 4.2 API

#### `ngx.semaphore.new(n)`

创建信号量。

- **参数**: `n` (number) - 初始资源数
- **返回值**: semaphore 对象 或 nil, err
- **示例**:
  ```lua
  local semaphore = require "ngx.semaphore"
  local sem, err = semaphore.new(0)
  ```

#### `sem:post(n?)`

释放资源。

- **参数**: `n` (number, 可选, 默认 1) - 释放数量
- **示例**:
  ```lua
  sem:post(1)  -- 释放 1 个资源
  ```

#### `sem:wait(timeout)`

等待资源。

- **参数**: `timeout` (number) - 超时秒数
- **返回值**: 1 或 nil, err
- **示例**:
  ```lua
  local ok, err = sem:wait(5)  -- 最多等待 5 秒
  if not ok then
      ngx.log(ngx.ERR, "wait timeout: ", err)
  end
  ```

#### `sem:count()`

获取可用资源数。

- **返回值**: number

### 4.3 使用示例

```lua
local semaphore = require "ngx.semaphore"

local sem = semaphore.new(0)

-- 生产者
local function producer()
    for i = 1, 10 do
        ngx.sleep(0.1)
        sem:post(1)
        ngx.log(ngx.INFO, "produced: ", i)
    end
end

-- 消费者
local function consumer()
    for i = 1, 10 do
        sem:wait(1)
        ngx.log(ngx.INFO, "consumed: ", i)
    end
end

ngx.thread.spawn(producer)
ngx.thread.spawn(consumer)
```

---

## 五、关系图

```
ngx.timer.* ─────────────────────────────┐
   │                                      │
   ▼                                      │
ngx_event_t (Nginx 定时器红黑树)          │
   │                                      │
   ▼                                      │
lua_State (协程) ◄── coroutine.create     │
   │                     │                │
   │                     ▼                │
   │           ngx_http_lua_co_ctx_t      │
   │                     │                │
   └─────────► ngx.thread.spawn ─────────►│
                (uthread/light thread)    │
                     │                    │
                     ▼                    │
               ngx.semaphore.new          │
                     │                    │
                     ▼                    │
               ngx_http_lua_sema_t ───────┘
```

---

## 六、最佳实践

### 6.1 定时器清理

```lua
local timer_id

local function cleanup()
    -- Worker 退出时清理
    ngx.log(ngx.INFO, "cleaning up...")
end

timer_id = ngx.timer.every(60, function(premature)
    if premature then
        cleanup()
        return
    end
    -- 正常任务
end)
```

### 6.2 并发控制

```lua
local semaphore = require "ngx.semaphore"
local sem = semaphore.new(3)  -- 最大 3 个并发

local function limited_task()
    sem:wait(10)  -- 等待获取资源
    -- 执行任务
    sem:post(1)   -- 释放资源
end

for i = 1, 10 do
    ngx.thread.spawn(limited_task)
end
```