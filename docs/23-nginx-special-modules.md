# NGINX WebDAV、图像处理与特殊功能模块

## 1. ngx_http_dav_module (WebDAV 模块)

WebDAV (Web-based Distributed Authoring and Versioning) 是一种基于 HTTP 的协议扩展，允许客户端远程管理服务器上的文件。NGINX 的 `ngx_http_dav_module` 模块实现了 WebDAV 的基本功能。

### 模块编译

该模块需要显式启用，默认不编译：

```bash
./configure --with-http_dav_module --add-module=/path/to/nginx-dav-ext-module
```

### dav_methods 指令

设置允许的 WebDAV HTTP 方法。

**语法**：`dav_methods [off | method ...];`

**可用方法**：

| 方法 | 说明 |
|------|------|
| `PUT` | 上传文件到服务器 |
| `DELETE` | 删除服务器上的文件 |
| `MKCOL` | 创建目录 |
| `COPY` | 复制文件或目录 |
| `MOVE` | 移动或重命名文件/目录 |

**配置示例**：

```nginx
location /upload/ {
    root /var/www/files;
    dav_methods PUT DELETE MKCOL COPY MOVE;
}
```

### dav_ext_methods 指令

启用额外的 WebDAV 方法（需要第三方模块 `nginx-dav-ext-module`）。

**语法**：`dav_ext_methods [method ...];`

**可用方法**：

| 方法 | 说明 |
|------|------|
| `PROPFIND` | 获取文件/目录属性 |
| `OPTIONS` | 获取服务器支持的方法 |
| `LOCK` | 锁定资源 |
| `UNLOCK` | 解锁资源 |

**配置示例**：

```nginx
location /webdav/ {
    root /var/www/webdav;
    dav_methods PUT DELETE MKCOL COPY MOVE;
    dav_ext_methods PROPFIND OPTIONS;
}
```

### create_full_put_path 指令

允许使用 PUT 方法创建不存在的中间目录。

**语法**：`create_full_put_path on | off;`

**默认值**：`off`

**配置示例**：

```nginx
location /upload/ {
    root /var/www/files;
    dav_methods PUT;
    create_full_put_path on;  # 允许自动创建父目录
}
```

### min_delete_depth 指令

设置允许 DELETE 方法删除目录的最小深度。

**语法**：`min_delete_depth number;`

**默认值**：`0`（允许删除任何目录）

**配置示例**：

```nginx
location /webdav/ {
    root /var/www/webdav;
    dav_methods DELETE;
    min_delete_depth 1;  # 禁止删除根目录下的直接子目录
}
```

### WebDAV 完整配置示例

```nginx
server {
    listen 80;
    server_name webdav.example.com;

    root /var/www/webdav;
    client_max_body_size 100m;  # 限制上传文件大小

    location / {
        dav_methods PUT DELETE MKCOL COPY MOVE;
        dav_ext_methods PROPFIND OPTIONS;

        create_full_put_path on;
        dav_access user:rw group:r all:r;  # 设置文件权限

        # 认证配置
        auth_basic "WebDAV Authentication";
        auth_basic_user_file /etc/nginx/webdav.htpasswd;

        # 允许的方法
        if ($request_method !~ ^(OPTIONS|GET|HEAD|POST|PUT|DELETE|MKCOL|COPY|MOVE|PROPFIND)$) {
            return 405;
        }
    }
}
```

### 客户端配置

**Windows 资源管理器**：
```
映射网络驱动器 -> \\webdav.example.com@80\DavWWWRoot
```

**macOS Finder**：
```
前往 -> 连接服务器 -> http://webdav.example.com
```

**Linux (davfs2)**：
```bash
sudo mount -t davfs http://webdav.example.com /mnt/webdav
```

---

## 2. ngx_http_image_filter_module (图像处理模块)

NGINX 的图像处理模块可以实时处理 JPEG、PNG、WebP 和 GIF 图像，支持调整大小、裁剪、旋转等操作。

### 依赖安装

该模块依赖 **libgd** 库：

```bash
# Ubuntu/Debian
sudo apt-get install libgd-dev

# CentOS/RHEL
sudo yum install gd-devel

# macOS
brew install gd
```

