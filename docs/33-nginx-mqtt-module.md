# NGINX MQTT 模块指南

## 1. MQTT Preread 模块概述

### 1.1 模块介绍

`ngx_stream_mqtt_preread_module` 模块用于在 preread 阶段从 MQTT CONNECT 消息中提取客户端信息，而无需终止 MQTT 连接。

### 1.2 版本要求

- 自 **1.23.4** 版本起可用
- 属于 **NGINX Plus 商业订阅**功能
- 支持 MQTT 协议版本 **3.1.1** 和 **5.0**

### 1.3 核心用途

MQTT Preread 模块主要用于以下场景：

1. **基于 Client ID 的路由** - 根据客户端 ID 将连接路由到不同的 MQTT Broker
2. **基于用户名的路由** - 根据用户名进行负载均衡或访问控制
3. **透明代理** - 在不解密 MQTT 连接的情况下获取连接元数据
4. **连接分析** - 记录和分析 MQTT 客户端连接信息

### 1.4 工作原理

```
客户端 MQTT 连接
       |
       v
[NGINX Stream 模块]
       |
       v
[Preread 阶段] <--- mqtt_preread 在此阶段读取 CONNECT 消息
       |
       v
[变量提取] <--- $mqtt_preread_clientid, $mqtt_preread_username
       |
       v
[路由决策] <--- 基于变量进行 upstream 选择
       |
       v
[代理到后端 MQTT Broker]
```

在 preread 阶段，NGINX 读取 MQTT CONNECT 消息的前几个字节（不超过 16KB），解析其中的 Client ID 和 Username，然后基于这些信息进行路由决策。

---

## 2. MQTT Filter 模块概述

### 2.1 模块介绍

`ngx_stream_mqtt_filter_module` 模块提供完整的 MQTT 协议支持，允许修改 CONNECT 消息中的字段。

### 2.2 版本要求

- 自 **1.23.4** 版本起可用
- 属于 **NGINX Plus 商业订阅**功能
- 支持 MQTT 协议版本 **3.1.1** 和 **5.0**

### 2.3 核心用途

MQTT Filter 模块主要用于以下场景：

1. **修改 Client ID** - 为客户端分配统一的 Client ID 格式
2. **修改用户名/密码** - 进行身份认证信息的转换或注入
3. **代理认证** - 在代理层添加统一的认证信息
4. **会话管理** - 控制客户端会话标识

### 2.4 与 Preread 模块的区别

| 特性 | Preread 模块 | Filter 模块 |
|------|--------------|-------------|
| 主要功能 | 读取 CONNECT 信息 | 修改 CONNECT 字段 |
| 处理阶段 | Preread 阶段 | 代理阶段 |
| 是否修改数据 | 否（只读） | 是（读写） |
| 使用场景 | 路由、分析 | 认证、转换 |
| 性能开销 | 极低 | 较低 |

---

## 3. 指令详解

### 3.1 MQTT Preread 模块指令

#### mqtt_preread

启用或禁用从 MQTT CONNECT 消息中提取信息。

**语法**：
```nginx
mqtt_preread on | off;
```

**默认值**：`off`

**上下文**：`stream`, `server`

**说明**：
- 在 preread 阶段启用 MQTT CONNECT 消息解析
- 启用后，可以通过变量 `$mqtt_preread_clientid` 和 `$mqtt_preread_username` 访问提取的信息
- 仅解析 CONNECT 消息，不修改数据流

**配置示例**：
```nginx
stream {
    server {
        listen 1883;
        
        # 启用 MQTT preread
        mqtt_preread on;
        
        # 基于 clientid 路由
        proxy_pass $mqtt_backend;
    }
}
```

---

### 3.2 MQTT Filter 模块指令

#### mqtt

为给定虚拟服务器启用 MQTT 协议支持。

**语法**：
```nginx
mqtt on | off;
```

**默认值**：`off`

**上下文**：`stream`, `server`

**说明**：
- 启用后可以使用其他 MQTT 相关指令
- 必须在 `mqtt_set_connect` 之前启用

**配置示例**：
```nginx
stream {
    server {
        listen 1883;
        
        # 启用 MQTT 支持
        mqtt on;
        
        proxy_pass backend;
    }
}
```

---

#### mqtt_buffers

