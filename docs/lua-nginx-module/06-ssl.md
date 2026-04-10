# lua-nginx-module SSL/TLS 功能

本文档详细说明 lua-nginx-module 的 SSL/TLS 相关功能。

---

## 一、核心文件

| 文件 | 说明 |
|------|------|
| `src/ngx_http_lua_ssl_certby.c` | 动态证书选择 |
| `src/ngx_http_lua_ssl_session_storeby.c` | Session 存储回调 |
| `src/ngx_http_lua_ssl_session_fetchby.c` | Session 获取回调 |
| `src/ngx_http_lua_ssl_client_helloby.c` | Client Hello 处理 |
| `src/ngx_http_lua_ssl.c` | SSL 上下文初始化 |

---

## 二、ssl_certificate_by_lua - 动态证书选择

### 2.1 功能概述

在 SSL 握手前执行 Lua 代码，动态选择证书。常用于：
- SNI 多域名证书选择
- 动态证书加载
- Let's Encrypt 自动证书

### 2.2 OpenSSL 回调集成

```c
int ngx_http_lua_ssl_cert_handler(ngx_ssl_conn_t *ssl_conn, void *data)
{
    // 1. 获取连接上下文
    // 2. 创建 fake connection/request
    // 3. 调用 Lua handler
    // 4. 返回结果 (1=成功, 0=失败)
}
```

### 2.3 FFI API

| 函数 | 功能 |
|------|------|
| `ngx_http_lua_ffi_ssl_clear_certs` | 清除当前证书 |
| `ngx_http_lua_ffi_set_cert` | 设置证书链 |
| `ngx_http_lua_ffi_set_priv_key` | 设置私钥 |
| `ngx_http_lua_ffi_ssl_server_name` | 获取 SNI 服务器名 |
| `ngx_http_lua_ffi_ssl_raw_server_addr` | 获取服务器地址 |
| `ngx_http_lua_ffi_ssl_client_random` | 获取客户端随机数 |
| `ngx_http_lua_ffi_ssl_verify_client` | 启用客户端证书验证 |

### 2.4 证书解析 API

```lua
local ssl = require "ngx.ssl"

-- PEM 转 DER
local der_cert, err = ssl.cert_pem_to_der(pem_cert)
local der_key, err = ssl.priv_key_pem_to_der(pem_key)

-- 设置证书
local ok, err = ssl.set_der_cert(der_cert)
local ok, err = ssl.set_der_priv_key(der_key)
```

### 2.5 使用示例

```nginx
server {
    listen 443 ssl;
    server_name ~^(.+)\.example\.com$;

    ssl_certificate_by_lua_block {
        local ssl = require "ngx.ssl"

        -- 获取 SNI
        local server_name, err = ssl.server_name()
        if not server_name then
            ngx.log(ngx.ERR, "no SNI")
            return ngx.exit(ngx.ERROR)
        end

        -- 动态加载证书
        local cert = load_cert_from_redis(server_name)
        if cert then
            ssl.set_der_cert(cert.der_cert)
            ssl.set_der_priv_key(cert.der_key)
        end
    }

    ssl_certificate /fallback.crt;
    ssl_certificate_key /fallback.key;
}
```

---

## 三、SSL Session 缓存

### 3.1 ssl_session_store_by_lua

存储 Session 到外部存储。

```nginx
ssl_session_store_by_lua_block {
    local ssl_session = require "ngx.ssl.session"

    local id, err = ssl_session.get_session_id()
    local data, err = ssl_session.get_serialized_session()

    -- 存储到 Redis
    local redis = require "resty.redis"
    local red = redis:new()
    red:connect("127.0.0.1", 6379)
    red:setex("ssl:sess:" .. id, 3600, data)
}
```

### 3.2 ssl_session_fetch_by_lua

从外部存储获取 Session。

```nginx
ssl_session_fetch_by_lua_block {
    local ssl_session = require "ngx.ssl.session"

    local id, err = ssl_session.get_session_id()

    local redis = require "resty.redis"
    local red = redis:new()
    red:connect("127.0.0.1", 6379)
    local data = red:get("ssl:sess:" .. id)

    if data then
        ssl_session.set_serialized_session(data)
    end
}
```

### 3.3 异步支持

Session fetch 支持异步操作：

```c
#ifdef SSL_ERROR_PENDING_SESSION
return SSL_magic_pending_session_ptr();  // 挂起，等待异步完成
#endif
```

---

## 四、ssl_client_hello_by_lua

### 4.1 功能概述

在 Client Hello 消息处理后执行，可用于：
- 基于客户端能力选择协议
- 访问控制
- 早期拒绝

### 4.2 要求

- **OpenSSL 1.1.1+**
- 使用 `SSL_ERROR_WANT_CLIENT_HELLO_CB` 回调机制

### 4.3 使用示例

```nginx
ssl_client_hello_by_lua_block {
    local ssl = require "ngx.ssl"

    -- 获取支持的协议版本
    local version = ssl.get_tls1_version()

    -- 拒绝旧版 TLS
    if version < 0x0303 then  -- TLS 1.2
        return ngx.exit(ngx.ERROR)
    end
}
```

---

## 五、Socket SSL

### 5.1 SSL 握手

```lua
local sock = ngx.socket.tcp()
sock:connect("example.com", 443)

-- SSL 握手
local session, err = sock:sslhandshake(
    nil,           -- session 复用对象
    "example.com", -- SNI
    true           -- 验证证书
)

if not session then
    ngx.log(ngx.ERR, "SSL handshake failed: ", err)
    return
end
```

### 5.2 SSL 配置指令

| 指令 | 说明 |
|------|------|
| `lua_ssl_protocols` | SSL 协议版本 |
| `lua_ssl_ciphers` | 加密套件 |
| `lua_ssl_verify_depth` | 验证深度 |
| `lua_ssl_trusted_certificate` | CA 证书 |

---

## 六、Proxy SSL

### 6.1 `proxy_ssl_certificate_by_lua`

动态设置代理 SSL 客户端证书。

### 6.2 `proxy_ssl_verify_by_lua`

自定义代理 SSL 验证。

---

## 七、FFI API 参考

### 证书操作

```c
// PEM 解析
void *ngx_http_lua_ffi_parse_pem_cert(const u_char *pem, size_t len, char **err);
void *ngx_http_lua_ffi_parse_pem_priv_key(const u_char *pem, size_t len, char **err);

// DER 解析
void *ngx_http_lua_ffi_parse_der_cert(const char *data, size_t len, char **err);
void *ngx_http_lua_ffi_parse_der_priv_key(const char *data, size_t len, char **err);

// 释放
void ngx_http_lua_ffi_free_cert(void *cdata);
void ngx_http_lua_ffi_free_priv_key(void *cdata);
```

### SSL 信息获取

```c
int ngx_http_lua_ffi_ssl_server_name(ngx_http_request_t *r,
    const char **name, size_t *namelen, char **err);

int ngx_http_lua_ffi_ssl_raw_server_addr(ngx_http_request_t *r,
    const char **addr, size_t *addrlen, int *port, char **err);

int ngx_http_lua_ffi_ssl_client_random(ngx_http_request_t *r,
    unsigned char *out, size_t *outlen, char **err);
```