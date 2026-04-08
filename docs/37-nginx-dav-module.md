# Nginx WebDAV 模块完整指南

## 1. WebDAV 模块概述

### 1.1 模块简介

`ngx_http_dav_module` 是 Nginx 的文件管理自动化模块，通过 WebDAV (Web Distributed Authoring and Versioning) 协议支持远程文件操作。

### 1.2 主要用途

- 文件上传与管理
- 远程文件编辑
- 目录结构创建
- 文件复制与移动
- 简单的文件服务器

### 1.3 编译要求

该模块**不会默认构建**，需要在编译时显式启用：

```bash
./configure --with-http_dav_module
```

### 1.4 支持的 HTTP 方法

| 方法 | 说明 |
|------|------|
| `PUT` | 上传/创建文件 |
| `DELETE` | 删除文件或目录 |
| `MKCOL` | 创建目录 (Make Collection) |
| `COPY` | 复制文件或目录 |
| `MOVE` | 移动文件或目录 |

> **注意**：Nginx WebDAV 模块仅支持上述 5 种方法。需要其他 WebDAV 方法（如 PROPFIND、PROPPATCH、OPTIONS、LOCK、UNLOCK）的客户端将无法与此模块配合工作。

---

## 2. 指令详解

### 2.1 dav_methods

启用指定的 HTTP 方法。

**语法：**
```nginx
dav_methods off | PUT | DELETE | MKCOL | COPY | MOVE ...;
```

**默认值：** `off`

**上下文：** `http`, `server`, `location`

**说明：**
- `off` - 禁止此模块的所有方法
- 可以指定一个或多个方法
- 未列出的方法将被禁止

**示例：**
```nginx
# 仅启用 PUT 和 DELETE
dav_methods PUT DELETE;

# 启用全部支持的方法
dav_methods PUT DELETE MKCOL COPY MOVE;

# 禁用所有 WebDAV 方法
dav_methods off;
```

---

### 2.2 create_full_put_path

允许创建所有必需的中间目录。

**语法：**
```nginx
create_full_put_path on | off;
```

**默认值：** `off`

**上下文：** `http`, `server`, `location`

**说明：**
- WebDAV 规范通常要求目标目录必须已存在
- 设置为 `on` 时，PUT 请求可以自动创建路径中的所有中间目录
- 对于深度嵌套的文件上传非常有用

**示例：**
```nginx
# 允许 PUT /files/a/b/c/file.txt 自动创建 a/b/c/ 目录
create_full_put_path on;
```

---

### 2.3 dav_access

设置新创建的文件和目录的访问权限。

**语法：**
```nginx
dav_access users:permissions ...;
```

**默认值：** `user:rw`

**上下文：** `http`, `server`, `location`

**权限格式：**
```
user:permissions    # 文件所有者权限
group:permissions   # 组权限
all:permissions     # 所有用户权限
```

**权限值：**
- `r` - 读
- `w` - 写
- `x` - 执行（目录为进入）

**示例：**
```nginx
# 默认：用户可读写
dav_access user:rw;

# 用户读写，组读，其他用户只读
dav_access user:rw group:r all:r;

# 用户完全权限，组读执行，其他无权限
dav_access user:rwx group:rx all:;

# 多组权限
dav_access group:rw all:r;
```

---

### 2.4 min_delete_depth

设置 DELETE 操作允许删除的最小路径深度。

**语法：**
```nginx
min_delete_depth number;
```

**默认值：** `0`

**上下文：** `http`, `server`, `location`

**说明：**
- 用于防止意外删除重要目录
- 路径深度计算以 `/` 分隔的元素数量
- 请求 URI 的元素数量必须 >= 设定值才能执行 DELETE

**示例：**
```nginx
# 至少需要 4 层深度才能删除
min_delete_depth 4;

# 允许删除：DELETE /users/00/00/name  (4 层)
# 禁止删除：DELETE /users/00/00       (3 层)
```

---

## 3. 配置示例

### 3.1 基础文件共享服务器