设置单个连接处理 MQTT 消息的缓冲区数量和大小。

**语法**：
```nginx
mqtt_buffers number size;
```

**默认值**：`100 1k;`

**上下文**：`stream`, `server`

**版本要求**：1.25.1+

**说明**：
- 控制用于处理 MQTT 消息的缓冲区配置
- 较大的缓冲区可以处理更大的 MQTT 消息
- 根据预期的消息大小和并发连接数调整

**配置示例**：
```nginx
stream {
    server {
        listen 1883;
        mqtt on;
        
        # 设置缓冲区：50 个缓冲区，每个 4KB
        mqtt_buffers 50 4k;
        
        proxy_pass backend;
    }
}
```

---

#### mqtt_rewrite_buffer_size

设置用于写入修改后消息的缓冲区大小。

**语法**：
```nginx
mqtt_rewrite_buffer_size size;
```

**默认值**：`4k` 或 `8k`（取决于平台内存页大小）

**上下文**：`server`

**版本要求**：1.25.1+

**废弃状态**：已废弃，建议使用 `mqtt_buffers`

**说明**：
- 该指令在 1.25.1 版本中已被废弃
- 请使用 `mqtt_buffers` 替代

---

#### mqtt_set_connect

设置 CONNECT 消息的字段为给定值。

**语法**：
```nginx
mqtt_set_connect field value;
```

**默认值**：无

**上下文**：`server`

**说明**：
- 支持修改的字段：`clientid`, `username`, `password`
- 值可以包含文本、变量及其组合
- 可以在同一级别指定多个指令

**可用字段**：

| 字段 | 说明 |
|------|------|
| `clientid` | MQTT 客户端标识符 |
| `username` | 连接用户名 |
| `password` | 连接密码 |

**配置示例**：
```nginx
stream {
    server {
        listen 18883;
        proxy_pass backend;
        proxy_buffer_size 16k;
        
        mqtt on;
        
        # 设置 Client ID
        mqtt_set_connect clientid "$client";
        
        # 设置用户名
        mqtt_set_connect username "$name";
        
        # 设置密码（从变量获取）
        mqtt_set_connect password "$mqtt_password";
    }
}
```

---

## 4. 嵌入变量

### 4.1 MQTT Preread 变量

| 变量 | 说明 |
|------|------|
| `$mqtt_preread_clientid` | CONNECT 消息中的 Client ID 值 |
| `$mqtt_preread_username` | CONNECT 消息中的 Username 值 |

**变量使用示例**：
```nginx
stream {
    # 使用 map 基于 clientid 路由
    map $mqtt_preread_clientid $backend_pool {
        ~^device-1    backend_1;
        ~^device-2    backend_2;
        ~^sensor-.*   sensors_backend;
        default       default_backend;
    }
    
    server {
        listen 1883;
        mqtt_preread on;
        
        proxy_pass $backend_pool;
    }
}
```

---

## 5. 配置示例

### 5.1 基础 MQTT 代理

简单的 MQTT 代理配置，将所有连接转发到后端 Broker：

```nginx
stream {
    upstream mqtt_backend {
        server 192.168.1.10:1883;
        server 192.168.1.11:1883 backup;
    }
    
    server {
        listen 1883;
        proxy_pass mqtt_backend;
        proxy_timeout 30s;
        proxy_connect_timeout 5s;
    }
}
```

### 5.2 基于 Client ID 的路由

根据 MQTT Client ID 将连接路由到不同的后端：

```nginx
stream {
    # 设备组 1：工业传感器
    upstream sensors_backend {
        server 10.0.1.10:1883 weight=5;
        server 10.0.1.11:1883;
    }
    
    # 设备组 2：智能家居设备
    upstream home_backend {
        server 10.0.2.10:1883;
        server 10.0.2.11:1883;
    }
    
    # 设备组 3：车联网
    upstream vehicle_backend {
        server 10.0.3.10:1883;
        server 10.0.3.11:1883;
    }
    
    # 默认后端
    upstream default_backend {
        server 10.0.0.10:1883;
    }
    
    # 基于 Client ID 的路由映射
    map $mqtt_preread_clientid $target_backend {
        # 传感器设备（匹配 sensor- 开头的 Client ID）
        ~^sensor-    sensors_backend;
        
        # 智能家居设备（匹配 home- 开头的 Client ID）
        ~^home-      home_backend;
        
        # 车载设备（匹配 vehicle- 或 car- 开头的 Client ID）
        ~^vehicle-   vehicle_backend;
        ~^car-       vehicle_backend;
        
        # 默认后端
        default      default_backend;
    }
    
    server {
        listen 1883;
        
        # 启用 MQTT preread
        mqtt_preread on;
        
        # 基于 Client ID 路由
        proxy_pass $target_backend;
        
        # 连接超时配置
        proxy_timeout 300s;
        proxy_connect_timeout 10s;
        
        # 启用 TCP keepalive
        proxy_socket_keepalive on;
    }
}
```