### 模块编译

```bash
./configure --with-http_image_filter_module
```

### image_filter 指令

定义图像处理操作。

**语法**：`image_filter (test | size | resize width height | crop width height | rotate 90|180|270);`

**操作类型**：

| 操作 | 说明 |
|------|------|
| `test` | 验证文件是否为有效图像，返回 204 或 415 |
| `size` | 返回图像的 JSON 元数据（宽度、高度、类型） |
| `resize width height` | 按比例调整大小，保持宽高比 |
| `crop width height` | 裁剪到指定尺寸，从中心开始 |
| `rotate 90\|180\|270` | 按指定角度旋转图像 |

**配置示例**：

```nginx
# 验证图像
location /img/test/ {
    image_filter test;
}

# 获取图像尺寸信息
location /api/img-size/ {
    image_filter size;
}

# 调整图像大小（最大 200x200，保持比例）
location /img/thumb/ {
    proxy_pass http://backend;
    image_filter resize 200 200;
    image_filter_jpeg_quality 75;
}

# 裁剪图像为固定尺寸
location /img/crop/ {
    proxy_pass http://backend;
    image_filter crop 100 100;
}
```

### image_filter_jpeg_quality 指令

设置 JPEG 图像处理的质量。

**语法**：`image_filter_jpeg_quality quality;`

**默认值**：`75`

**范围**：`1-100`

**配置示例**：

```nginx
location /img/jpeg/ {
    proxy_pass http://backend;
    image_filter resize 800 -;  # - 表示自动计算高度
    image_filter_jpeg_quality 85;
}
```

### image_filter_webp_quality 指令

设置 WebP 图像处理的质量。

**语法**：`image_filter_webp_quality quality;`

**默认值**：`80`

**范围**：`1-100`

**配置示例**：

```nginx
location /img/webp/ {
    proxy_pass http://backend;
    image_filter resize 800 -;
    image_filter_webp_quality 90;
}
```

### image_filter_transparency 指令

定义是否保留透明通道（PNG/WebP）。

**语法**：`image_filter_transparency on | off;`

**默认值**：`on`

**配置示例**：

```nginx
location /img/png/ {
    proxy_pass http://backend;
    image_filter resize 200 200;
    image_filter_transparency on;
}
```

### 动态图片处理示例

```nginx
server {
    listen 80;
    server_name images.example.com;

    # 原始图像
    location /original/ {
        alias /var/www/images/;
    }

    # 缩略图 200x200
    location /thumb/ {
        alias /var/www/images/;
        image_filter resize 200 200;
        image_filter_jpeg_quality 75;
        image_filter_buffer 10M;  # 限制源文件大小
    }

    # 中等尺寸 800x600
    location /medium/ {
        alias /var/www/images/;
        image_filter resize 800 600;
        image_filter_jpeg_quality 85;
    }

    # 带缓存的动态处理
    location /dynamic/ {
        proxy_pass http://localhost:8080;
        image_filter resize $arg_w $arg_h;
        image_filter_jpeg_quality $arg_q;

        proxy_cache img_cache;
        proxy_cache_valid 200 24h;
    }
}

# 缓存配置
http {
    proxy_cache_path /data/nginx/cache levels=1:2 keys_zone=img_cache:10m max_size=1g;
}
```

### 安全注意事项

```nginx
location /img/ {
    # 限制最大图像尺寸
    image_filter_buffer 10M;      # 限制源文件大小
    image_filter_size 10M;        # 限制输出文件大小

    # 限制 resize 参数
    image_filter resize 2000 2000; # 最大输出尺寸

    proxy_pass http://backend;
}
```

---

## 3. ngx_http_random_index_module (随机索引模块)

该模块从目录中随机选择一个文件作为索引文件，适用于随机展示内容。

### random_index 指令

启用或禁用随机索引。

**语法**：`random_index on | off;`

**默认值**：`off`

**上下文**：`location`

**配置示例**：

```nginx
server {
    listen 80;
    server_name gallery.example.com;

    location / {
        root /var/www/gallery;
        random_index on;  # 从目录随机选择文件作为首页
    }
}
```

