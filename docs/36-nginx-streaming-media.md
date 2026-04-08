# NGINX 流媒体模块指南

## 1. 模块概述

NGINX 提供多个模块支持 HTTP 流媒体服务，涵盖直播、点播和伪流媒体场景。

### 1.1 模块对比

| 模块 | 协议/格式 | 用途 | 可用性 |
|------|-----------|------|--------|
| `ngx_http_hls_module` | HLS (HTTP Live Streaming) | MP4/MOV 文件的 HLS 直播流 | NGINX Plus 商业版 |
| `ngx_http_flv_module` | FLV (Flash Video) | FLV 文件伪流媒体 | 开源版 |
| `ngx_http_mp4_module` | MP4 (H.264/AAC) | MP4 文件伪流媒体 | 开源版 |
| `ngx_http_f4f_module` | F4F/F4M (Adobe HDS) | Adobe HTTP Dynamic Streaming | NGINX Plus 商业版 |

### 1.2 伪流媒体 vs 直播流

**伪流媒体 (Pseudo-Streaming)**：
- 客户端通过 `start` 参数请求特定时间点
- 服务器从该时间点开始发送视频数据
- 适用于点播场景，支持随机 seek

**直播流 (Live Streaming)**：
- 实时生成媒体片段 (TS/F4F)
- 动态更新播放列表 (M3U8/F4M)
- 支持 HLS、HDS 等自适应码率协议

---

## 2. HLS 模块 (ngx_http_hls_module)

为 MP4 和 MOV 媒体文件提供 HTTP Live Streaming (HLS) 服务器端支持。

### 2.1 编译配置

```bash
# 商业订阅版本已包含此模块
# 无需额外编译参数
```

### 2.2 指令详解

#### hls

**语法**：`hls;`

**默认值**：无

**上下文**：`location`

**说明**：在 surrounding location 中开启 HLS 流媒体服务。

```nginx
location / {
    hls;
}
```

#### hls_fragment

**语法**：`hls_fragment time;`

**默认值**：`hls_fragment 5s;`

**上下文**：`http`, `server`, `location`

**说明**：为未带 `len` 参数请求的播放列表 URI 定义默认片段长度。

```nginx
hls_fragment 10s;    # 每个 TS 片段 10 秒
```

#### hls_buffers

**语法**：`hls_buffers number size;`

**默认值**：`hls_buffers 8 2m;`

**上下文**：`http`, `server`, `location`

**说明**：设置用于读写数据帧的最大缓冲区数量和大小。

```nginx
hls_buffers 10 10m;  # 10 个缓冲区，每个 10MB
```

#### hls_forward_args

**语法**：`hls_forward_args on | off;`

**默认值**：`hls_forward_args off;`

**上下文**：`http`, `server`, `location`

**说明**：将播放列表请求中的参数添加到片段 (fragment) 的 URI 中。

**用途**：
- 客户端授权
- 配合 `ngx_http_secure_link_module` 保护 HLS 流

```nginx
hls_forward_args on;
```

#### hls_mp4_buffer_size

**语法**：`hls_mp4_buffer_size size;`

**默认值**：`hls_mp4_buffer_size 512k;`

**上下文**：`http`, `server`, `location`

**说明**：设置用于处理 MP4 和 MOV 文件的初始缓冲区大小。

```nginx
hls_mp4_buffer_size 1m;
```

#### hls_mp4_max_buffer_size

**语法**：`hls_mp4_max_buffer_size size;`

**默认值**：`hls_mp4_max_buffer_size 10m;`

**上下文**：`http`, `server`, `location`

**说明**：在元数据处理期间，缓冲区最大不能超过此值，否则返回 500 错误。

**错误日志**：`"mp4 moov atom is too large"`

```nginx
hls_mp4_max_buffer_size 5m;
```

### 2.3 请求参数

HLS 播放列表支持以下 URI 参数：

| 参数 | 说明 | 示例 |
|------|------|------|
| `start` | 起始时间（秒） | `?start=1.000` |
| `end` | 结束时间（秒） | `?end=2.200` |
| `offset` | 偏移时间（秒） | `?offset=1.000` |
| `len` | 片段长度（秒） | `?len=10` |

### 2.4 配置示例

#### 基本 HLS 配置

```nginx
server {
    listen 80;
    server_name hls.example.com;

    location / {
        hls;
        hls_fragment            5s;
        hls_buffers             10 10m;
        hls_mp4_buffer_size     1m;
        hls_mp4_max_buffer_size 5m;
        root /var/video/;
    }
}
```