### 5.3 基于用户名的负载均衡

根据用户名进行路由，适用于多租户场景：

```nginx
stream {
    # 租户 A 集群
    upstream tenant_a_backend {
        server 10.0.10.10:1883;
        server 10.0.10.11:1883;
    }
    
    # 租户 B 集群
    upstream tenant_b_backend {
        server 10.0.20.10:1883;
        server 10.0.20.11:1883;
    }
    
    # 管理员集群
    upstream admin_backend {
        server 10.0.0.10:1883;
    }
    
    # 基于用户名路由
    map $mqtt_preread_username $tenant_backend {
        "tenant-a-user"    tenant_a_backend;
        "tenant-a-admin"   tenant_a_backend;
        "tenant-b-user"    tenant_b_backend;
        "tenant-b-admin"   tenant_b_backend;
        ~^admin-.*         admin_backend;
        default            tenant_a_backend;
    }
    
    server {
        listen 1883;
        mqtt_preread on;
        
        proxy_pass $tenant_backend;
        proxy_timeout 60s;
    }
}
```

### 5.4 修改 CONNECT 消息（Filter 模块）

在代理层修改 MQTT CONNECT 消息中的字段：

```nginx
stream {
    upstream mqtt_backend {
        server 192.168.1.10:1883;
    }
    
    server {
        listen 1883;
        proxy_pass mqtt_backend;
        proxy_buffer_size 16k;
        
        # 启用 MQTT Filter
        mqtt on;
        
        # 设置固定的 Client ID 前缀（追加原始 ID）
        mqtt_set_connect clientid "ng-$mqtt_preread_clientid";
        
        # 注入代理认证用户名
        mqtt_set_connect username "nginx-proxy";
        
        # 设置代理密码（从文件或环境变量获取）
        mqtt_set_connect password "$proxy_mqtt_password";
    }
}
```

### 5.5 综合配置示例

结合 Preread 和 Filter 模块的完整配置：

```nginx
stream {
    # 日志格式
    log_format mqtt_log '$remote_addr [$time_local] '
                        'clientid:$mqtt_preread_clientid '
                        'username:$mqtt_preread_username '
                        'upstream:$upstream_addr '
                        'status:$status';
    
    # 设备专用集群
    upstream device_cluster_a {
        zone devices 64k;
        server 10.0.1.10:1883 weight=5;
        server 10.0.1.11:1883;
        server 10.0.1.12:1883 backup;
    }
    
    upstream device_cluster_b {
        zone devices 64k;
        server 10.0.2.10:1883;
        server 10.0.2.11:1883;
    }
    
    # 普通设备集群
    upstream default_devices {
        zone devices 64k;
        least_conn;
        server 10.0.3.10:1883;
        server 10.0.3.11:1883;
        server 10.0.3.12:1883;
    }
    
    # Client ID 到后端映射
    map $mqtt_preread_clientid $backend_pool {
        ~^dev-a-    device_cluster_a;
        ~^device-a- device_cluster_a;
        ~^dev-b-    device_cluster_b;
        ~^device-b- device_cluster_b;
        default     default_devices;
    }
    
    # 服务器配置
    server {
        listen 1883;
        access_log /var/log/nginx/mqtt-access.log mqtt_log;
        
        # 启用 MQTT Preread 获取 Client ID 和用户名
        mqtt_preread on;
        
        # 启用 MQTT Filter（可选，需要修改 CONNECT 时启用）
        mqtt on;
        mqtt_buffers 50 4k;
        
        # 可选：修改 CONNECT 消息
        # mqtt_set_connect clientid "proxy-$mqtt_preread_clientid";
        
        # 基于 Client ID 路由
        proxy_pass $backend_pool;
        
        # 超时配置
        proxy_timeout 300s;
        proxy_connect_timeout 10s;
        
        # 启用 TCP keepalive
        proxy_socket_keepalive on;
        
        # 连接限制
        limit_conn mqtt_conn 100;
    }
    
    # TLS MQTT 端口
    server {
        listen 8883 ssl;
        
        ssl_certificate     /etc/nginx/ssl/mqtt.crt;
        ssl_certificate_key /etc/nginx/ssl/mqtt.key;
        ssl_protocols       TLSv1.2 TLSv1.3;
        
        mqtt_preread on;
        proxy_pass $backend_pool;
        proxy_timeout 300s;
    }
}

# 连接限制共享内存
limit_conn_zone $binary_remote_addr zone=mqtt_conn:10m;
```