```nginx
server {
    listen 80;
    server_name dav.example.com;

    location / {
        # 根目录
        root /data/www;

        # 临时文件目录（与根目录同一文件系统以获得最佳性能）
        client_body_temp_path /data/client_temp;

        # 启用 WebDAV 方法
        dav_methods PUT DELETE MKCOL COPY MOVE;

        # 允许创建中间目录
        create_full_put_path on;

        # 设置文件权限
        dav_access group:rw all:r;

        # 限制写操作的访问
        limit_except GET {
            allow 192.168.1.0/24;
            deny  all;
        }
    }
}
```

### 3.2 只读文件服务器（带选择性上传）

```nginx
server {
    listen 80;
    server_name files.example.com;

    # 公共只读区域
    location /public/ {
        root /data/files;
        dav_methods off;  # 禁止写操作
        autoindex on;     # 启用目录列表
    }

    # 受限上传区域
    location /uploads/ {
        root /data/files;
        dav_methods PUT DELETE;
        dav_access user:rw;
        
        # 仅允许特定 IP 上传
        limit_except GET {
            allow 10.0.0.0/8;
            deny  all;
        }
    }
}
```

### 3.3 支持深度目录的上传服务

```nginx
server {
    listen 80;
    server_name upload.example.com;

    location /storage/ {
        root /data/storage;
        
        dav_methods PUT DELETE MKCOL;
        create_full_put_path on;  # 自动创建嵌套目录
        dav_access user:rw group:r;
        
        # 防止删除顶层目录
        min_delete_depth 3;
        
        # 限制请求体大小
        client_max_body_size 100M;
        
        # 限制写操作
        limit_except GET PUT {
            allow 192.168.0.0/16;
            deny  all;
        }
    }
}
```

### 3.4 带认证的 WebDAV 服务

```nginx
server {
    listen 80;
    server_name secure-dav.example.com;

    location / {
        root /data/secure;
        
        dav_methods PUT DELETE MKCOL COPY MOVE;
        create_full_put_path on;
        dav_access user:rw;
        
        # HTTP 基本认证
        auth_basic "WebDAV Access";
        auth_basic_user_file /etc/nginx/.dav_passwd;
        
        # 限制客户端大小
        client_max_body_size 500M;
    }
}
```

生成密码文件：
```bash
htpasswd -c /etc/nginx/.dav_passwd username
```

### 3.5 带 SSL 的 WebDAV 服务器

```nginx
server {
    listen 80;
    server_name dav.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl;
    server_name dav.example.com;

    ssl_certificate     /etc/nginx/ssl/dav.crt;
    ssl_certificate_key /etc/nginx/ssl/dav.key;
    ssl_protocols       TLSv1.2 TLSv1.3;

    location / {
        root /data/www;
        
        dav_methods PUT DELETE MKCOL COPY MOVE;
        create_full_put_path on;
        dav_access user:rw group:r all:r;
        
        client_max_body_size 1G;
    }
}
```

---

## 4. 客户端使用指南

### 4.1 使用 curl

```bash
# 上传文件（PUT）
curl -T localfile.txt http://dav.example.com/remotefile.txt

# 创建目录（MKCOL）
curl -X MKCOL http://dav.example.com/newdir/

# 删除文件（DELETE）
curl -X DELETE http://dav.example.com/file.txt

# 复制文件（COPY）
curl -X COPY -H "Destination: http://dav.example.com/copy.txt" \
     http://dav.example.com/original.txt

# 移动文件（MOVE）
curl -X MOVE -H "Destination: http://dav.example.com/moved.txt" \
     http://dav.example.com/original.txt

# 带认证上传
curl -T file.txt -u username:password \
     http://dav.example.com/file.txt

# 查看文件内容（GET）
curl http://dav.example.com/file.txt

# 列出目录内容（需要客户端支持 PROPFIND）
curl -X PROPFIND http://dav.example.com/
```

### 4.2 使用 cadaver（命令行 WebDAV 客户端）

```bash
# 安装
# Debian/Ubuntu
apt-get install cadaver
# macOS
brew install cadaver

# 连接
cadaver http://dav.example.com/

# cadaver 常用命令
dav:> ls              # 列出目录
dav:> cd directory    # 进入目录
dav:> put file.txt    # 上传文件
dav:> get file.txt    # 下载文件
dav:> mkdir newdir    # 创建目录
dav:> rm file.txt     # 删除文件
dav:> mv old new      # 移动/重命名
dav:> quit            # 退出
```

