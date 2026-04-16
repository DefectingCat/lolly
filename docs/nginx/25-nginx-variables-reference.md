# Nginx 内置变量速查表

本文档汇总 nginx 所有模块提供的内置变量，便于快速查阅。

---

## 1. HTTP 核心模块变量 (ngx_http_core_module)

### 请求信息

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `$arg_name` | 请求参数 name 的值 | foo |
| `$args` | 所有请求参数 | a=1&b=2 |
| `$is_args` | 是否有参数 | ? 或 空 |
| `$query_string` | 同 $args | a=1&b=2 |
| `$content_length` | Content-Length 头 | 1024 |
| `$content_type` | Content-Type 头 | application/json |
| `$cookie_name` | Cookie name 的值 | session_id |

### 客户端信息

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `$remote_addr` | 客户端 IP | 192.168.1.1 |
| `$remote_port` | 客户端端口 | 54321 |
| `$binary_remote_addr` | 二进制 IP（4/16字节） | 用于 limit_conn |
| `$remote_user` | 认证用户名 | admin |

### URI 相关

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `$uri` | 当前请求 URI（解码后） | /path/to/file |
| `$document_uri` | 同 $uri | /path/to/file |
| `$request_uri` | 原始请求 URI（含参数） | /path?a=1 |
| `$host` | 请求主机名（优先 Host 头） | example.com |
| `$hostname` | 服务器主机名 | server01 |
| `$server_name` | server 配置的第一个名字 | example.com |

### 请求详情

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `$request` | 完整请求行 | GET / HTTP/1.1 |
| `$request_method` | 请求方法 | GET/POST |
| `$request_body` | 请求体内容 | {...} |
| `$request_body_file` | 请求体临时文件路径 | /tmp/... |
| `$request_completion` | 请求完成状态 | OK 或 空 |
| `$request_filename` | 映射的文件路径 | /var/www/index.html |
| `$request_id` | 唯一请求 ID（1.11.0+） | abc123... |
| `$request_length` | 请求长度（含头） | 2048 |
| `$request_time` | 请求处理时间（秒） | 0.001 |
| `$document_root` | root 指令值 | /var/www |
| `$realpath_root` | root 的真实路径 | /var/www |

### 服务器信息

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `$server_addr` | 服务器 IP | 10.0.0.1 |
| `$server_port` | 服务器端口 | 80 |
| `$scheme` | 协议 | http/https |
| `$server_protocol` | HTTP 版本 | HTTP/1.1 |
| `$https` | 是否 HTTPS | on 或 空 |

### 响应信息

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `$status` | 响应状态码 | 200/404 |
| `$body_bytes_sent` | 响应体字节数 | 1024 |
| `$bytes_sent` | 总发送字节 | 2048 |

### 时间相关

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `$time_local` | 本地时间 | 03/Apr/2026:14:30:00 |
| `$time_iso8601` | ISO8601 时间 | 2026-04-03T14:30:00 |
| `$msec` | 毫秒时间戳 | 1617456789.123 |

### 连接信息

| 变量 | 说明 | 示例值 |
|------|------|--------|
| `$connection` | 连接序号 | 12345 |
| `$connection_requests` | 连接请求数 | 10 |
| `$pipe` | 是否管道化 | p 或 . |
| `$pid` | worker 进程 PID | 12345 |

### PROXY 协议

| 变量 | 说明 |
|------|------|
| `$proxy_protocol_addr` | 客户端真实 IP |
| `$proxy_protocol_port` | 客户端真实端口 |
| `$proxy_protocol_server_addr` | 服务器 IP |
| `$proxy_protocol_server_port` | 服务器端口 |

### TCP 信息

| 变量 | 说明 |
|------|------|
| `$tcpinfo_rtt` | RTT（微秒） |
| `$tcpinfo_rttvar` | RTT 方差 |
| `$tcpinfo_snd_cwnd` | 发送窗口 |
| `$tcpinfo_rcv_space` | 接收窗口 |

---

## 2. Upstream 模块变量 (ngx_http_upstream_module)