### 5.6 与 SSL/TLS 结合

MQTT over TLS 的配置：

```nginx
stream {
    upstream mqtt_ssl_backend {
        server 192.168.1.10:8883;
        server 192.168.1.11:8883;
    }
    
    # 终止 TLS 并读取 MQTT 信息
    server {
        listen 8883 ssl;
        
        ssl_certificate     /etc/nginx/ssl/server.crt;
        ssl_certificate_key /etc/nginx/ssl/server.key;
        ssl_protocols       TLSv1.2 TLSv1.3;
        
        # 在解密后读取 MQTT 信息
        mqtt_preread on;
        mqtt on;
        
        proxy_pass mqtt_ssl_backend;
        
        # 上游也使用 SSL
        proxy_ssl on;
        proxy_ssl_protocols TLSv1.2 TLSv1.3;
    }
}
```

---

## 6. 与 Lolly 项目的关系

### 6.1 Lolly 项目简介

Lolly 是一个使用 Go 语言编写的高性能 HTTP 服务器与反向代理，基于 fasthttp 构建。它提供了 HTTP/3、WebSocket、TCP/UDP Stream 代理等功能。

### 6.2 功能对比

| 特性 | NGINX Plus (MQTT) | Lolly |
|------|-------------------|-------|
| MQTT Preread | 支持（商业版） | 未实现 |
| MQTT Filter | 支持（商业版） | 未实现 |
| TCP Stream 代理 | 支持 | 支持 |
| 基于内容路由 | 支持 | 有限支持 |
| SSL/TLS 终端 | 支持 | 支持 |
| 负载均衡 | 丰富算法 | 轮询/加权/最少连接/IP哈希 |

### 6.3 Lolly 中的 Stream 代理

Lolly 目前支持基础的 TCP/UDP Stream 代理：

```go
// Lolly Stream 代理示例配置（YAML）
stream:
  - listen: ":1883"
    protocol: "tcp"
    upstream:
      targets:
        - addr: "mqtt1:1883"
          weight: 3
        - addr: "mqtt2:1883"
          weight: 1
      load_balance: "round_robin"
```

当前 Lolly 的 Stream 实现位于 `internal/stream/stream.go`，提供：
- TCP/UDP 代理
- 负载均衡（轮询、加权轮询、最少连接、IP 哈希）
- 健康检查
- 会话管理（UDP）

### 6.4 在 Lolly 中实现 MQTT 支持的方案

#### 方案 1：独立 MQTT Preread 中间件

在 Lolly 的 Stream 模块中添加 MQTT Preread 功能：

```go
// internal/stream/mqtt_preread.go
package stream

import (
    "bufio"
    "encoding/binary"
    "io"
    "net"
)

// MQTTPrereadConfig MQTT Preread 配置
type MQTTPrereadConfig struct {
    Enabled  bool
    OnClientID func(clientID string) string // 路由回调
    OnUsername func(username string) string // 认证回调
}

// MQTTConnectInfo 解析后的 MQTT CONNECT 信息
type MQTTConnectInfo struct {
    ClientID string
    Username string
    Password []byte
    Protocol byte
}

// ParseMQTTConnect 从连接读取并解析 MQTT CONNECT 消息
func ParseMQTTConnect(conn net.Conn) (*MQTTConnectInfo, error) {
    // 1. 读取固定头（2-5 字节）
    // 2. 读取剩余长度
    // 3. 读取可变头（协议名、协议级别、连接标志）
    // 4. 读取 Payload（Client ID、Will Topic、Will Message、Username、Password）
    // 5. 返回解析结果
}
```