### 4.3 使用 GNOME Nautilus（文件管理器）

1. 打开 Nautilus 文件管理器
2. 按 `Ctrl+L` 或选择"其他位置"
3. 输入地址：`dav://dav.example.com/`
4. 输入凭据（如需要）
5. 可像本地文件夹一样操作

### 4.4 挂载为本地文件系统（Linux）

使用 davfs2：

```bash
# 安装
apt-get install davfs2

# 挂载
mount -t davfs http://dav.example.com/ /mnt/webdav

# 或使用 /etc/fstab 自动挂载
http://dav.example.com/  /mnt/webdav  davfs  user,noauto  0  0
```

### 4.5 Windows 资源管理器

1. 打开"此电脑"
2. 右键 -> "添加网络位置"
3. 输入：`http://dav.example.com/`
4. 完成向导

或使用 `net use`：
```cmd
net use * http://dav.example.com/
```

### 4.6 macOS Finder

1. Finder -> "前往" -> "连接服务器"
2. 输入：`http://dav.example.com/`
3. 点击"连接"

---

## 5. 与 Lolly 项目的关系

### 5.1 Lolly 项目概述

Lolly 是一个用 Go 编写的高性能 HTTP 服务器/代理工具，提供：
- 灵活的路由和重写规则
- 中间件支持
- 反向代理功能
- 性能分析工具（pprof）

### 5.2 与 Nginx WebDAV 的对比

| 特性 | Nginx WebDAV | Lolly |
|------|--------------|-------|
| 协议支持 | WebDAV (有限) | 标准 HTTP |
| 配置方式 | 声明式配置 | Go 代码 |
| 扩展性 | 需重新编译 | 热插拔中间件 |
| 性能分析 | 第三方模块 | 内置 pprof |
| 学习曲线 | 低 | 中（需 Go 基础） |

### 5.3 建议与集成方案

#### 方案一：Nginx + Lolly 组合架构

```
客户端 -> Nginx (WebDAV/静态文件) -> Lolly (动态请求/代理)
```

```nginx
server {
    listen 80;
    server_name example.com;

    # WebDAV 文件存储
    location /dav/ {
        root /data/storage;
        dav_methods PUT DELETE MKCOL;
        create_full_put_path on;
        dav_access user:rw group:r;
    }

    # 动态请求代理到 Lolly
    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # 静态文件由 Nginx 直接服务
    location /static/ {
        root /data/www;
        expires 30d;
    }
}
```

#### 方案二：Lolly 实现类似 WebDAV 功能

如需在 Lolly 中实现类似 WebDAV 的文件上传功能，可参考以下思路：

```go
// 概念示例 - 实际实现请参考 Lolly 代码
router.PUT("/files/*filepath", func(c *gin.Context) {
    filepath := c.Param("filepath")
    fullPath := "/data/storage/" + filepath
    
    // 创建中间目录（类似 create_full_put_path on）
    os.MkdirAll(path.Dir(fullPath), 0755)
    
    // 保存上传的文件
    c.SaveUploadedFile(file, fullPath)
    
    // 设置权限（类似 dav_access）
    os.Chmod(fullPath, 0644)
    
    c.Status(http.StatusCreated)
})

router.DELETE("/files/*filepath", func(c *gin.Context) {
    filepath := c.Param("filepath")
    fullPath := "/data/storage/" + filepath
    
    // 检查深度（类似 min_delete_depth）
    depth := len(strings.Split(filepath, "/"))
    if depth < 4 {
        c.JSON(http.StatusForbidden, gin.H{"error": "path too shallow"})
        return
    }
    
    os.Remove(fullPath)
    c.Status(http.StatusNoContent)
})
```

#### 方案三：Lolly 作为 WebDAV 后端代理

```nginx
location /webdav/ {
    # 认证和限流在 Nginx 层处理
    auth_basic "WebDAV";
    auth_basic_user_file /etc/nginx/.passwd;
    
    # 限流
    limit_req zone=webdav burst=10;
    
    # 代理到 Lolly
    proxy_pass http://localhost:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header Destination $http_destination;
}
```

