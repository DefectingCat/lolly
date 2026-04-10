# lua-nginx-module 配置指令详解

本文档详细列出 lua-nginx-module 的所有配置指令，供 lolly 项目参考实现。

## 一、初始化类指令 (Initialization Phase)

| 指令 | 参数类型 | 配置层级 | 执行时机 | Nginx Phase |
|------|---------|---------|---------|-------------|
| `init_by_lua` | inline script | main | Master 进程启动时 | 配置加载阶段 |
| `init_by_lua_block` | block | main | Master 进程启动时 | 配置加载阶段 |
| `init_by_lua_file` | file path | main | Master 进程启动时 | 配置加载阶段 |
| `init_worker_by_lua` | inline script | main | Worker 进程启动时 | worker 初始化 |
| `init_worker_by_lua_block` | block | main | Worker 进程启动时 | worker 初始化 |
| `init_worker_by_lua_file` | file path | main | Worker 进程启动时 | worker 初始化 |
| `exit_worker_by_lua_block` | block | main | Worker 进程退出时 | worker 退出 |
| `exit_worker_by_lua_file` | file path | main | Worker 进程退出时 | worker 退出 |

### 使用示例

```nginx
# init_by_lua - 初始化 Lua VM，加载共享模块
init_by_lua_block {
    require "resty.core"
    local cache = require "myapp.cache"
    cache.init()
}

# init_worker_by_lua - Worker 级初始化（如定时任务）
init_worker_by_lua_block {
    local timer = require "myapp.timer"
    timer.start_heartbeat()
}

# exit_worker_by_lua - Worker 退出清理
exit_worker_by_lua_block {
    local cleanup = require "myapp.cleanup"
    cleanup.flush_pending_data()
}
```

---

## 二、变量设置类指令 (Variable Phase)

| 指令 | 参数类型 | 配置层级 | 执行时机 |
|------|---------|---------|---------|
| `set_by_lua` | inline script + args | server/if/location | 变量赋值时 |
| `set_by_lua_block` | block | server/if/location | 变量赋值时 |
| `set_by_lua_file` | file path + args | server/if/location | 变量赋值时 |

### 特点

- **同步执行**: 不能 yield，必须立即返回
- **返回值**: 通过 `return` 返回结果赋给变量
- **参数传递**: 支持额外参数传递给 Lua 代码

### 使用示例

```nginx
location /set {
    set $res "";
    set_by_lua_block $res {
        local a = tonumber(ngx.arg[1])
        local b = tonumber(ngx.arg[2])
        return a + b
    } $arg_a $arg_b;

    return 200 "Result: $res";
}
```

---

## 三、请求处理类指令 (Request Handling Phases)

### 3.1 Server Rewrite Phase

| 指令 | 参数类型 | 配置层级 |
|------|---------|---------|
| `server_rewrite_by_lua_block` | block | main/server |
| `server_rewrite_by_lua_file` | file path | main/server |

### 3.2 Rewrite Phase

| 指令 | 参数类型 | 配置层级 |
|------|---------|---------|
| `rewrite_by_lua` | inline script | main/server/location/if |
| `rewrite_by_lua_block` | block | main/server/location/if |
| `rewrite_by_lua_file` | file path | main/server/location/if |

### 3.3 Access Phase

| 指令 | 参数类型 | 配置层级 |
|------|---------|---------|
| `access_by_lua` | inline script | main/server/location/if |
| `access_by_lua_block` | block | main/server/location/if |
| `access_by_lua_file` | file path | main/server/location/if |

### 3.4 Precontent Phase

| 指令 | 参数类型 | 配置层级 |
|------|---------|---------|
| `precontent_by_lua_block` | block | main/server/location/if |
| `precontent_by_lua_file` | file path | main/server/location/if |

### 3.5 Content Phase

| 指令 | 参数类型 | 配置层级 |
|------|---------|---------|
| `content_by_lua` | inline script | location/if |
| `content_by_lua_block` | block | location/if |
| `content_by_lua_file` | file path | location/if |