#### 安全链接配置

配合 `hls_forward_args on` 使用 secure_link：

```nginx
http {
    # 提取基础 URI（去掉 .m3u8 和 .ts 后缀）
    map $uri $hls_uri {
        ~^(?<base_uri>.*)\.m3u8$ $base_uri;
        ~^(?<base_uri>.*)\.ts$   $base_uri;
        default                 $uri;
    }

    server {
        listen 80;
        server_name secure-hls.example.com;

        location /hls/ {
            hls;
            hls_forward_args on;
            alias /var/videos/;

            # 安全链接验证
            secure_link $arg_md5,$arg_expires;
            secure_link_md5 "$secure_link_expires$hls_uri$remote_addr secret";

            if ($secure_link = "") { return 403; }
            if ($secure_link = "0") { return 410; }
        }
    }
}
```

### 2.5 请求 URI 示例

对于文件 `/var/video/test.mp4`：

| 类型 | URI 示例 |
|------|----------|
| 播放列表 | `http://hls.example.com/test.mp4.m3u8?offset=1.000&start=1.000&end=2.200` |
| 片段 | `http://hls.example.com/test.mp4.ts?start=1.000&end=2.200` |

---

## 3. FLV 模块 (ngx_http_flv_module)

为 Flash Video (FLV) 文件提供伪流媒体服务器端支持。

### 3.1 编译配置

```bash
# 此模块默认不构建，需要显式启用
./configure --with-http_flv_module ...
```

### 3.2 指令详解

#### flv

**语法**：`flv;`

**默认值**：无

**上下文**：`location`

**说明**：在 surrounding location 中开启 FLV 模块处理。

**行为**：
- 特殊处理包含 `start` 参数的请求
- 从请求的字节偏移量发送文件内容
- 自动前置 FLV 头

```nginx
location ~ \.flv$ {
    flv;
}
```

### 3.3 请求参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `start` | 起始字节偏移量 | `?start=1000` |

### 3.4 配置示例

```nginx
server {
    listen 80;
    server_name video.example.com;

    location /videos/ {
        root /var/www/;
    }

    location ~ \.flv$ {
        flv;
        root /var/www/videos/;
    }
}
```

---

## 4. MP4 模块 (ngx_http_mp4_module)

为 MP4 文件提供服务器端伪流媒体支持，允许通过 `start` 和 `end` 参数进行随机 seek。

### 4.1 编译配置

```bash
# 此模块默认不构建，需要显式启用
./configure --with-http_mp4_module ...
```

### 4.2 指令详解

#### mp4

**语法**：`mp4;`

**默认值**：无

**上下文**：`location`

**说明**：在 surrounding location 中开启 MP4 模块处理。

```nginx
location /video/ {
    mp4;
}
```

#### mp4_buffer_size

**语法**：`mp4_buffer_size size;`

**默认值**：`mp4_buffer_size 512K;`

**上下文**：`http`, `server`, `location`

**说明**：设置用于处理 MP4 文件的初始缓冲区大小。

```nginx
mp4_buffer_size 1m;
```

#### mp4_max_buffer_size

**语法**：`mp4_max_buffer_size size;`

**默认值**：`mp4_max_buffer_size 10M;`

**上下文**：`http`, `server`, `location`

**说明**：元数据处理期间缓冲区的最大大小。若 moov atom 过大，返回 500 错误。

```nginx
mp4_max_buffer_size 5m;
```

#### mp4_limit_rate

**语法**：`mp4_limit_rate on | off | factor;`

**默认值**：`mp4_limit_rate off;`

**上下文**：`http`, `server`, `location`

**说明**：基于文件平均比特率限制响应传输速率。

**参数说明**：
- `on`：限速因子为 1.1
- `factor`：自定义限速因子

**注意**：此指令仅适用于商业订阅版本。

```nginx
mp4_limit_rate on;       # 因子 1.1
mp4_limit_rate 1.5;      # 自定义因子 1.5
```

#### mp4_limit_rate_after

**语法**：`mp4_limit_rate_after time;`

**默认值**：`mp4_limit_rate_after 60s;`

**上下文**：`http`, `server`, `location`

**说明**：设置开始限速前的初始媒体数据播放时长。

**注意**：此指令仅适用于商业订阅版本。

```nginx
mp4_limit_rate_after 30s;
```

#### mp4_start_key_frame

**语法**：`mp4_start_key_frame on | off;`

**默认值**：`mp4_start_key_frame off;`

**上下文**：`http`, `server`, `location`