### 5.4 选择建议

| 场景 | 推荐方案 |
|------|----------|
| 简单文件共享 | Nginx WebDAV |
| 需要复杂业务逻辑 | Lolly |
| 高性能静态文件 | Nginx |
| 需要自定义协议扩展 | Lolly |
| 快速部署 | Nginx WebDAV |
| 深度集成应用 | Lolly |

---

## 6. 性能优化建议

### 6.1 文件系统建议

**强烈推荐**：临时目录和数据目录使用**同一文件系统**。

```nginx
location / {
    root                  /data/www;           # 数据目录
    client_body_temp_path /data/client_temp;   # 临时目录（同文件系统）
    
    dav_methods PUT DELETE;
}
```

**原因：**
- 同文件系统：文件上传使用 `rename()` 系统调用（原子操作，极快）
- 不同文件系统：文件上传需要 `copy + delete`（慢，占用双倍空间）

### 6.2 客户端大小限制

```nginx
# 限制上传文件大小，防止资源耗尽
client_max_body_size 100M;

# 调整缓冲区
client_body_buffer_size 128k;
```

### 6.3 超时设置

```nginx
# WebDAV 操作可能需要较长时间
client_body_timeout 300s;
send_timeout 300s;
```

---

## 7. 故障排查

### 7.1 常见问题

| 问题 | 可能原因 | 解决方案 |
|------|----------|----------|
| 405 Method Not Allowed | 方法未启用 | 检查 `dav_methods` 配置 |
| 403 Forbidden | 权限不足或 IP 限制 | 检查 `dav_access` 和 `limit_except` |
| 409 Conflict | 目录不存在 | 设置 `create_full_put_path on` |
| 423 Locked | 文件被锁定 | Nginx 不支持锁，换客户端 |
| 500 Internal Error | 路径过深或循环 | 检查 `min_delete_depth` |

### 7.2 启用调试日志

```nginx
error_log /var/log/nginx/error.log debug;
```

查看日志：
```bash
tail -f /var/log/nginx/error.log | grep dav
```

### 7.3 验证配置

```bash
# 测试配置文件语法
nginx -t

# 重新加载配置
nginx -s reload
```

---

## 8. 完整配置模板

```nginx
# /etc/nginx/conf.d/webdav.conf
server {
    listen 80;
    server_name dav.example.com;

    # 访问日志
    access_log /var/log/nginx/webdav_access.log;
    error_log  /var/log/nginx/webdav_error.log;

    # 客户端限制
    client_max_body_size    500M;
    client_body_buffer_size 128k;
    client_body_timeout     300s;
    send_timeout            300s;

    location / {
        # 根目录
        root /data/webdav;

        # 临时目录（与根目录同文件系统）
        client_body_temp_path /data/webdav_tmp;

        # 启用 WebDAV 方法
        dav_methods PUT DELETE MKCOL COPY MOVE;

        # 允许自动创建中间目录
        create_full_put_path on;

        # 文件权限设置
        dav_access user:rw group:r all:r;

        # 防止误删顶层目录
        min_delete_depth 2;

        # HTTP 基本认证
        auth_basic "WebDAV Repository";
        auth_basic_user_file /etc/nginx/.webdav_passwd;

        # 访问控制
        limit_except GET {
            # 允许内网
            allow 192.168.0.0/16;
            allow 10.0.0.0/8;
            allow 127.0.0.1;
            # 拒绝其他
            deny all;
        }

        # 目录列表（可选）
        autoindex on;
        autoindex_format html;
        autoindex_localtime on;
    }
}
```

---

## 9. 参考资源

- [Nginx 官方文档 - ngx_http_dav_module](https://nginx.org/en/docs/http/ngx_http_dav_module.html)
- [WebDAV RFC 4918](https://tools.ietf.org/html/rfc4918)
- [cadaver 客户端](http://www.webdav.org/cadaver/)
- [davfs2 - Linux WebDAV 挂载](http://savannah.nongnu.org/projects/davfs2/)
