# NGINX 日志与监控指南

## 1. 访问日志配置

### 基础配置

```nginx
http {
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';

    access_log /var/log/nginx/access.log main;
}
```

### 日志格式定义

```nginx
log_format name [escape=default|json|none] string ...;
```

**转义选项**：
- `default`：转义 `"`、`\`、控制字符为 `\xXX`
- `json`：按 JSON 标准转义
- `none`：禁用转义

### 预定义格式

**combined（默认）**：
```nginx
log_format combined '$remote_addr - $remote_user [$time_local] '
                    '"$request" $status $body_bytes_sent '
                    '"$http_referer" "$http_user_agent"';
```

### 自定义格式

```nginx
# JSON 格式
log_format json_combined escape=json
    '{'
        '"time":"$time_iso8601",'
        '"remote_addr":"$remote_addr",'
        '"method":"$request_method",'
        '"uri":"$request_uri",'
        '"status":$status,'
        '"body_bytes_sent":$body_bytes_sent,'
        '"request_time":$request_time,'
        '"http_referrer":"$http_referer",'
        '"http_user_agent":"$http_user_agent"'
    '}';

# 详细性能日志
log_format performance
    '$remote_addr - $remote_user [$time_local] '
    '"$request" $status $body_bytes_sent '
    'rt=$request_time uct="$upstream_connect_time" '
    'uht="$upstream_header_time" urt="$upstream_response_time"';

# 包含缓存状态
log_format cache '$remote_addr - $remote_user [$time_local] '
                 '"$request" $status $upstream_cache_status';
```

---

## 2. access_log 指令

### 语法

```nginx
access_log path [format [buffer=size] [gzip[=level]] [flush=time] [if=condition]];
access_log off;
```

### 配置示例

```nginx
http {
    # 基础配置
    access_log /var/log/nginx/access.log main;

    # 带缓冲
    access_log /var/log/nginx/access.log main buffer=32k;

    # 带压缩
    access_log /var/log/nginx/access.log.gz main gzip=6 buffer=32k flush=5m;

    # 条件日志
    map $status $loggable {
        ~^[23]  0;
        default 1;
    }
    access_log /var/log/nginx/access.log main if=$loggable;

    # 多日志输出
    access_log /var/log/nginx/access.log main;
    access_log /var/log/nginx/access_json.log json_combined;

    # Syslog
    access_log syslog:server=192.168.1.1:514,facility=local7,tag=nginx main;

    # 禁用日志
    location /health {
        access_log off;
    }
}
```

### 变量路径

```nginx
server {
    root /var/www/$host;
    access_log /var/log/nginx/$host/access.log;

    # 缓存变量路径的日志文件描述符
    open_log_file_cache max=1000 inactive=20s valid=1m min_uses=2;
}
```

---

## 3. 错误日志配置

### 语法

```nginx
error_log file [level];
error_log syslog:server=address [level];
```

### 日志级别

| 级别 | 说明 | 使用场景 |
|------|------|----------|
| `debug` | 调试信息 | 开发调试 |
| `info` | 信息 | 一般信息 |
| `notice` | 通知 | 重要事件 |
| `warn` | 警告 | 潜在问题 |
| `error` | 错误 | 操作错误 |
| `crit` | 严重 | 严重错误 |
| `alert` | 警报 | 需立即处理 |
| `emerg` | 紧急 | 系统不可用 |

### 配置示例

```nginx
error_log /var/log/nginx/error.log warn;

# Server 级别覆盖
server {
    error_log /var/log/nginx/example.com/error.log notice;
}

# 仅记录特定客户端的调试日志
events {
    debug_connection 192.168.1.1;
    debug_connection 192.168.10.0/24;
}
```

---

## 4. 调试日志

### 启用调试

**编译时**：
```bash
./configure --with-debug
```

**运行时**：
```nginx
error_log /var/log/nginx/debug.log debug;
```

### 验证调试支持

```bash
nginx -V | grep -- --with-debug
```

### 内存缓冲调试

```nginx
error_log memory:32m debug;
```

**提取日志**：
```bash
# GDB
gdb -p $(cat /var/run/nginx.pid)
set $log = ngx_cycle->log
while $log->writer != ngx_log_memory_writer
    set $log = $log->next
end
set $buf = (ngx_log_memory_buf_t *) $log->wdata
dump binary memory debug.log $buf->start $buf->end

# LLDB
lldb -p $(cat /var/run/nginx.pid)
expr ngx_log_t *$log = ngx_cycle->log
expr while ($log->writer != ngx_log_memory_writer) { $log = $log->next; }
expr ngx_log_memory_buf_t *$buf = (ngx_log_memory_buf_t *) $log->wdata
memory read --force --outfile debug.log --binary $buf->start $buf->end
```

---

## 5. 条件日志

### 基于状态码

```nginx
map $status $loggable {
    ~^[23]  0;    # 2xx, 3xx 不记录
    default 1;
}

access_log /var/log/nginx/access.log main if=$loggable;
```

### 基于请求路径

```nginx
map $uri $loggable {
    /health      0;
    /favicon.ico 0;
    ~^/static/   0;
    default      1;
}

access_log /var/log/nginx/access.log main if=$loggable;
```

### 基于响应时间

```nginx
map $request_time $slow_request {
    default     0;
    "~^[0-9]+\.[5-9]"  1;  # 大于 0.5 秒
}

