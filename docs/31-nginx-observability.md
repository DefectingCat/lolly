# NGINX OpenTelemetry 可观测性指南

本文档介绍如何在 NGINX 中使用 OpenTelemetry 模块实现分布式追踪和可观测性。

## 目录

1. [OpenTelemetry 概述](#opentelemetry-概述)
2. [模块指令参考](#模块指令参考)
3. [分布式追踪配置](#分布式追踪配置)
4. [与 Jaeger/Zipkin 集成](#与-jaegerzipkin-集成)
5. [自定义属性和事件](#自定义属性和事件)
6. [完整配置示例](#完整配置示例)
7. [最佳实践](#最佳实践)

---

## OpenTelemetry 概述

### 什么是 OpenTelemetry

OpenTelemetry 是一个开源的可观测性框架，提供标准化的 API、库和工具来收集分布式追踪、指标和日志数据。它由 Cloud Native Computing Foundation (CNCF) 托管，是 Prometheus、Jaeger 和 OpenCensus 等项目合并后的统一解决方案。

### 核心概念

| 概念 | 描述 |
|------|------|
| **Trace** | 分布式追踪，表示请求在系统中的完整调用链路 |
| **Span** | 追踪中的基本工作单元，包含操作名称、起止时间、属性等 |
| **Context** | 追踪上下文，用于在服务间传播追踪信息（traceparent/tracestate） |
| **Resource** | 描述产生遥测数据的实体（如服务名称、版本、主机） |
| **Exporter** | 将遥测数据发送到后端存储（如 OTLP、gRPC） |

### 架构流程

```
┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐
│  Client │───▶│  NGINX  │───▶│ Backend │───▶│Database │
└─────────┘    └────┬────┘    └─────────┘    └─────────┘
                    │
                    ▼
           ┌────────────────┐
           │ ngx_otel_module │
           └───────┬────────┘
                   │
                   ▼
           ┌───────────────┐    ┌───────────┐    ┌──────────┐
           │OTEL Collector │───▶│   Jaeger  │    │  Zipkin  │
           └───────────────┘    └───────────┘    └──────────┘
```

### 模块版本要求

- NGINX Plus R28 或更高版本
- `ngx_otel_module` 动态模块（从源码编译或 NGINX Plus 包含）

---

## 模块指令参考

### otel_exporter

配置 OpenTelemetry 数据导出参数。

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `otel_exporter` | `{ ... }` | — | `http` |

**子指令：**

| 子指令 | 语法 | 默认值 | 描述 |
|--------|------|--------|------|
| `endpoint` | `[(http\|https)://]host:port;` | — | OTLP/gRPC 端点地址 |
| `trusted_certificate` | `path;` | 系统 CA | PEM 格式 CA 证书文件（v0.1.2+） |
| `header` | `name value;` | — | 自定义 HTTP 请求头 |
| `interval` | `time;` | `5s` | 导出最大间隔时间 |
| `batch_size` | `number;` | `512` | 每批次最大 Span 数量 |
| `batch_count` | `number;` | `4` | 每个 worker 的待处理批次数 |

**示例：**

```nginx
http {
    otel_exporter {
        endpoint otel-collector:4317;
        interval 5s;
        batch_size 512;
        batch_count 4;
        trusted_certificate /etc/nginx/certs/ca.pem;
        header X-API-Key secret_key;
    }
}
```

### otel_service_name

设置 OTel Resource 的 `service.name` 属性。

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `otel_service_name` | `name;` | `unknown_service:nginx` | `http` |

**示例：**

```nginx
http {
    otel_service_name nginx-gateway;
}
```

### otel_resource_attr

设置自定义 OTel Resource 属性（v0.1.2+）。

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `otel_resource_attr` | `name value;` | — | `http` |

**示例：**

```nginx
http {
    otel_resource_attr deployment.environment production;
    otel_resource_attr service.version 1.2.3;
    otel_resource_attr host.name $hostname;
}
```

### otel_trace

启用或禁用 OpenTelemetry 追踪。

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `otel_trace` | `on \| off \| $variable;` | `off` | `http`, `server`, `location` |

**示例：**

```nginx
http {
    otel_trace off;

    server {
        listen 80;
        otel_trace on;

        location /api {
            otel_trace on;
        }

        location /health {
            otel_trace off;  # 健康检查不记录
        }
    }
}
```

### otel_trace_context

配置 traceparent/tracestate 头的传播方式。

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `otel_trace_context` | `extract \| inject \| propagate \| ignore;` | `ignore` | `http`, `server`, `location` |

**选项说明：**

| 值 | 描述 |
|----|------|
| `extract` | 从入站请求中提取追踪上下文，继承上游标识符 |
| `inject` | 向出站请求注入新的追踪上下文，覆盖现有上下文 |
| `propagate` | 更新现有上下文（先 extract 再 inject），保持追踪链完整 |
| `ignore` | 忽略上下文头处理 |

**示例：**

```nginx
server {
    location / {
        # 作为入口网关，注入新追踪上下文
        otel_trace_context inject;
        proxy_pass http://backend;
    }

    location /api/ {
        # 作为中间代理，传播上游追踪上下文
        otel_trace_context propagate;
        proxy_pass http://api_backend;
    }
}
```

### otel_span_name

定义 OTel Span 的名称。

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `otel_span_name` | `name;` | location 名称 | `http`, `server`, `location` |

**示例：**

```nginx
server {
    location /api/users {
        otel_span_name "GET /api/users";
        # 或使用变量
        otel_span_name "$request_method $uri";
    }
}
```

### otel_span_attr

添加自定义 OTel Span 属性。

| 指令 | 语法 | 默认值 | 上下文 |
|------|------|--------|--------|
| `otel_span_attr` | `name value;` | — | `http`, `server`, `location` |

**示例：**

```nginx
server {
    location /api/ {
        otel_span_attr http.route "/api/*";
        otel_span_attr user.id $remote_user;
        otel_span_attr client.ip $remote_addr;
    }
}
```

### 嵌入式变量

| 变量 | 描述 |
|------|------|
| `$otel_trace_id` | 追踪标识符 |
| `$otel_span_id` | 当前 Span 标识符 |
| `$otel_parent_id` | 父 Span 标识符 |
| `$otel_parent_sampled` | 父 Span 的采样标志（`1` 或 `0`） |

---

## 分布式追踪配置

### Trace 上下文传播

追踪上下文传播是分布式追踪的核心，确保请求在多个服务间保持相同的追踪标识。

#### W3C Trace Context 标准

NGINX 使用 W3C Trace Context 标准：
- **traceparent**: `00-{trace-id}-{parent-id}-{flags}`
- **tracestate**: 厂商特定的上下文信息

```
Traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
           │  │   │                              │                  │
           │  │   │                              │                  └── 标志位( sampled: 01)
           │  │   │                              └── 父 Span ID
           │  │   └── Trace ID
           │  └── 版本
           └── 固定前缀
```

#### 传播模式配置

**场景 1: 边缘网关（追踪入口）**

```nginx
http {
    otel_service_name nginx-edge-gateway;
    otel_trace on;

    server {
        listen 80;
        server_name api.example.com;

        location / {
            # 注入新的追踪上下文
            otel_trace_context inject;
            
            # 将追踪 ID 传递给后端
            proxy_set_header X-Trace-ID $otel_trace_id;
            proxy_set_header X-Span-ID $otel_span_id;
            
            proxy_pass http://backend_cluster;
        }
    }
}
```

**场景 2: 中间代理（追踪传播）**

```nginx
server {
    listen 8080;
    
    location / {
        # 传播上游追踪上下文
        otel_trace_context propagate;
        
        # 将追踪头传递给下游服务
        proxy_set_header traceparent $http_traceparent;
        proxy_set_header tracestate $http_tracestate;
        
        proxy_pass http://internal_services;
    }
}
```

**场景 3: 混合模式**

```nginx
server {
    location /public/ {
        # 公共 API: 创建新追踪
        otel_trace_context inject;
        proxy_pass http://public_backend;
    }

    location /internal/ {
        # 内部服务: 传播已有追踪
        otel_trace_context propagate;
        proxy_pass http://internal_backend;
    }

    location /health {
        # 健康检查: 忽略追踪
        otel_trace off;
        return 200 "healthy\n";
    }
}
```

### Span 配置

#### 标准 Span 属性

NGINX 自动记录的 Span 属性：

| 属性 | 描述 | 示例值 |
|------|------|--------|
| `http.method` | HTTP 方法 | GET, POST, PUT |
| `http.url` | 请求 URL | `https://api.example.com/users` |
| `http.scheme` | 协议 | http, https |
| `http.host` | 主机名 | `api.example.com` |
| `http.status_code` | 响应状态码 | 200, 404, 500 |
| `http.user_agent` | 用户代理 | Mozilla/5.0... |
| `http.request_content_length` | 请求体大小 | 1024 |
| `http.response_content_length` | 响应体大小 | 2048 |
| `net.peer.ip` | 客户端 IP | 192.168.1.100 |
| `net.peer.port` | 客户端端口 | 54321 |

#### 自定义 Span 名称

```nginx
map $request_method $span_name {
    default "$request_method $uri";
    GET     "get_request";
    POST    "create_resource";
}

server {
    location /api/ {
        otel_span_name $span_name;
        proxy_pass http://backend;
    }
}
```

#### 条件性 Span 属性

```nginx
map $status $error_type {
    ~^[45]  "client_or_server_error";
    default "";
}

server {
    location / {
        otel_span_attr error.class $error_type;
        otel_span_attr request.id $request_id;
        otel_span_attr tenant.id $http_x_tenant_id;
        
        proxy_pass http://backend;
    }
}
```

### 采样策略

采样控制追踪数据的收集量，平衡可观测性和性能开销。

#### 采样类型

| 采样类型 | 描述 | 使用场景 |
|----------|------|----------|
| **Head-Based** | 在追踪开始时决定采样 | 低延迟、低资源开销 |
| **Tail-Based** | 基于完整追踪数据决定 | 捕获错误/慢请求 |
| **Parent-Based** | 继承父 Span 的采样决定 | 保持追踪完整性 |

#### 配置示例

**1. 始终采样（开发/测试环境）**

```nginx
http {
    otel_trace on;
    # 所有请求都记录
}
```

**2. 比例采样（基于变量）**

```nginx
# 使用 Lua 或外部模块实现比例采样
# 这里展示基于 Nginx 变量的实现

split_clients "$remote_addr$request_id" $trace_sampled {
    10%     "1";   # 10% 采样率
    *       "0";   # 90% 不采样
}

server {
    location / {
        otel_trace $trace_sampled;
        proxy_pass http://backend;
    }
}
```

**3. 基于请求特征采样**

```nginx
map $uri $should_trace {
    default              "0";
    ~*\.html$            "1";  # 采样 HTML 页面
    /api/critical/       "1";  # 采样关键 API
    /api/payment/        "1";  # 采样支付相关
}

map $http_x_debug $force_trace {
    default  "";
    true      "1";
}

server {
    location / {
        # 优先使用 debug header，其次基于 URI
        otel_trace $force_trace$should_trace;
        proxy_pass http://backend;
    }
}
```

**4. 错误/慢请求采样（结合 OpenTelemetry Collector）**

```yaml
# otel-collector-config.yaml
processors:
  tail_sampling:
    policies:
      - name: slow_requests
        type: latency
        latency: {threshold_ms: 500}
      - name: errors
        type: status_code
        status_code: {status_codes: [500, 502, 503, 504]}
      - name: probabilistic
        type: probabilistic
        probabilistic: {sampling_percentage: 10}
```

---

## 与 Jaeger/Zipkin 集成

### Jaeger 集成

#### 方法 1: Jaeger 原生 OTLP（推荐）

Jaeger 1.35+ 原生支持 OTLP 协议。

**docker-compose.yaml:**

```yaml
version: "3.8"

services:
  jaeger:
    image: jaegertracing/all-in-one:1.60.0
    container_name: jaeger
    ports:
      - "16686:16686"   # Jaeger UI
      - "4317:4317"     # OTLP gRPC
      - "4318:4318"     # OTLP HTTP
    environment:
      - COLLECTOR_OTLP_ENABLED=true
    networks:
      - observability

  nginx:
    image: nginx:alpine
    container_name: nginx
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    ports:
      - "80:80"
    depends_on:
      - jaeger
    networks:
      - observability

networks:
  observability:
    driver: bridge
```

**nginx.conf:**

```nginx
load_module modules/ngx_otel_module.so;

events {
    worker_connections 1024;
}

http {
    # OTLP 导出器配置
    otel_exporter {
        endpoint jaeger:4317;
        interval 5s;
        batch_size 512;
    }

    # 服务标识
    otel_service_name nginx-gateway;
    otel_resource_attr deployment.environment production;
    otel_resource_attr host.name $hostname;

    # 启用追踪
    otel_trace on;

    server {
        listen 80;
        server_name localhost;

        location / {
            otel_trace_context inject;
            otel_span_name "$request_method $uri";
            
            # 传递追踪上下文给后端
            proxy_set_header traceparent $http_traceparent;
            proxy_set_header tracestate $http_tracestate;
            
            proxy_pass http://backend;
        }

        location /jaeger {
            # 返回当前追踪信息（调试用途）
            default_type application/json;
            return 200 '{"trace_id":"$otel_trace_id","span_id":"$otel_span_id"}';
        }
    }
}
```

#### 方法 2: 通过 OpenTelemetry Collector

用于需要额外处理的场景（过滤、转换、批处理）。

**docker-compose.yaml:**

```yaml
version: "3.8"

services:
  otel-collector:
    image: otel/opentelemetry-collector-contrib:0.117.0
    container_name: otel-collector
    command: ["--config=/etc/otel-collector-config.yaml"]
    volumes:
      - ./otel-collector-config.yaml:/etc/otel-collector-config.yaml:ro
    ports:
      - "4317:4317"     # OTLP gRPC
      - "4318:4318"     # OTLP HTTP
      - "9464:9464"     # Prometheus metrics
    networks:
      - observability

  jaeger:
    image: jaegertracing/all-in-one:1.60.0
    container_name: jaeger
    ports:
      - "16686:16686"
    environment:
      - COLLECTOR_OTLP_ENABLED=true
    networks:
      - observability

  nginx:
    image: nginx:alpine
    container_name: nginx
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    ports:
      - "80:80"
    depends_on:
      - otel-collector
    networks:
      - observability

networks:
  observability:
    driver: bridge
```

**otel-collector-config.yaml:**

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 1s
    send_batch_size: 1024
  
  resource:
    attributes:
      - key: environment
        value: production
        action: upsert

  tail_sampling:
    policies:
      - name: slow_requests
        type: latency
        latency: {threshold_ms: 500}
      - name: errors
        type: status_code
        status_code: {status_codes: [500, 502, 503, 504]}

exporters:
  otlp/jaeger:
    endpoint: jaeger:4317
    tls:
      insecure: true
  
  debug:
    verbosity: detailed

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, resource, tail_sampling]
      exporters: [otlp/jaeger, debug]
```

### Zipkin 集成

#### 方法 1: 通过 OpenTelemetry Collector

**docker-compose.yaml:**

```yaml
version: "3.8"

services:
  zipkin:
    image: openzipkin/zipkin:3
    container_name: zipkin
    ports:
      - "9411:9411"
    environment:
      - STORAGE_TYPE=mem
    networks:
      - observability

  otel-collector:
    image: otel/opentelemetry-collector-contrib:0.117.0
    container_name: otel-collector
    command: ["--config=/etc/otel-collector-config.yaml"]
    volumes:
      - ./otel-collector-config.yaml:/etc/otel-collector-config.yaml:ro
    ports:
      - "4317:4317"
      - "4318:4318"
    depends_on:
      - zipkin
    networks:
      - observability

  nginx:
    image: nginx:alpine
    container_name: nginx
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    ports:
      - "80:80"
    depends_on:
      - otel-collector
    networks:
      - observability

networks:
  observability:
    driver: bridge
```

**otel-collector-config.yaml:**

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 1s
    send_batch_size: 1024

exporters:
  zipkin:
    endpoint: http://zipkin:9411/api/v2/spans
    format: json
    
  debug:
    verbosity: detailed

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [zipkin, debug]
```

#### 方法 2: Zipkin 直接接收

如果您的系统已使用 Zipkin，可以让 Collector 同时接收 OTLP 和 Zipkin 格式。

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
  
  zipkin:
    endpoint: 0.0.0.0:9411

processors:
  batch:

exporters:
  zipkin:
    endpoint: http://zipkin:9411/api/v2/spans

service:
  pipelines:
    traces:
      receivers: [otlp, zipkin]
      processors: [batch]
      exporters: [zipkin]
```

---

## 自定义属性和事件

### 自定义 Span 属性

#### 静态属性

```nginx
http {
    otel_resource_attr service.namespace ecommerce;
    otel_resource_attr service.version 2.1.0;
    
    server {
        location /api/ {
            otel_span_attr api.version v1;
            otel_span_attr team backend;
        }
    }
}
```

#### 动态属性（使用变量）

```nginx
map $request_time $latency_bucket {
    ~^0\.[0-4]   "fast";
    ~^0\.[5-9]   "medium";
    default      "slow";
}

server {
    location / {
        otel_span_attr http.latency_bucket $latency_bucket;
        otel_span_attr request.size $request_length;
        otel_span_attr response.size $bytes_sent;
        otel_span_attr upstream.addr $upstream_addr;
        otel_span_attr upstream.response_time $upstream_response_time;
        
        proxy_pass http://backend;
    }
}
```

#### 条件属性

```nginx
map $upstream_status $upstream_error {
    ~^[45]  "true";
    default "false";
}

map $upstream_cache_status $cache_hit {
    HIT     "true";
    default "false";
}

server {
    location / {
        otel_span_attr upstream.error $upstream_error;
        otel_span_attr cache.hit $cache_hit;
        otel_span_attr cache.status $upstream_cache_status;
        
        proxy_pass http://backend;
        proxy_cache my_cache;
    }
}
```

### 业务属性

```nginx
server {
    location /api/orders {
        # 业务相关属性
        otel_span_attr business.domain orders;
        otel_span_attr business.criticality high;
        otel_span_attr business.region $geoip_country_code;
        
        # 用户相关属性（注意：避免 PII）
        otel_span_attr user.type $http_x_user_type;
        otel_span_attr user.tier $http_x_user_tier;
        
        proxy_pass http://order_service;
    }
}
```

### 使用 Lua 扩展（需要 lua-nginx-module）

```nginx
server {
    location / {
        access_by_lua_block {
            local otel = require("opentelemetry")
            local span = otel.get_current_span()
            
            -- 添加自定义属性
            span:set_attribute("custom.timestamp", ngx.now())
            span:set_attribute("custom.request_hash", ngx.md5(ngx.var.request_uri))
            
            -- 添加事件
            span:add_event("request_processing_started", {
                ["http.method"] = ngx.var.request_method,
                ["client.ip"] = ngx.var.remote_addr
            })
        }
        
        proxy_pass http://backend;
    }
}
```

---

## 完整配置示例

### 示例 1: 基础配置

```nginx
# 加载动态模块
load_module modules/ngx_otel_module.so;

user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log notice;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # OpenTelemetry 导出器配置
    otel_exporter {
        endpoint otel-collector:4317;
        interval 5s;
        batch_size 512;
        batch_count 4;
    }

    # 服务标识
    otel_service_name nginx-proxy;
    otel_resource_attr deployment.environment production;
    otel_resource_attr host.name $hostname;

    # 全局启用追踪
    otel_trace on;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for" '
                    'trace_id=$otel_trace_id span_id=$otel_span_id';

    access_log /var/log/nginx/access.log main;

    sendfile on;
    keepalive_timeout 65;

    upstream backend {
        server backend1:8080 weight=5;
        server backend2:8080 weight=5;
        keepalive 32;
    }

    server {
        listen 80;
        server_name localhost;

        # 健康检查：禁用追踪
        location /health {
            otel_trace off;
            access_log off;
            return 200 "healthy\n";
        }

        # 静态资源：采样
        location /static/ {
            otel_trace $http_x_trace_sampled;
            alias /var/www/static/;
            expires 1d;
        }

        # API 请求：完整追踪
        location /api/ {
            otel_trace on;
            otel_trace_context propagate;
            otel_span_name "$request_method $uri";
            
            otel_span_attr http.route /api/*;
            otel_span_attr api.version v1;
            otel_span_attr request.id $request_id;
            
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Request-ID $request_id;
            
            # 传递追踪上下文
            proxy_set_header traceparent $http_traceparent;
            proxy_set_header tracestate $http_tracestate;
            
            proxy_pass http://backend;
        }

        # 默认位置
        location / {
            otel_trace_context inject;
            proxy_pass http://backend;
        }
    }
}
```

### 示例 2: 多环境配置

```nginx
load_module modules/ngx_otel_module.so;

events {
    worker_connections 1024;
}

http {
    # 根据环境变量配置
    env NGINX_ENV;
    env OTEL_ENDPOINT;

    # 动态采样率配置
    split_clients "$remote_addr$request_id" $trace_sampled {
        10%     "1";
        *       "0";
    }

    map $http_x_b3_sampled $b3_sampled {
        default  "";
        "1"      "1";
        "0"      "";
        "true"   "1";
        "false"  "";
        "d"      "1";
    }

    map $b3_sampled$trace_sampled $final_trace {
        default     "0";
        ~.*1.*      "1";
    }

    # OTLP 导出器
    otel_exporter {
        endpoint ${OTEL_ENDPOINT};
        interval 5s;
        batch_size 512;
    }

    otel_service_name nginx-${NGINX_ENV};
    otel_resource_attr deployment.environment ${NGINX_ENV};

    # 生产环境：按比例采样
    # 测试环境：全量采样
    otel_trace ${NGINX_ENV} == "prod" ? $final_trace : on;

    # 上游配置
    upstream api_backend {
        server api1.internal:8080;
        server api2.internal:8080;
    }

    upstream web_backend {
        server web1.internal:8080;
        server web2.internal:8080;
    }

    # API 网关
    server {
        listen 8080;
        server_name api.example.com;

        location / {
            otel_trace_context propagate;
            otel_span_name "api:$request_method $uri";
            
            otel_span_attr upstream.service api;
            otel_span_attr rate.limit.bucket $limit_req_status;
            
            proxy_pass http://api_backend;
        }
    }

    # Web 网关
    server {
        listen 80;
        server_name www.example.com;

        location / {
            otel_trace_context inject;
            otel_span_name "web:$request_method $uri";
            
            otel_span_attr upstream.service web;
            otel_span_attr cache.status $upstream_cache_status;
            
            proxy_pass http://web_backend;
            proxy_cache web_cache;
        }
    }
}
```

### 示例 3: 微服务网关配置

```nginx
load_module modules/ngx_otel_module.so;

events {
    worker_connections 4096;
}

http {
    # OpenTelemetry 配置
    otel_exporter {
        endpoint otel-collector:4317;
        interval 3s;
        batch_size 256;
        header X-Scope-OrgID tenant-1;
    }

    otel_service_name nginx-microgateway;
    otel_resource_attr service.namespace platform;
    otel_resource_attr service.version 1.0.0;
    otel_resource_attr deployment.environment production;

    # 追踪配置
    otel_trace on;

    # 日志格式包含追踪信息
    log_format trace '$remote_addr - $remote_user [$time_iso8601] '
                     '"$request" $status $body_bytes_sent '
                     '"$http_referer" "$http_user_agent" '
                     '"trace_id":"$otel_trace_id",'
                     '"span_id":"$otel_span_id",'
                     '"parent_id":"$otel_parent_id"';

    access_log /var/log/nginx/access.log trace;

    # 服务发现（使用 resolver）
    resolver 127.0.0.11 valid=30s;

    # 服务定义
    upstream user_service {
        server user-service:8080 resolve;
        keepalive 64;
    }

    upstream order_service {
        server order-service:8080 resolve;
        keepalive 64;
    }

    upstream inventory_service {
        server inventory-service:8080 resolve;
        keepalive 64;
    }

    # 通用追踪配置
    map $request_method $trace_operation {
        GET     "read";
        POST    "create";
        PUT     "update";
        DELETE  "delete";
        PATCH   "patch";
        default "unknown";
    }

    server {
        listen 80;
        server_name gateway.internal;

        # 追踪上下文传播
        otel_trace_context propagate;

        # User Service
        location /api/users/ {
            otel_span_name "users:$trace_operation";
            otel_span_attr service.name user-service;
            otel_span_attr service.operation $trace_operation;
            otel_span_attr service.resource users;
            
            proxy_pass http://user_service/;
            proxy_set_header traceparent $http_traceparent;
            proxy_set_header tracestate $http_tracestate;
        }

        # Order Service
        location /api/orders/ {
            otel_span_name "orders:$trace_operation";
            otel_span_attr service.name order-service;
            otel_span_attr service.operation $trace_operation;
            otel_span_attr service.resource orders;
            
            proxy_pass http://order_service/;
            proxy_set_header traceparent $http_traceparent;
            proxy_set_header tracestate $http_tracestate;
        }

        # Inventory Service
        location /api/inventory/ {
            otel_span_name "inventory:$trace_operation";
            otel_span_attr service.name inventory-service;
            otel_span_attr service.operation $trace_operation;
            otel_span_attr service.resource inventory;
            
            proxy_pass http://inventory_service/;
            proxy_set_header traceparent $http_traceparent;
            proxy_set_header tracestate $http_tracestate;
        }

        # 健康检查（无追踪）
        location /health {
            otel_trace off;
            access_log off;
            return 200 '{"status":"healthy","service":"nginx"}';
        }

        # 追踪信息端点（调试）
        location /debug/trace {
            otel_trace on;
            default_type application/json;
            return 200 '{
                "trace_id": "$otel_trace_id",
                "span_id": "$otel_span_id",
                "parent_id": "$otel_parent_id",
                "sampled": "$otel_parent_sampled"
            }';
        }
    }
}
```

### 示例 4: Kubernetes 环境配置

```nginx
load_module modules/ngx_otel_module.so;

events {
    worker_connections 1024;
}

http {
    # 从环境变量读取 K8s 信息
    env KUBERNETES_NAMESPACE;
    env KUBERNETES_POD_NAME;
    env KUBERNETES_NODE_NAME;
    env OTEL_COLLECTOR_SERVICE;

    # OTLP 导出器
    otel_exporter {
        endpoint ${OTEL_COLLECTOR_SERVICE}:4317;
        interval 5s;
        batch_size 512;
    }

    # 丰富的资源属性
    otel_service_name nginx-ingress;
    otel_resource_attr k8s.namespace.name ${KUBERNETES_NAMESPACE};
    otel_resource_attr k8s.pod.name ${KUBERNETES_POD_NAME};
    otel_resource_attr k8s.node.name ${KUBERNETES_NODE_NAME};
    otel_resource_attr host.name ${KUBERNETES_POD_NAME};

    # 启用追踪
    otel_trace on;

    # 上游配置（K8s Service）
    resolver kube-dns.kube-system.svc.cluster.local valid=10s;

    server {
        listen 80;

        location / {
            otel_trace_context propagate;
            otel_span_name "$request_method $uri";
            
            otel_span_attr k8s.destination.service $proxy_host;
            otel_span_attr k8s.destination.namespace ${KUBERNETES_NAMESPACE};
            
            # 传递 K8s 相关的追踪头
            proxy_set_header X-Request-ID $request_id;
            proxy_set_header traceparent $http_traceparent;
            proxy_set_header tracestate $http_tracestate;
            
            proxy_pass http://backend-service;
        }
    }
}
```

---

## 最佳实践

### 1. 采样策略

**生产环境建议：**

```nginx
# 使用 Head-Based 采样降低开销
split_clients "$request_id" $trace_decision {
    5%      "1";   # 5% 基础采样
    *       "";
}

# 关键路径始终采样
map $uri $is_critical {
    default              "";
    ~*payment            "1";
    ~*order              "1";
    ~*auth               "1";
}

map $trace_decision$is_critical $should_trace {
    default     "0";
    ~.*1.*      "1";
}

otel_trace $should_trace;
```

**关键原则：**
- 错误率高的服务：提高采样率
- 高流量服务：降低采样率（0.1% - 1%）
- 关键业务路径：全量采样
- 使用 Parent-Based 采样保持追踪链完整

### 2. 敏感数据处理

**禁止在 Span 属性中包含：**
- 密码、API Key
- 信用卡号、身份证号
- 个人身份信息 (PII)
- 会话令牌

**安全实践：**

```nginx
# 正确：使用安全的标识符
otel_span_attr user.id $http_x_user_id;           # 用户 ID
otel_span_attr session.hash $cookie_session_hash; # 会话哈希

# 错误：不要记录敏感信息
# otel_span_attr user.email $http_x_user_email;   # 禁止！
# otel_span_attr auth.token $http_authorization;   # 禁止！

# 敏感路径禁用追踪
location /auth/login {
    otel_span_attr auth.endpoint login;
    # 不记录请求体
    proxy_pass http://auth_service;
}
```

### 3. Span 命名规范

使用清晰、一致的命名：

```nginx
# 推荐：包含 HTTP 方法和路径
otel_span_name "$request_method $uri";

# 或按服务分类
otel_span_name "nginx:$request_method $uri";

# 避免：过于笼统或过于详细
# otel_span_name "request";                    # 太笼统
# otel_span_name "GET /api/v1/users/12345";    # 包含动态 ID
```

### 4. 上下文传播

**服务边界处理：**

```nginx
# 入口服务：注入新上下文
server {
    location /api/ {
        otel_trace_context inject;
        # 向后传递
        proxy_set_header traceparent $http_traceparent;
        proxy_pass http://backend;
    }
}

# 中间服务：传播上下文
server {
    location / {
        otel_trace_context propagate;
        # 既提取上游上下文，又注入到下游
        proxy_set_header traceparent $http_traceparent;
        proxy_pass http://next_service;
    }
}

# 出口服务：提取上下文
server {
    location / {
        otel_trace_context extract;
        # 只使用上游传入的上下文，不向后传播
        proxy_pass http://final_backend;
    }
}
```

### 5. 性能优化

**减少开销的配置：**

```nginx
http {
    # 增大批处理大小减少网络开销
    otel_exporter {
        endpoint otel-collector:4317;
        interval 10s;      # 增大导出间隔
        batch_size 1024;   # 增大批大小
        batch_count 8;     # 增加队列深度
    }

    # 选择性启用追踪
    map $request_uri $trace_enabled {
        ~*\.(css|js|png|jpg|gif|ico)$  "";    # 静态资源不追踪
        /health                          "";    # 健康检查不追踪
        /metrics                         "";    # 指标端点不追踪
        default                          "1";   # 其他请求追踪
    }

    otel_trace $trace_enabled;
}
```

### 6. 监控 Collector 健康

```nginx
# 监控 OTLP 导出器状态
server {
    location /nginx_status {
        stub_status on;
        allow 10.0.0.0/8;
        deny all;
    }

    location /otel_status {
        default_type application/json;
        return 200 '{
            "module": "ngx_otel_module",
            "service_name": "${otel_service_name}",
            "trace_enabled": "${otel_trace}"
        }';
    }
}
```

### 7. 故障排查

**常见问题及解决方案：**

| 问题 | 可能原因 | 解决方案 |
|------|----------|----------|
| 没有追踪数据 | Collector 不可达 | 检查网络连通性和端口 |
| 追踪链断裂 | 上下文传播配置错误 | 检查 `otel_trace_context` 设置 |
| Span 名称重复 | 未使用变量 | 使用 `$uri` 或 `$request_uri` |
| 采样率异常 | 变量配置错误 | 检查 `split_clients` 或 map |
| 属性缺失 | 变量未定义 | 使用 `map` 提供默认值 |

**调试配置：**

```nginx
# 临时开启详细日志
error_log /var/log/nginx/error.log debug;

# 添加调试端点
server {
    location /debug/otel {
        default_type application/json;
        return 200 '{
            "trace_id": "$otel_trace_id",
            "span_id": "$otel_span_id",
            "parent_id": "$otel_parent_id",
            "parent_sampled": "$otel_parent_sampled",
            "request_id": "$request_id",
            "http_traceparent": "$http_traceparent",
            "http_tracestate": "$http_tracestate"
        }';
    }
}
```

### 8. 多协议支持

如果后端服务使用不同协议：

```nginx
# W3C Trace Context (标准)
proxy_set_header traceparent $http_traceparent;
proxy_set_header tracestate $http_tracestate;

# B3 Propagation (Zipkin)
proxy_set_header X-B3-TraceId $otel_trace_id;
proxy_set_header X-B3-SpanId $otel_span_id;
proxy_set_header X-B3-ParentSpanId $otel_parent_id;
proxy_set_header X-B3-Sampled $otel_parent_sampled;

# Jaeger Propagation
proxy_set_header uber-trace-id "$otel_trace_id:$otel_span_id:$otel_parent_id:$otel_parent_sampled";
```

---

## 参考资源

- [NGINX OpenTelemetry Module 官方文档](https://nginx.org/en/docs/ngx_otel_module.html)
- [OpenTelemetry 官方文档](https://opentelemetry.io/docs/)
- [W3C Trace Context 规范](https://www.w3.org/TR/trace-context/)
- [Jaeger 文档](https://www.jaegertracing.io/docs/)
- [Zipkin 文档](https://zipkin.io/)

---

*文档版本: 1.0 | 最后更新: 2025-01*