### 应用场景

- **随机图片展示**：每次访问显示不同的图片
- **轮播广告**：随机展示广告内容
- **A/B 测试**：随机分配不同页面版本

---

## 4. ngx_http_empty_gif_module (空 GIF 模块)

该模块生成一个 1x1 像素的透明 GIF 图像，常用于追踪像素、占位符等场景。

### empty_gif 指令

返回 1x1 像素的透明 GIF。

**语法**：`empty_gif;`

**上下文**：`location`

**配置示例**：

```nginx
server {
    listen 80;

    # 追踪像素
    location = /track.gif {
        empty_gif;
        access_log /var/log/nginx/track.log tracking;
    }

    # 广告占位符
    location = /ad/placeholder.gif {
        empty_gif;
    }
}
```

### 追踪像素用途

```nginx
# 统计访问数据
log_format tracking '$remote_addr - $time_local "$request" '
                    '$http_referer "$http_user_agent" '
                    '$cookie_sessionid';

server {
    location = /pixel.gif {
        empty_gif;
        access_log /var/log/nginx/pixel.log tracking;

        # 设置追踪 cookie
        add_header Set-Cookie "visited=1; Path=/; Max-Age=86400";
    }
}
```

### 性能优势

- 不读取磁盘文件，内存中直接返回
- 极小的响应体（43 字节）
- 零 I/O 开销

---

## 5. ngx_http_flv_module / ngx_http_mp4_module (流媒体模块)

这两个模块支持 FLV 和 MP4 视频的伪流式传输（pseudo-streaming），允许用户从视频任意位置开始播放。

### 模块编译

```bash
./configure --with-http_flv_module --with-http_mp4_module
```

### FLV 伪流式传输

FLV 模块支持 `start` 参数，允许从指定字节位置开始播放。

**配置示例**：

```nginx
server {
    listen 80;
    server_name video.example.com;

    location ~ \.flv$ {
        root /var/www/videos;
        flv;  # 启用 FLV 伪流式传输

        # 支持 ?start= 参数
        # 例如：http://video.example.com/movie.flv?start=1024000
    }
}
```

### MP4 伪流式传输

MP4 模块支持更精确的基于时间的定位。

**配置示例**：

```nginx
server {
    listen 80;
    server_name video.example.com;

    location ~ \.mp4$ {
        root /var/www/videos;
        mp4;                  # 启用 MP4 伪流式传输
        mp4_buffer_size 1m;   # 设置 MP4 元数据缓冲区大小
        mp4_max_buffer_size 10m;  # 设置 MP4 元数据最大缓冲区
    }
}
```

### MP4 模块指令

| 指令 | 说明 | 默认值 |
|------|------|--------|
| `mp4` | 启用 MP4 伪流式传输 | - |
| `mp4_buffer_size` | 初始元数据缓冲区大小 | `512K` |
| `mp4_max_buffer_size` | 最大元数据缓冲区大小 | `10M` |

### 流媒体配置示例

```nginx
server {
    listen 80;
    server_name video.example.com;

    root /var/www/videos;

    # MP4 视频
    location ~ \.mp4$ {
        mp4;
        mp4_buffer_size 1m;
        mp4_max_buffer_size 5m;

        # 添加 CORS 头
        add_header Access-Control-Allow-Origin "*";
    }

    # FLV 视频
    location ~ \.flv$ {
        flv;
        add_header Access-Control-Allow-Origin "*";
    }

    # 限速（可选）
    limit_rate_after 5m;
    limit_rate 500k;
}
```

### 客户端播放器示例

**Video.js**：
```html
<video id="my-video" class="video-js" controls preload="auto">
    <source src="http://video.example.com/movie.mp4" type="video/mp4">
</video>
```

**Flowplayer**：
```html
<div class="flowplayer" data-swf="flowplayer.swf">
    <video>
        <source type="video/mp4" src="http://video.example.com/movie.mp4">
    </video>
</div>
```

---

## 6. ngx_http_hls_module (HLS 模块)