**说明**：强制输出视频从关键帧开始。

**行为**：
- 若 `start` 指定的位置非关键帧，使用 edit list 隐藏初始帧
- 需要 NGINX 1.21.4+
- 主流播放器（Chrome、Safari 等）支持 edit list

```nginx
mp4_start_key_frame on;
```

### 4.3 请求参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `start` | 起始时间（秒） | `?start=238.88` |
| `end` | 结束时间（秒） | `?end=555.55` |

**组合示例**：`?start=238.88&end=555.55`

### 4.4 配置示例

#### 基本 MP4 配置

```nginx
server {
    listen 80;
    server_name video.example.com;

    location /video/ {
        mp4;
        mp4_buffer_size       1m;
        mp4_max_buffer_size   5m;
        root /var/videos/;
    }
}
```

#### 高级配置（商业版功能）

```nginx
server {
    listen 80;
    server_name video.example.com;

    location /video/ {
        mp4;
        mp4_buffer_size       1m;
        mp4_max_buffer_size   5m;
        mp4_limit_rate        on;
        mp4_limit_rate_after  30s;
        mp4_start_key_frame   on;
        root /var/videos/;
    }
}
```

### 4.5 性能优化建议

**文件结构优化**：
- 若 moov atom（元数据）位于文件末尾，NGINX 需要读取整个文件
- 建议使用工具（如 `qt-faststart`）将 moov atom 移到文件开头：

```bash
# 使用 qt-faststart 优化 MP4 文件
qt-faststart input.mp4 output.mp4
```

---

## 5. F4F 模块 (ngx_http_f4f_module)

提供 Adobe HTTP Dynamic Streaming (HDS) 的服务器端支持。

### 5.1 模块说明

**功能**：
- 处理 `/videoSeg1-Frag1` 形式的请求
- 使用 `.f4x` 索引文件从 `.f4f` 文件中提取片段

**可用性**：仅作为 NGINX Plus 商业订阅的一部分提供。

### 5.2 指令详解

#### f4f

**语法**：`f4f;`

**默认值**：无

**上下文**：`location`

**说明**：在 surrounding location 中开启 F4F 模块处理。

```nginx
location /video/ {
    f4f;
}
```

#### f4f_buffer_size

**语法**：`f4f_buffer_size size;`

**默认值**：`f4f_buffer_size 512k;`

**上下文**：`http`, `server`, `location`

**说明**：设置用于读取 `.f4x` 索引文件的缓冲区大小。

```nginx
f4f_buffer_size 1m;
```

### 5.3 配置示例

```nginx
server {
    listen 80;
    server_name hds.example.com;

    location /video/ {
        f4f;
        f4f_buffer_size 1m;
        root /var/f4f/;
    }
}
```

---

## 6. 完整配置示例

### 6.1 综合流媒体服务器

```nginx
# 负载均衡后端（用于回源）
upstream media_backend {
    server 192.168.1.10:8080 weight=5;
    server 192.168.1.11:8080 weight=5;
    server 192.168.1.12:8080 backup;
}

# 限速配置
limit_rate_after 1m;
limit_rate 1m;

server {
    listen 80;
    server_name media.example.com;

    # 日志格式
    log_format media '$remote_addr - $remote_user [$time_local] '
                     '"$request" $status $bytes_sent '
                     '"$http_referer" "$http_user_agent" '
                     'time=$request_time';

    access_log /var/log/nginx/media-access.log media;

    # HLS 流媒体（NGINX Plus）
    location /hls/ {
        hls;
        hls_fragment            5s;
        hls_buffers             10 10m;
        hls_mp4_buffer_size     1m;
        hls_mp4_max_buffer_size 5m;
        alias /var/videos/hls/;

        # 可选：安全链接
        # hls_forward_args on;
        # secure_link ...
    }

    # FLV 伪流媒体
    location /flv/ {
        location ~ \.flv$ {
            flv;
            alias /var/videos/flv/;
        }
    }

    # MP4 伪流媒体
    location /mp4/ {
        location ~ \.mp4$ {
            mp4;
            mp4_buffer_size       1m;
            mp4_max_buffer_size   5m;
            alias /var/videos/mp4/;

            # NGINX Plus 功能
            # mp4_limit_rate        on;
            # mp4_limit_rate_after  30s;
            # mp4_start_key_frame   on;
        }
    }

    # F4F 流媒体（NGINX Plus）
    location /hds/ {
        f4f;
        f4f_buffer_size 1m;
        alias /var/videos/hds/;
    }

    # 视频文件通用缓存配置
    location ~* \.(mp4|flv|f4f|ts|m3u8)$ {
        expires 1d;
        add_header Cache-Control "public, immutable";

        # 跨域支持
        add_header Access-Control-Allow-Origin "*";
        add_header Access-Control-Allow-Methods "GET, HEAD, OPTIONS";
    }

    # 播放列表不缓存（实时更新）
    location ~ \.m3u8$ {
        expires -1;
        add_header Cache-Control "no-cache, no-store, must-revalidate";
        add_header Pragma "no-cache";
    }

    # 状态监控
    location /nginx_status {
        stub_status on;
        allow 127.0.0.1;
        allow 10.0.0.0/8;
        deny all;
    }
}

# HTTPS 配置
server {
    listen 443 ssl http2;
    server_name media.example.com;

    ssl_certificate     /etc/ssl/certs/media.crt;
    ssl_certificate_key /etc/ssl/private/media.key;
    ssl_protocols       TLSv1.2 TLSv1.3;

    # 复用 HTTP 配置
    include /etc/nginx/conf.d/media-locations.conf;
}
```