| 变量 | 说明 | 用途 |
|------|------|------|
| `$upstream_addr` | 后端地址 | 日志 |
| `$upstream_status` | 后端状态码 | 监控 |
| `$upstream_response_time` | 后端响应时间 | 性能分析 |
| `$upstream_response_length` | 后端响应长度 | 日志 |
| `$upstream_connect_time` | 连接耗时 | 性能分析 |
| `$upstream_header_time` | 头部接收耗时 | 性能分析 |
| `$upstream_first_byte_time` | 首字节时间 | 性能分析 |
| `$upstream_bytes_sent` | 发送到后端字节 | 流量统计 |
| `$upstream_bytes_received` | 从后端接收字节 | 流量统计 |
| `$upstream_cache_status` | 缓存状态 | HIT/MISS/BYPASS |
| `$upstream_http_name` | 后端响应头 | 提取认证信息 |
| `$upstream_cookie_name` | 后端 Cookie | 提取 Cookie |

---

## 3. SSL/TLS 模块变量 (ngx_http_ssl_module)

| 变量 | 说明 |
|------|------|
| `$ssl_cipher` | 加密套件 |
| `$ssl_ciphers` | 支持的加密套件列表 |
| `$ssl_protocol` | SSL 协议版本 |
| `$ssl_server_name` | SNI 服务器名 |
| `$ssl_session_id` | 会话 ID |
| `$ssl_session_reused` | 是否重用会话 |
| `$ssl_client_cert` | 客户端证书 |
| `$ssl_client_raw_cert` | 客户端原始证书 |
| `$ssl_client_escaped_cert` | 转义证书 |
| `$ssl_client_fingerprint` | 证书指纹 |
| `$ssl_client_i_dn` | 签发者 DN |
| `$ssl_client_s_dn` | 主体 DN |
| `$ssl_client_serial` | 证书序列号 |
| `$ssl_client_verify` | 验证结果 |
| `$ssl_client_v_start` | 证书开始时间 |
| `$ssl_client_v_end` | 证书结束时间 |
| `$ssl_client_v_remain` | 证书剩余天数 |
| `$ssl_curves` | 支持的曲线 |
| `$ssl_early_data` | 是否早期数据 |

---

## 4. Proxy 模块变量 (ngx_http_proxy_module)

| 变量 | 说明 |
|------|------|
| `$proxy_add_x_forwarded_for` | X-Forwarded-For 链 |
| `$proxy_host` | proxy_pass 主机 |
| `$proxy_port` | proxy_pass 端口 |

---

## 5. FastCGI 模块变量

| 变量 | 说明 |
|------|------|
| `$fastcgi_path_info` | PATH_INFO |
| `$fastcgi_script_name` | SCRIPT_NAME |

---

## 6. Stream 模块变量 (ngx_stream_core_module)

| 变量 | 说明 |
|------|------|
| `$binary_remote_addr` | 二进制 IP |
| `$connection` | 连接序号 |
| `$remote_addr` | 客户端 IP |
| `$remote_port` | 客户端端口 |
| `$server_addr` | 服务器 IP |
| `$server_port` | 服务器端口 |
| `$status` | 状态码 |
| `$time_iso8601` | ISO 时间 |
| `$time_local` | 本地时间 |
| `$upstream_addr` | 后端地址 |
| `$upstream_bytes_sent` | 发送字节 |
| `$upstream_bytes_received` | 接收字节 |
| `$upstream_connect_time` | 连接时间 |

---

## 7. SSL Preread 模块变量 (ngx_stream_ssl_preread_module)

| 变量 | 说明 |
|------|------|
| `$ssl_preread_protocol` | SSL 协议版本 |
| `$ssl_preread_server_name` | SNI 名称 |

---

## 8. Geo 模块变量 (ngx_http_geo_module)

| 变量 | 说明 |
|------|------|
| 自定义 | 根据 IP 映射的值 |

---

## 9. Map 模块变量 (ngx_http_map_module)

| 变量 | 说明 |
|------|------|
| 自定义 | 映射规则生成的值 |

---

## 10. Limit Request 模块变量

| 变量 | 说明 |
|------|------|
| `$limit_req` | 限流延迟时间（毫秒） |

---

## 11. 常用组合示例

### 日志格式
```nginx
log_format main '$remote_addr - $remote_user [$time_local] '
                '"$request" $status $body_bytes_sent '
                '"$http_referer" "$http_user_agent" '
                'rt=$request_time uct="$upstream_connect_time" '
                'uht="$upstream_header_time" urt="$upstream_response_time"';
```

### 性能监控
```nginx
log_format perf '$request_id $request_time $upstream_response_time '
                '$upstream_connect_time $upstream_header_time';
```

### 安全日志
```nginx
log_format security '$remote_addr $request_method $request_uri '
                    '$status $ssl_protocol $ssl_cipher';
```

---

*文档生成时间：2026-04-03*
*基于 nginx 1.24+ 版本*
