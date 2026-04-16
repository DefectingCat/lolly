## 1. internal 指令

### 概述
- 标记 location 为内部专用
- 外部请求返回 404

### 内部请求触发方式
| 方式 | 说明 |
|------|------|
| X-Accel-Redirect | 上游响应头触发 |
| error_page | 错误页面重定向 |
| rewrite ... internal | 重写内部跳转 |
| SSI 包含 | 服务端包含 |

### 配置示例
```nginx
location /internal/ {
    internal;
    alias /data/protected/;
}
```

## 2. X-Accel-Redirect 响应头

### 工作原理
1. 客户端请求 → nginx → 应用服务器
2. 应用服务器验证权限
3. 返回 X-Accel-Redirect 头
4. nginx 内部重定向到受保护文件
5. 文件直接传输给客户端

### 文件下载授权示例
```nginx
# 下载入口
location /download/ {
    proxy_pass http://backend;
    proxy_hide_header X-Accel-Redirect;
}

# 受保护文件
location /internal/files/ {
    internal;
    alias /var/www/private/;
    limit_rate 500k;
}
```

### 应用服务器代码（Python 示例）
```python
@app.route('/download/<file_id>')
def download(file_id):
    if not user.has_permission(file_id):
        return "无权限", 403
    response.headers['X-Accel-Redirect'] = f'/internal/files/{file_id}'
    return response
```

## 3. X-Accel-* 系列响应头

| 响应头 | 功能 | 示例值 |
|--------|------|--------|
| X-Accel-Redirect | 内部重定向路径 | /internal/files/report.pdf |
| X-Accel-Expires | 缓存过期时间(秒) | 3600 |
| X-Accel-Limit-Rate | 限速(字节/秒) | 102400 |
| X-Accel-Buffering | 缓冲控制 | yes/no |
| X-Accel-Charset | 字符集 | utf-8 |

### VIP 用户不限速示例
```python
if user.is_vip():
    response.headers['X-Accel-Limit-Rate'] = '0'  # 不限速
else:
    response.headers['X-Accel-Limit-Rate'] = '512000'  # 500KB/s
```

## 4. 安全注意事项

### 必须标记 internal
```nginx
# 错误：外部可直接访问
location /protected/ {
    alias /data/secret/;
}

# 正确：仅内部访问
location /internal/protected/ {
    internal;
    alias /data/secret/;
}
```

### 路径验证
- 后端必须验证路径合法性
- 防止目录遍历攻击

### 完整配置示例
```nginx
server {
    location /download/ {
        proxy_pass http://backend;
        proxy_set_header X-Real-IP $remote_addr;
    }
    
    location /internal/premium/ {
        internal;
        alias /var/www/premium/;
        sendfile on;
        limit_rate_after 1m;
        limit_rate 1m;
    }
    
    location /internal/standard/ {
        internal;
        alias /var/www/standard/;
        limit_rate 500k;
    }
}
```