### 6.2 带转码的流媒体配置

```nginx
# 使用 ngx_rtmp_module（第三方模块）做 RTMP 转 HLS
rtmp {
    server {
        listen 1935;

        application live {
            live on;

            # 转 HLS
            hls on;
            hls_path /var/videos/hls/;
            hls_fragment 5s;
            hls_playlist_length 60s;

            # 多码率
            hls_variant _low BANDWIDTH=500000;
            hls_variant _mid BANDWIDTH=1500000;
            hls_variant _high BANDWIDTH=5000000;
        }
    }
}

http {
    server {
        listen 80;
        server_name live.example.com;

        # 服务 HLS 流
        location /hls/ {
            types {
                application/vnd.apple.mpegurl m3u8;
                video/mp2t ts;
            }

            alias /var/videos/hls/;
            add_header Cache-Control "no-cache";
            add_header Access-Control-Allow-Origin "*";
        }
    }
}
```

---

## 7. 与 Lolly 项目的关系和建议

### 7.1 当前状态对比

| 特性 | NGINX 流媒体模块 | Lolly 当前状态 |
|------|------------------|----------------|
| HLS 支持 | 完整（商业版） | 暂未实现 |
| FLV 支持 | 完整（开源版） | 暂未实现 |
| MP4 点播 | 完整（需编译） | 暂未实现 |
| F4F/HDS | 完整（商业版） | 暂未实现 |
| 静态文件 | 完整 | 支持 |
| 文件缓存 | 完整 | 支持 |

### 7.2 实现建议

对于 Lolly 项目，可以考虑以下实现策略：

#### 1. 伪流媒体实现（MP4/FLV）

```go
// handler/streaming.go
package handler

import (
    "github.com/valyala/fasthttp"
)

// MP4Handler 处理 MP4 伪流媒体请求
func MP4Handler(ctx *fasthttp.RequestCtx) {
    start := ctx.QueryArgs().GetFloat64("start")
    end := ctx.QueryArgs().GetFloat64("end")

    // 解析 MP4 moov atom，计算偏移量
    // 从指定时间点开始传输
    // 处理 end 参数截断
}

// FLVHandler 处理 FLV 伪流媒体请求
func FLVHandler(ctx *fasthttp.RequestCtx) {
    start := ctx.QueryArgs().GetInt("start")

    // 发送 FLV 头
    // 从指定字节偏移开始传输
}
```

#### 2. HLS 服务实现

```go
// handler/hls.go
package handler

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// HLSPlaylistHandler 生成 M3U8 播放列表
func HLSPlaylistHandler(ctx *fasthttp.RequestCtx) {
    videoPath := getVideoPath(ctx)

    // 解析 MP4，计算片段
    segments := generateSegments(videoPath, fragmentDuration)

    // 生成 M3U8 内容
    playlist := generateM3U8(segments)

    ctx.SetContentType("application/vnd.apple.mpegurl")
    ctx.WriteString(playlist)
}

// HLSSegmentHandler 返回 TS 片段
func HLSSegmentHandler(ctx *fasthttp.RequestCtx) {
    // 从 MP4 提取指定时间范围的 TS 数据
    // 或使用预生成的 TS 文件
}
```

#### 3. 配置扩展示例