### 3.6 Log Phase

| 指令 | 参数类型 | 配置层级 |
|------|---------|---------|
| `log_by_lua` | inline script | main/server/location/if |
| `log_by_lua_block` | block | main/server/location/if |
| `log_by_lua_file` | file path | main/server/location/if |

### 阶段执行顺序

```
请求进入
    ↓
server_rewrite_by_lua*
    ↓
rewrite_by_lua*
    ↓
access_by_lua*
    ↓
precontent_by_lua*
    ↓
content_by_lua*
    ↓
header_filter_by_lua* → body_filter_by_lua*
    ↓
log_by_lua*
```

---

## 四、过滤类指令 (Filter Phase)

| 指令 | 参数类型 | 配置层级 | 说明 |
|------|---------|---------|------|
| `header_filter_by_lua` | inline script | main/server/location/if | 响应头过滤 |
| `header_filter_by_lua_block` | block | main/server/location/if | 响应头过滤 |
| `header_filter_by_lua_file` | file path | main/server/location/if | 响应头过滤 |
| `body_filter_by_lua` | inline script | main/server/location/if | 响应体过滤 |
| `body_filter_by_lua_block` | block | main/server/location/if | 响应体过滤 |
| `body_filter_by_lua_file` | file path | main/server/location/if | 响应体过滤 |

### 特点

- **同步执行**: 不能 yield
- **ngx.arg 访问**: `ngx.arg[1]` 数据块, `ngx.arg[2]` EOF 标记

---

## 五、负载均衡类指令 (Upstream/Balancer)

| 指令 | 参数类型 | 配置层级 | 说明 |
|------|---------|---------|------|
| `balancer_by_lua_block` | block | upstream | 选择后端服务器 |
| `balancer_by_lua_file` | file path | upstream | 选择后端服务器 |
| `balancer_keepalive` | number | upstream | 连接池配置 |

---

## 六、SSL/TLS 类指令 (SSL Phase)

### 6.1 服务器端 SSL

| 指令 | 参数类型 | 配置层级 | 执行时机 |
|------|---------|---------|---------|
| `ssl_client_hello_by_lua_block` | block | main/server | SSL Client Hello |
| `ssl_client_hello_by_lua_file` | file path | main/server | SSL Client Hello |
| `ssl_certificate_by_lua_block` | block | main/server | SSL 证书阶段 |
| `ssl_certificate_by_lua_file` | file path | main/server | SSL 证书阶段 |
| `ssl_session_store_by_lua_block` | block | main/server | SSL 会话存储 |
| `ssl_session_store_by_lua_file` | file path | main/server | SSL 会话存储 |
| `ssl_session_fetch_by_lua_block` | block | main/server | SSL 会话获取 |
| `ssl_session_fetch_by_lua_file` | file path | main/server | SSL 会话获取 |

### 6.2 代理 SSL (Proxy SSL)

| 指令 | 参数类型 | 配置层级 |
|------|---------|---------|
| `proxy_ssl_certificate_by_lua_block` | block | location/if |
| `proxy_ssl_certificate_by_lua_file` | file path | location/if |
| `proxy_ssl_verify_by_lua_block` | block | location/if |
| `proxy_ssl_verify_by_lua_file` | file path | location/if |

---

## 七、配置控制类指令

| 指令 | 参数类型 | 配置层级 | 默认值 | 说明 |
|------|---------|---------|--------|------|
| `lua_code_cache` | on/off | main/server/location/if | on | Lua 代码缓存开关 |
| `lua_need_request_body` | on/off | main/server/location/if | off | 强制读取请求体 |
| `lua_transform_underscores_in_response_headers` | on/off | main/server/location/if | on | 转换下划线为连字符 |
| `lua_socket_log_errors` | on/off | main/server/location/if | on | socket 错误日志 |

---

## 八、Lua 环境配置指令