HLS (HTTP Live Streaming) 是 Apple 开发的基于 HTTP 的流媒体协议。NGINX 可以通过第三方模块支持 HLS。

### HLS 工作原理

1. 视频被分割成多个小片段 (.ts 文件)
2. 生成播放列表文件 (.m3u8)
3. 客户端根据播放列表顺序下载并播放片段

### nginx-rtmp-module 配置

使用 `nginx-rtmp-module` 模块实现 RTMP 推流和 HLS 播放：

```nginx
# 加载 RTMP 模块
load_module modules/ngx_rtmp_module.so;

rtmp {
    server {
        listen 1935;  # RTMP 端口

        application live {
            live on;

            # 启用 HLS
            hls on;
            hls_path /var/www/hls;           # HLS 片段存放路径
            hls_fragment 3s;                  # 每个片段时长
            hls_playlist_length 60s;          # 播放列表长度

            # 多码率支持
            hls_variant _low BANDWIDTH=500000;
            hls_variant _mid BANDWIDTH=1500000;
            hls_variant _high BANDWIDTH=5000000;
        }
    }
}

http {
    server {
        listen 80;
        server_name hls.example.com;

        location /hls/ {
            alias /var/www/hls/;

            # HLS 播放列表缓存
            location ~ \.m3u8$ {
                expires -1;  # 不缓存播放列表
                add_header Cache-Control "no-cache";
            }

            # 视频片段缓存
            location ~ \.ts$ {
                expires 7d;
                add_header Cache-Control "public";
            }

            # CORS 支持
            add_header Access-Control-Allow-Origin "*";
        }
    }
}
```

### HLS 播放

**Safari/iOS**（原生支持）：
```html
<video src="http://hls.example.com/hls/stream.m3u8" controls></video>
```

**hls.js**（其他浏览器）：
```html
<video id="video" controls></video>
<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
<script>
    var video = document.getElementById('video');
    var hls = new Hls();
    hls.loadSource('http://hls.example.com/hls/stream.m3u8');
    hls.attachMedia(video);
</script>
```

---

## 7. ngx_http_xslt_module (XSLT 模块)

该模块使用 XSLT 样式表转换 XML 响应，常用于动态生成 HTML 页面。

### 依赖安装

```bash
# Ubuntu/Debian
sudo apt-get install libxslt1-dev

# CentOS/RHEL
sudo yum install libxslt-devel
```

### 模块编译

```bash
./configure --with-http_xslt_module
```

### xslt_stylesheet 指令

指定 XSLT 样式表文件及参数。

**语法**：`xslt_stylesheet stylesheet [parameter=value ...];`

**配置示例**：

```nginx
location /api/ {
    proxy_pass http://backend;

    # 转换 XML 响应为 HTML
    xslt_stylesheet /var/www/xslt/api-to-html.xslt
                    title='API Documentation'
                    version='1.0';
}
```

### 完整配置示例

```nginx
server {
    listen 80;
    server_name xml.example.com;

    location /data/ {
        # 源 XML 数据
        alias /var/www/data/;

        # 根据请求参数选择不同样式表
        xslt_stylesheet /var/www/xslt/default.xslt
                        theme=$arg_theme
                        lang=$arg_lang;
    }
}
```

### XSLT 样式表示例

```xml
<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
    <xsl:output method="html" encoding="UTF-8" indent="yes"/>

    <xsl:template match="/">
        <html>
            <head><title><xsl:value-of select="page/title"/></title></head>
            <body>
                <h1><xsl:value-of select="page/title"/></h1>
                <xsl:apply-templates select="page/content"/>
            </body>
        </html>
    </xsl:template>
</xsl:stylesheet>
```

---

## 8. ngx_http_degradation_module (降级模块)

该模块在内存不足时对请求进行降级处理，防止服务完全不可用。

### degradation 指令

设置降级条件。

**语法**：`degradation [sbrk=[size]] [rate=[rate]];`

**配置示例**：

