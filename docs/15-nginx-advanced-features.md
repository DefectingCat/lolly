# Nginx 高级特性

## 1. 动态模块管理

### load_module 指令
- 语法：`load_module file;`
- 上下文：main（顶层）
- 示例：`load_module modules/ngx_http_geoip_module.so;`

### 静态编译 vs 动态加载
| 特性 | 静态模块 | 动态模块 |
|------|----------|----------|
| 编译方式 | 编译进二进制 | 独立 .so 文件 |
| 添加/移除 | 需重新编译 | 修改配置即可 |
| 内存占用 | 始终占用 | 按需加载 |

### 编译动态模块
```bash
./configure --with-compat --add-dynamic-module=/path/to/module
make modules
```

### 配置示例
```nginx
load_module modules/ngx_http_geoip_module.so;
load_module modules/ngx_stream_module.so;
```

## 2. 线程池与异步 I/O

### thread_pool 指令
```nginx
thread_pool default threads=32 max_queue=65536;
thread_pool fast threads=64 max_queue=131072;
```

### aio 指令选项
| 值 | 说明 |
|----|------|
| off | 禁用 AIO |
| on | 内核 AIO |
| threads | 默认线程池 |
| threads=pool | 指定线程池 |

### 大文件优化配置
```nginx
location /videos/ {
    sendfile on;
    aio threads=fast;
    directio 4m;
    output_buffers 2 1m;
}
```

### 性能影响
- 小文件：无显著提升
- 大文件：2-10x 提升
- 高并发：5-20x 提升

## 3. 开放文件缓存 (open_file_cache)

### 指令详解
| 指令 | 说明 | 默认值 |
|------|------|--------|
| open_file_cache | 缓存配置 | - |
| open_file_cache_valid | 有效期 | 60s |
| open_file_cache_min_uses | 最小访问次数 | 1 |
| open_file_cache_errors | 缓存错误 | off |

### 缓存内容
- 文件描述符
- 文件大小
- 修改时间
- 文件存在性

### 配置示例
```nginx
open_file_cache max=10000 inactive=60s;
open_file_cache_valid 30s;
open_file_cache_min_uses 2;
open_file_cache_errors on;
```

### 配置建议矩阵
| 场景 | max | inactive | valid |
|------|-----|----------|-------|
| 高流量 CDN | 50000+ | 60s | 30s |
| 企业网站 | 10000 | 60s | 30s |
| 下载服务 | 5000 | 300s | 60s |

## 4. 最佳实践

- 核心模块静态编译，可选模块动态加载
- 线程数设置为 CPU 核心数 2-4 倍
- open_file_cache max 值为总文件数 1.5-2 倍