access_log /var/log/nginx/slow.log main if=$slow_request;
```

---

## 6. 日志轮转

### logrotate 配置

```bash
# /etc/logrotate.d/nginx
/var/log/nginx/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 nginx adm
    sharedscripts
    postrotate
        [ -f /var/run/nginx.pid ] && kill -USR1 $(cat /var/run/nginx.pid)
    endscript
}
```

### 手动轮转

```bash
# 重命名日志文件
mv /var/log/nginx/access.log /var/log/nginx/access.log.1

# 重新打开日志文件
nginx -s reopen
# 或
kill -USR1 $(cat /var/run/nginx.pid)

# 压缩旧日志
gzip /var/log/nginx/access.log.1
```

---

## 7. 内置变量

### 请求相关

| 变量 | 说明 |
|------|------|
| `$request` | 完整原始请求行 |
| `$request_method` | 请求方法 |
| `$request_uri` | 完整请求 URI（含参数） |
| `$uri` | 规范化后的 URI |
| `$args` | 查询参数 |
| `$arg_name` | 指定参数值 |
| `$request_body` | 请求体内容 |
| `$request_length` | 请求长度（含行、头、体） |
| `$request_time` | 请求处理时间（秒，毫秒精度） |

### 客户端相关

| 变量 | 说明 |
|------|------|
| `$remote_addr` | 客户端 IP |
| `$remote_port` | 客户端端口 |
| `$remote_user` | 认证用户名 |
| `$http_user_agent` | User-Agent |
| `$http_referer` | Referer |

### 响应相关

| 变量 | 说明 |
|------|------|
| `$status` | 响应状态码 |
| `$body_bytes_sent` | 发送的字节数（不含头） |
| `$bytes_sent` | 发送的总字节数 |
| `$sent_http_name` | 响应头字段 |

### 上游相关

| 变量 | 说明 |
|------|------|
| `$upstream_addr` | 上游服务器地址 |
| `$upstream_response_time` | 上游响应时间 |
| `$upstream_connect_time` | 连接上游时间 |
| `$upstream_header_time` | 接收响应头时间 |
| `$upstream_cache_status` | 缓存状态 |

### 时间相关

| 变量 | 说明 |
|------|------|
| `$time_local` | 本地时间 |
| `$time_iso8601` | ISO 8601 格式时间 |
| `$msec` | 当前时间（秒，毫秒精度） |

---

## 8. 状态监控

### stub_status

```nginx
location /nginx_status {
    stub_status;
    allow 127.0.0.1;
    allow 10.0.0.0/8;
    deny all;
}
```

**输出示例**：
```
Active connections: 10
server accepts handled requests
 100 100 200
Reading: 0 Writing: 1 Waiting: 9
```

**指标说明**：
- `Active connections`：当前活跃连接数
- `accepts`：接受的连接总数
- `handled`：处理的连接总数
- `requests`：请求总数
- `Reading`：正在读取请求的连接数
- `Writing`：正在写入响应的连接数
- `Waiting`：等待下一个请求的连接数（keepalive）

---

## 9. 监控集成

### Prometheus Exporter

使用 `nginx-prometheus-exporter`：

```yaml
# docker-compose.yml
version: '3'
services:
  nginx:
    image: nginx:latest
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf

  nginx-exporter:
    image: nginx/nginx-prometheus-exporter:latest
    ports:
      - "9113:9113"
    command:
      - -nginx.scrape-uri=http://nginx/nginx_status
```

### ELK 集成

```nginx
log_format json_log escape=json
    '{'
        '"@timestamp":"$time_iso8601",'
        '"remote_addr":"$remote_addr",'
        '"request":"$request",'
        '"status":$status,'
        '"body_bytes_sent":$body_bytes_sent,'
        '"request_time":$request_time,'
        '"upstream_response_time":"$upstream_response_time",'
        '"http_user_agent":"$http_user_agent"'
    '}';

access_log /var/log/nginx/access.log json_log;
```

---

## 10. 日志分析

### 常用分析命令

```bash
# 请求数统计
awk '{print $1}' /var/log/nginx/access.log | sort | uniq -c | sort -rn | head -10

# 状态码统计
awk '{print $9}' /var/log/nginx/access.log | sort | uniq -c | sort -rn

# 响应时间分析
awk '{print $NF}' /var/log/nginx/access.log | awk '{sum+=$1; count++} END {print "avg:", sum/count, "total:", count}'

# 慢请求
awk '$NF > 1 {print}' /var/log/nginx/access.log

# 404 请求
awk '$9 == 404 {print $7}' /var/log/nginx/access.log | sort | uniq -c | sort -rn | head -10

# 每小时请求量
awk '{print substr($4, 14, 2)}' /var/log/nginx/access.log | sort | uniq -c
```

### GoAccess 实时分析

```bash
# 安装
apt install goaccess

# 实时分析
goaccess /var/log/nginx/access.log -o /var/www/html/report.html --real-time-html --log-format=COMBINED
```

---

## 11. 日志最佳实践

### 日志级别建议

| 环境 | 错误日志级别 | 访问日志 |
|------|-------------|----------|
| 开发 | debug | 详细 |
| 测试 | notice | 标准 |
| 生产 | warn | 条件记录 |

### 性能考虑

```nginx
# 高流量场景
location / {
    access_log off;              # 禁用访问日志
    # 或使用缓冲
    access_log /var/log/nginx/access.log main buffer=64k flush=5m;
}

# 静态资源不记录
location ~* \.(css|js|png|jpg|gif|ico)$ {
    access_log off;
}

# 健康检查不记录
location /health {
    access_log off;
    return 200 "OK";
}
```