#### 方案 2：基于配置的路由

在 Lolly 配置中添加 MQTT 路由规则：

```yaml
stream:
  - listen: ":1883"
    protocol: "tcp"
    mqtt_preread: true
    routes:
      - match: "clientid =~ ^sensor-"
        upstream: sensors
      - match: "clientid =~ ^home-"
        upstream: home_devices
      - match: "username == admin"
        upstream: admin_cluster
    upstreams:
      sensors:
        targets:
          - addr: "mqtt-sensors-1:1883"
          - addr: "mqtt-sensors-2:1883"
      home_devices:
        targets:
          - addr: "mqtt-home-1:1883"
      admin_cluster:
        targets:
          - addr: "mqtt-admin-1:1883"
```

#### 方案 3：与现有 Stream 模块集成

扩展现有的 `internal/stream/stream.go`：

```go
// Target MQTT 扩展
type Target struct {
    addr   string
    weight int
    healthy atomic.Bool
    conns int64
    
    // MQTT 特定字段
    mqttMatcher func(*MQTTConnectInfo) bool // 匹配函数
}

// Upstream 添加 MQTT 选择支持
type Upstream struct {
    name     string
    targets  []*Target
    balancer Balancer
    
    // MQTT 路由表
    mqttRoutes map[string][]*Target // 标签 -> 目标列表
}

// SelectByMQTT 基于 MQTT CONNECT 信息选择目标
func (u *Upstream) SelectByMQTT(info *MQTTConnectInfo) *Target {
    // 1. 检查 MQTT 路由表
    // 2. 回退到默认负载均衡
}
```

### 6.5 实现建议

#### 短期建议（PoC 验证）

1. **实现基础 MQTT CONNECT 解析器**
   - 支持 MQTT 3.1.1 和 5.0
   - 提取 Client ID、Username、Password
   - 位置：`internal/stream/mqtt.go`

2. **添加基于 Client ID 的路由**
   - 简单的正则匹配
   - 配置文件支持
   - 与现有 upstream 集成

3. **性能测试**
   - 对比有无 preread 的性能差异
   - 内存占用分析

#### 中期建议（功能完善）

1. **完整 MQTT Filter 支持**
   - 支持修改 CONNECT 字段
   - 支持消息重写
   - 配置热重载

2. **监控与日志**
   - MQTT 特定指标（连接数、消息数）
   - 结构化日志输出

3. **安全增强**
   - 基于 Client ID 的访问控制
   - 速率限制

#### 长期建议（企业级功能）

1. **MQTT 5.0 完整支持**
   - 用户属性处理
   - 共享订阅支持
   - 消息过期处理

2. **高级路由**
   - 基于 Topic 的路由（需要解析 PUBLISH/SUBSCRIBE）
   - 动态后端发现

3. **与 HTTP 层的集成**
   - 统一的配置管理
   - 共享的健康检查

### 6.6 代码示例

以下是在 Lolly 中实现 MQTT Preread 的参考代码：