| 指令 | 参数类型 | 配置层级 | 说明 |
|------|---------|---------|------|
| `lua_package_path` | path | main | Lua 模块搜索路径 |
| `lua_package_cpath` | path | main | Lua C 模块搜索路径 |
| `lua_shared_dict` | name size | main | 共享内存字典 |
| `lua_regex_cache_max_entries` | number | main | 正则缓存条目数(默认1024) |
| `lua_regex_match_limit` | number | main | 正则匹配限制 |
| `lua_max_pending_timers` | number | main | 最大挂起定时器数(默认1024) |
| `lua_max_running_timers` | number | main | 最大运行定时器数(默认256) |
| `lua_thread_cache_max_entries` | number | main | 线程缓存条目数 |
| `lua_worker_thread_vm_pool_size` | number | main | Worker 线程 VM 池大小(默认10) |

---

## 九、Socket 配置指令

| 指令 | 参数类型 | 配置层级 | 默认值 | 说明 |
|------|---------|---------|--------|------|
| `lua_socket_keepalive_timeout` | time | main/server/location/if | 60s | 连接保活超时 |
| `lua_socket_connect_timeout` | time | main/server/location/if | 60s | 连接超时 |
| `lua_socket_send_timeout` | time | main/server/location/if | 60s | 发送超时 |
| `lua_socket_read_timeout` | time | main/server/location/if | 60s | 读取超时 |
| `lua_socket_send_lowat` | size | main/server/location/if | 0 | 发送低水位 |
| `lua_socket_buffer_size` | size | main/server/location/if | pagesize | 缓冲区大小 |
| `lua_socket_pool_size` | number | main/server/location/if | 30 | 连接池大小 |

---

## 十、SSL 配置指令

| 指令 | 参数类型 | 配置层级 | 说明 |
|------|---------|---------|------|
| `lua_ssl_protocols` | [SSLv2\|SSLv3\|TLSv1...] | main/server/location | SSL 协议版本 |
| `lua_ssl_ciphers` | ciphers | main/server/location | SSL 加密套件 |
| `lua_ssl_verify_depth` | number | main/server/location | 验证深度 |
| `lua_ssl_certificate` | path | main/server/location | SSL 证书 |
| `lua_ssl_certificate_key` | path | main/server/location | SSL 证书密钥 |
| `lua_ssl_trusted_certificate` | path | main/server/location | 可信证书 |
| `lua_ssl_crl` | path | main/server/location | 证书吊销列表 |
| `lua_ssl_key_log` | path | main/server/location | SSL 密钥日志 |
| `lua_ssl_conf_command` | key value | main/server/location | SSL 配置命令 |

---

## 十一、其他指令

| 指令 | 参数类型 | 配置层级 | 默认值 | 说明 |
|------|---------|---------|--------|------|
| `lua_load_resty_core` | on/off | main | - | 加载 resty.core (已废弃) |
| `lua_capture_error_log` | size | main | - | 错误日志捕获 |
| `lua_sa_restart` | on/off | main | on | SA_RESTART 信号处理 |
| `lua_http10_buffering` | on/off | main/server/location/if | on | HTTP/1.0 缓冲 |
| `lua_check_client_abort` | on/off | main/server/location/if | off | 检测客户端中断 |
| `lua_use_default_type` | on/off | main/server/location/if | on | 使用默认 Content-Type |

---

## 指令参数类型说明

| 类型 | 说明 | 示例 |
|------|------|------|
| inline script | 直接嵌入 Lua 代码字符串 | `content_by_lua "ngx.say('hello')";` |
| block | 使用 `{ }` 包裹的 Lua 代码块 | `content_by_lua_block { ngx.say('hello') }` |
| file path | 外部 Lua 文件路径 | `content_by_lua_file /path/to/script.lua;` |
| time | 时间值，支持单位 (s/ms) | `60s`, `5000ms` |
| size | 大小值，支持单位 (k/m/g) | `10m`, `1g` |
| path | 文件系统路径 | `/usr/local/openresty/lualib` |