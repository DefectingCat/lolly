## 1. 请求镜像模块 (ngx_http_mirror_module)

### 概述
- 版本要求：1.13.4+
- 实时流量复制到测试环境
- 不影响原请求响应

### 核心指令
| 指令 | 说明 | 默认值 |
|------|------|--------|
| mirror | 镜像目标 location | off |
| mirror_request_body | 是否镜像请求体 | on |

### 基础配置示例
```nginx
server {
    location / {
        proxy_pass http://production_backend;
        mirror /mirror;
    }
    
    location = /mirror {
        internal;
        proxy_pass http://test_backend$request_uri;
    }
}
```

### 多目标镜像
```nginx
location / {
    proxy_pass http://production;
    mirror /mirror_test;
    mirror /mirror_staging;
}
```

### 条件镜像
```nginx
map $http_x_mirror $do_mirror {
    default "";
    "true" "/mirror";
}

server {
    location / {
        proxy_pass http://production;
        mirror $do_mirror;
    }
}
```

### 应用场景
- 灰度测试验证
- 压力测试（真实流量）
- 安全审计
- A/B 测试对比

### 最佳实践
- 使用 internal 标记镜像目标
- 设置短超时 `proxy_read_timeout 1s`
- 大文件上传关闭 `mirror_request_body off`

## 2. 大文件分片模块 (ngx_http_slice_module)

### 概述
- 版本要求：1.9.8+
- 将大文件分割成小块请求
- 支持 Range 请求高效缓存

### 核心指令
| 指令 | 说明 | 默认值 |
|------|------|--------|
| slice | 分片大小 | 0（禁用）|

### 内置变量
| 变量 | 说明 |
|------|------|
| $slice_range | 当前分片范围（bytes=0-1048575）|

### 配置示例
```nginx
location /videos/ {
    slice 1m;
    proxy_set_header Range $slice_range;
    proxy_cache my_cache;
    proxy_cache_key $uri$slice_range;
    proxy_cache_valid 200 206 1d;
    proxy_pass http://video_backend;
}
```

### 工作原理
```
客户端请求 Range: bytes=0-2M
    ↓
nginx 分割为：
  - Range: bytes=0-1M (分片1)
  - Range: bytes=1M-2M (分片2)
    ↓
每个分片独立缓存
```

### 应用场景
- 视频流媒体（边下边播）
- 大文件下载（断点续传）
- CDN 源站

### 最佳实践
- 视频文件：1-2MB 分片
- 大文件下载：5-10MB 分片
- 必须设置 `proxy_set_header Range $slice_range`
- 缓存键必须包含 `$slice_range`
- 缓存 206 状态码

## 3. 综合配置示例

```nginx
proxy_cache_path /var/cache/nginx levels=1:2 keys_zone=media:10m max_size=10g;

server {
    # 生产服务
    location /videos/ {
        slice 2m;
        proxy_set_header Range $slice_range;
        proxy_cache media;
        proxy_cache_key $uri$slice_range;
        proxy_cache_valid 200 206 7d;
        proxy_pass http://video_backend;
        
        # 同时镜像到测试环境
        mirror /mirror;
    }
    
    # 镜像目标
    location = /mirror {
        internal;
        proxy_pass http://test_video_backend$request_uri;
        proxy_read_timeout 1s;
    }
}
```