```go
// internal/stream/mqtt.go
package stream

import (
    "bufio"
    "encoding/binary"
    "fmt"
    "io"
    "net"
)

const (
    // MQTT 控制包类型
    mqttCONNECT = 1
    
    // MQTT 协议名
    mqttProtocol311 = "MQTT"    // v3.1.1
    mqttProtocol31  = "MQIsdp"  // v3.1
)

// MQTTPrereadHandler MQTT Preread 处理器
type MQTTPrereadHandler struct {
    config *MQTTPrereadConfig
}

// NewMQTTPrereadHandler 创建处理器
func NewMQTTPrereadHandler(config *MQTTPrereadConfig) *MQTTPrereadHandler {
    return &MQTTPrereadHandler{config: config}
}

// Handle 处理连接，解析 MQTT CONNECT 并返回信息
func (h *MQTTPrereadHandler) Handle(conn net.Conn) (*MQTTConnectInfo, net.Conn, error) {
    // 使用 bufio 预读数据
    reader := bufio.NewReaderSize(conn, 4096)
    
    // 读取固定头第一个字节
    firstByte, err := reader.Peek(1)
    if err != nil {
        return nil, nil, err
    }
    
    // 检查是否为 CONNECT 包
    packetType := (firstByte[0] >> 4) & 0x0F
    if packetType != mqttCONNECT {
        return nil, nil, fmt.Errorf("expected CONNECT packet, got %d", packetType)
    }
    
    // 读取剩余长度（可变长度编码）
    remainingLen, headerLen, err := decodeRemainingLength(reader)
    if err != nil {
        return nil, nil, err
    }
    
    // 读取完整的 CONNECT 包
    totalLen := 1 + headerLen + remainingLen
    packet, err := reader.Peek(totalLen)
    if err != nil {
        return nil, nil, err
    }
    
    // 解析 CONNECT 包
    info, err := parseConnectPacket(packet[1+headerLen:])
    if err != nil {
        return nil, nil, err
    }
    
    // 创建包装连接，将预读的数据返回给后续处理
    wrappedConn := &mqttConn{
        Conn:   conn,
        reader: reader,
        peeked: totalLen,
    }
    
    return info, wrappedConn, nil
}

// decodeRemainingLength 解码 MQTT 剩余长度字段
func decodeRemainingLength(r *bufio.Reader) (int, int, error) {
    var value int
    var multiplier int = 1
    var headerLen int
    
    for {
        b, err := r.ReadByte()
        if err != nil {
            return 0, 0, err
        }
        headerLen++
        
        value += int(b&0x7F) * multiplier
        multiplier *= 128
        
        if (b & 0x80) == 0 {
            break
        }
        
        if multiplier > 128*128*128 {
            return 0, 0, fmt.Errorf("malformed remaining length")
        }
    }
    
    return value, headerLen, nil
}

// parseConnectPacket 解析 CONNECT 包体
func parseConnectPacket(data []byte) (*MQTTConnectInfo, error) {
    info := &MQTTConnectInfo{}
    offset := 0
    
    // 读取协议名长度
    protoLen := binary.BigEndian.Uint16(data[offset:])
    offset += 2
    
    // 读取协议名
    protoName := string(data[offset : offset+int(protoLen)])
    offset += int(protoLen)
    
    // 判断协议版本
    if protoName == mqttProtocol311 {
        info.Protocol = 4 // MQTT 3.1.1
    } else if protoName == mqttProtocol31 {
        info.Protocol = 3 // MQTT 3.1
    }
    
    // 读取协议级别
    protocolLevel := data[offset]
    offset++
    _ = protocolLevel
    
    // 读取连接标志
    connectFlags := data[offset]
    offset++
    
    usernameFlag := (connectFlags >> 7) & 1
    passwordFlag := (connectFlags >> 6) & 1
    // willRetain := (connectFlags >> 5) & 1
    willQoS := (connectFlags >> 3) & 3
    willFlag := (connectFlags >> 2) & 1
    // cleanSession := (connectFlags >> 1) & 1
    
    // 读取保持连接时间（跳过）
    offset += 2
    
    // 读取 Client ID
    clientIDLen := binary.BigEndian.Uint16(data[offset:])
    offset += 2
    info.ClientID = string(data[offset : offset+int(clientIDLen)])
    offset += int(clientIDLen)
    
    // 读取 Will Topic 和 Will Message（如果有）
    if willFlag == 1 {
        // Will Topic
        willTopicLen := binary.BigEndian.Uint16(data[offset:])
        offset += 2
        offset += int(willTopicLen)
        
        // Will Message
        willMsgLen := binary.BigEndian.Uint16(data[offset:])
        offset += 2
        offset += int(willMsgLen)
    }
    
    // 读取 Username（如果有）
    if usernameFlag == 1 {
        usernameLen := binary.BigEndian.Uint16(data[offset:])
        offset += 2
        info.Username = string(data[offset : offset+int(usernameLen)])
        offset += int(usernameLen)
    }
    
    // 读取 Password（如果有）
    if passwordFlag == 1 {
        passwordLen := binary.BigEndian.Uint16(data[offset:])
        offset += 2
        info.Password = data[offset : offset+int(passwordLen)]
    }
    
    return info, nil
}

// mqttConn 包装连接，支持预读数据回退
type mqttConn struct {
    net.Conn
    reader *bufio.Reader
    peeked int
}

func (c *mqttConn) Read(p []byte) (n int, err error) {
    return c.reader.Read(p)
}
```