```nginx
http {
    # 当可用内存小于 128M 时启用降级
    degradation sbrk=128m rate=50%;

    server {
        listen 80;

        location / {
            # 正常处理
            proxy_pass http://backend;
        }

        location /degrade/ {
            # 降级响应：返回简单页面
            return 503 "Service temporarily unavailable due to high load";
        }
    }
}
```

### 内存不足时的降级策略

```nginx
http {
    # 监控内存使用情况
    degradation sbrk=256m;

    server {
        listen 80;

        # 正常内容
        location / {
            proxy_pass http://backend;
        }

        # 降级模式：关闭图片、简化页面
        location /images/ {
            # 内存不足时返回空 GIF
            degradation sbrk=100m;
            empty_gif;
        }

        # 降级模式：关闭复杂功能
        location /api/complex/ {
            degradation sbrk=50m;
            return 503;
        }
    }
}
```

### 注意事项

- 该模块主要用于嵌入式系统或内存受限环境
- 现代 Linux 系统通常使用虚拟内存管理，该模块效果有限
- 建议配合监控系统和自动扩展使用

---

## 9. 综合配置示例

```nginx
http {
    # WebDAV 文件共享
    server {
        listen 80;
        server_name files.example.com;

        root /var/www/webdav;
        client_max_body_size 500m;

        location / {
            dav_methods PUT DELETE MKCOL COPY MOVE;
            dav_ext_methods PROPFIND OPTIONS;
            create_full_put_path on;

            auth_basic "WebDAV";
            auth_basic_user_file /etc/nginx/webdav.htpasswd;
        }
    }

    # 图像处理服务
    server {
        listen 80;
        server_name img.example.com;

        location /thumb/ {
            alias /var/www/images/;
            image_filter resize 200 200;
            image_filter_jpeg_quality 75;
        }

        location /medium/ {
            alias /var/www/images/;
            image_filter resize 800 600;
            image_filter_jpeg_quality 85;
        }

        location /api/size/ {
            alias /var/www/images/;
            image_filter size;
        }
    }

    # 视频流媒体
    server {
        listen 80;
        server_name video.example.com;

        location ~ \.mp4$ {
            root /var/www/videos;
            mp4;
            mp4_buffer_size 1m;
            add_header Access-Control-Allow-Origin "*";
        }

        location ~ \.flv$ {
            root /var/www/videos;
            flv;
            add_header Access-Control-Allow-Origin "*";
        }

        location /hls/ {
            alias /var/www/hls/;
            add_header Access-Control-Allow-Origin "*";

            location ~ \.m3u8$ {
                expires -1;
                add_header Cache-Control "no-cache";
            }

            location ~ \.ts$ {
                expires 7d;
            }
        }
    }

    # 追踪像素服务
    server {
        listen 80;
        server_name track.example.com;

        location = /pixel.gif {
            empty_gif;
            access_log /var/log/nginx/pixel.log;
            add_header Set-Cookie "track_id=$request_id; Path=/";
        }
    }
}
```

---

## 10. 模块可用性参考

| 模块 | 默认编译 | 配置参数 | 依赖 |
|------|----------|----------|------|
| `ngx_http_dav_module` | 否 | `--with-http_dav_module` | 无 |
| `ngx_http_image_filter_module` | 否 | `--with-http_image_filter_module` | libgd |
| `ngx_http_random_index_module` | 是 | - | 无 |
| `ngx_http_empty_gif_module` | 是 | - | 无 |
| `ngx_http_flv_module` | 否 | `--with-http_flv_module` | 无 |
| `ngx_http_mp4_module` | 否 | `--with-http_mp4_module` | 无 |
| `ngx_http_xslt_module` | 否 | `--with-http_xslt_module` | libxslt |
| `ngx_http_degradation_module` | 否 | `--with-http_degradation_module` | 无 |

---

## 11. 安全建议

1. **WebDAV**：始终启用身份验证，限制访问 IP，设置合理的文件大小限制
2. **图像处理**：限制源文件大小和输出尺寸，防止 DoS 攻击
3. **流媒体**：添加 CORS 头支持跨域，配置防盗链
4. **HLS**：播放列表不缓存，视频片段长期缓存
5. **追踪像素**：注意隐私合规性，遵守 GDPR 等法规