```yaml
# lolly.yaml 流媒体配置示例
server:
  # 静态文件服务（已支持）
  static:
    - path: "/"
      root: "/var/www/html"

  # 流媒体服务（建议新增）
  streaming:
    # HLS 配置
    hls:
      enabled: true
      path: "/hls/"
      root: "/var/videos/"
      fragment: "5s"
      buffers: 10
      buffer_size: "10m"
      mp4_buffer_size: "1m"
      mp4_max_buffer_size: "5m"

    # MP4 伪流媒体
    mp4:
      enabled: true
      path: "/mp4/"
      root: "/var/videos/"
      buffer_size: "1m"
      max_buffer_size: "5m"

    # FLV 伪流媒体
    flv:
      enabled: true
      path: "/flv/"
      root: "/var/videos/"

    # 跨域配置
    cors:
      enabled: true
      origins: ["*"]
      methods: ["GET", "HEAD", "OPTIONS"]
```

### 7.3 技术实现要点

#### MP4 文件处理

```go
// 关键：解析 moov atom，计算时间到字节的映射
type MP4Parser struct {
    Moov *MoovBox
}

type MoovBox struct {
    Tracks []*Track
}

type Track struct {
    Timescale uint32
    Samples   []*Sample
}

// SeekToTime 返回指定时间对应的文件偏移和样本索引
func (p *MP4Parser) SeekToTime(seconds float64) (offset int64, sampleIdx int) {
    // 遍历样本表，找到对应时间点的样本
    // 计算文件偏移
}
```

#### HLS 切片生成

```go
// SegmentInfo 表示一个 TS 片段
type SegmentInfo struct {
    Sequence  int
    Duration  float64
    StartTime float64
    EndTime   float64
}

// GenerateSegments 将 MP4 切分为片段信息
func GenerateSegments(videoPath string, fragmentDuration float64) ([]SegmentInfo, error) {
    // 解析 MP4 结构
    // 按关键帧边界分割片段
    // 返回片段信息列表
}
```

### 7.4 依赖库建议

| 功能 | 推荐库 |
|------|--------|
| MP4 解析 | `github.com/abema/go-mp4` 或 `github.com/deepch/mp4` |
| HLS 生成 | 自行实现或使用 `github.com/grafov/m3u8` |
| 视频转码 | `github.com/xfrr/goffmpeg` (FFmpeg 绑定) |
| FLV 解析 | `github.com/yapingcat/gomedia` |

### 7.5 性能优化建议

1. **文件缓存**：复用现有文件缓存系统缓存解析后的 MP4 元数据
2. **预生成切片**：对于点播内容，预先生成 TS 片段文件
3. **零拷贝传输**：大视频文件使用 sendfile
4. **Goroutine 池**：控制并发转码任务数量
5. **内存复用**：使用 sync.Pool 复用缓冲区

### 7.6 安全建议

1. **路径遍历防护**：验证请求路径，防止访问非视频目录
2. **限速控制**：对视频流进行带宽限制
3. **防盗链**：使用 secure_link 或 JWT token 验证
4. **CORS 配置**：按需配置跨域访问

---

## 8. 常见问题

### Q1: HLS 播放列表不更新？

**A**: 确保播放列表响应头禁用缓存：

```nginx
location ~ \.m3u8$ {
    expires -1;
    add_header Cache-Control "no-cache, no-store, must-revalidate";
}
```

### Q2: MP4 seek 不准确？

**A**: 启用 `mp4_start_key_frame on`（NGINX Plus 1.21.4+），或使用 edit list 隐藏非关键帧。

### Q3: FLV 无法 seek？

**A**: FLV 需要播放器支持，确保播放器发送 `start` 参数。

### Q4: 大文件处理缓慢？

**A**: 使用 `qt-faststart` 将 moov atom 移到文件开头：

```bash
qt-faststart input.mp4 output.mp4
```

### Q5: 跨域播放失败？

**A**: 添加 CORS 响应头：

```nginx
add_header Access-Control-Allow-Origin "*";
add_header Access-Control-Allow-Methods "GET, HEAD, OPTIONS";
```

---

## 9. 参考资源

- [NGINX HLS Module](https://nginx.org/en/docs/http/ngx_http_hls_module.html)
- [NGINX FLV Module](https://nginx.org/en/docs/http/ngx_http_flv_module.html)
- [NGINX MP4 Module](https://nginx.org/en/docs/http/ngx_http_mp4_module.html)
- [NGINX F4F Module](https://nginx.org/en/docs/http/ngx_http_f4f_module.html)
- [Apple HLS Specification](https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis)
- [Adobe HDS Specification](https://www.adobe.com/devnet/hds.html)