### 6.7 测试验证

添加 MQTT Preread 的单元测试：

```go
// internal/stream/mqtt_test.go
package stream

import (
    "bytes"
    "testing"
)

func TestParseConnectPacket(t *testing.T) {
    // 构造一个 MQTT 3.1.1 CONNECT 包
    // CONNECT + 剩余长度 + 协议名长度(4) + "MQTT" + 协议级别(4) + 连接标志 + 保持连接 + Client ID
    packet := []byte{
        // 可变头开始
        0x00, 0x04,       // 协议名长度 = 4
        'M', 'Q', 'T', 'T', // 协议名 "MQTT"
        0x04,             // 协议级别 4 (3.1.1)
        0xC2,             // 连接标志: 用户名(1) + 密码(1) + Clean Session(0)
        0x00, 0x3C,       // 保持连接 = 60 秒
        // Payload
        0x00, 0x0A,       // Client ID 长度 = 10
        't', 'e', 's', 't', '-', 'c', 'l', 'i', 'e', 'n', 't', // Client ID
        0x00, 0x05,       // 用户名长度 = 5
        'a', 'd', 'm', 'i', 'n', // 用户名
        0x00, 0x08,       // 密码长度 = 8
        'p', 'a', 's', 's', 'w', 'o', 'r', 'd', // 密码
    }
    
    info, err := parseConnectPacket(packet)
    if err != nil {
        t.Fatalf("parseConnectPacket failed: %v", err)
    }
    
    if info.ClientID != "test-client" {
        t.Errorf("ClientID = %q, want %q", info.ClientID, "test-client")
    }
    
    if info.Username != "admin" {
        t.Errorf("Username = %q, want %q", info.Username, "admin")
    }
    
    if info.Protocol != 4 {
        t.Errorf("Protocol = %d, want %d", info.Protocol, 4)
    }
}

func TestDecodeRemainingLength(t *testing.T) {
    tests := []struct {
        input    []byte
        expected int
    }{
        {[]byte{0x00}, 0},
        {[]byte{0x7F}, 127},
        {[]byte{0x80, 0x01}, 128},
        {[]byte{0xFF, 0x7F}, 16383},
        {[]byte{0x80, 0x80, 0x01}, 16384},
    }
    
    for _, tt := range tests {
        r := bufio.NewReader(bytes.NewReader(tt.input))
        value, _, err := decodeRemainingLength(r)
        if err != nil {
            t.Errorf("decodeRemainingLength(%v) error: %v", tt.input, err)
            continue
        }
        if value != tt.expected {
            t.Errorf("decodeRemainingLength(%v) = %d, want %d", tt.input, value, tt.expected)
        }
    }
}
```

---

## 7. 总结

### 7.1 NGINX MQTT 模块要点

1. **Preread 模块** (`ngx_stream_mqtt_preread_module`)
   - 只读解析 MQTT CONNECT 消息
   - 提取 Client ID 和 Username 用于路由
   - 适用于基于内容的负载均衡

2. **Filter 模块** (`ngx_stream_mqtt_filter_module`)
   - 支持修改 MQTT CONNECT 字段
   - 可用于代理认证和会话管理
   - 与 Preread 模块可以配合使用

3. **商业订阅限制**
   - 两个模块都需要 NGINX Plus 商业订阅
   - 自 1.23.4 版本起可用

### 7.2 适用场景

- **物联网平台** - 海量设备接入和路由
- **多租户 MQTT** - 基于用户名隔离租户
- **边缘网关** - 设备接入层代理
- **MQTT 迁移** - 平滑迁移到新 Broker

### 7.3 Lolly 实现建议优先级

1. **P0** - 基础 MQTT CONNECT 解析器
2. **P1** - 基于 Client ID 的路由
3. **P2** - MQTT Filter（修改 CONNECT）
4. **P3** - MQTT 5.0 高级特性

### 7.4 参考资料

- [NGINX MQTT Preread Module](https://nginx.org/en/docs/stream/ngx_stream_mqtt_preread_module.html)
- [NGINX MQTT Filter Module](https://nginx.org/en/docs/stream/ngx_stream_mqtt_filter_module.html)
- [MQTT 3.1.1 Specification](http://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html)
- [MQTT 5.0 Specification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html)
