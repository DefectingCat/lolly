# NGINX 安装与构建指南

## 1. 安装方式概述

NGINX 提供三种主要安装途径：

| 方式 | 适用场景 | 说明 |
|------|----------|------|
| **Linux 软件包** | 生产环境快速部署 | 使用 nginx.org 提供的预构建包 |
| **FreeBSD Ports/Packages** | FreeBSD 系统 | Ports 提供更多编译选项 |
| **源码编译** | 需要特殊功能 | 最大灵活性，可自定义模块 |

---

## 2. Linux 软件包安装

### RHEL/CentOS

```bash
# 安装 EPEL 仓库
yum install epel-release

# 安装 nginx
yum install nginx
```

### Ubuntu/Debian

```bash
# 更新包索引
apt update

# 安装 nginx
apt install nginx
```

### 官方仓库安装（推荐）

**RHEL/CentOS**：
```bash
# 添加 nginx 官方仓库
cat > /etc/yum.repos.d/nginx.repo << 'EOF'
[nginx]
name=nginx repo
baseurl=http://nginx.org/packages/rhel/$releasever/$basearch/
gpgcheck=0
enabled=1
EOF

yum install nginx
```

**Ubuntu/Debian**：
```bash
# 安装依赖
apt install curl gnupg2 ca-certificates lsb-release

# 添加 nginx 签名密钥
curl https://nginx.org/keys/nginx_signing.key | gpg --dearmor > /usr/share/keyrings/nginx-archive-keyring.gpg

# 添加仓库
echo "deb [signed-by=/usr/share/keyrings/nginx-archive-keyring.gpg] http://nginx.org/packages/ubuntu `lsb_release -cs` nginx" > /etc/apt/sources.list.d/nginx.list

apt update
apt install nginx
```

---

## 3. 从源码构建

### 构建流程

```bash
# 1. 下载源码
wget http://nginx.org/download/nginx-1.24.0.tar.gz
tar -xzf nginx-1.24.0.tar.gz
cd nginx-1.24.0

# 2. 配置编译选项
./configure --option=value ...

# 3. 编译
make

# 4. 安装
make install
```

---

## 4. 配置参数详解

### 路径配置参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--prefix=path` | 服务器文件目录（所有相对路径基准） | `/usr/local/nginx` |
| `--sbin-path=path` | nginx 可执行文件路径 | `prefix/sbin/nginx` |
| `--modules-path=path` | 动态模块安装目录 | `prefix/modules` |
| `--conf-path=path` | 配置文件路径 | `prefix/conf/nginx.conf` |
| `--error-log-path=path` | 错误日志文件路径 | `prefix/logs/error.log` |
| `--pid-path=path` | 主进程 PID 文件 | `prefix/logs/nginx.pid` |
| `--lock-path=path` | 锁文件路径 | `prefix/logs/nginx.lock` |
| `--http-log-path=path` | HTTP 请求日志文件 | `prefix/logs/access.log` |
| `--http-client-body-temp-path=path` | 客户端请求体临时文件目录 | `prefix/client_body_temp` |
| `--http-proxy-temp-path=path` | 代理临时文件目录 | `prefix/proxy_temp` |
| `--http-fastcgi-temp-path=path` | FastCGI 临时文件目录 | `prefix/fastcgi_temp` |
| `--http-uwsgi-temp-path=path` | uwsgi 临时文件目录 | `prefix/uwsgi_temp` |
| `--http-scgi-temp-path=path` | SCGI 临时文件目录 | `prefix/scgi_temp` |

### 用户与进程参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--user=name` | 工作进程使用的非特权用户 | `nobody` |
| `--group=name` | 工作进程使用的组 | 与用户同名 |
| `--build=name` | 构建名称 | - |
| `--builddir=path` | 构建目录 | - |

### HTTP 模块启用参数 (`--with-*`)

| 参数 | 说明 | 依赖 |
|------|------|------|
| `--with-http_ssl_module` | HTTPS 支持 | OpenSSL |
| `--with-http_v2_module` | HTTP/2 支持 | - |
| `--with-http_v3_module` | HTTP/3 支持 | OpenSSL 3.5+ / BoringSSL |
| `--with-http_realip_module` | 修改客户端地址 | - |
| `--with-http_addition_module` | 响应前后添加文本 | - |
| `--with-http_xslt_module` | XML 转换 | libxml2, libxslt |
| `--with-http_image_filter_module` | 图像转换 | libgd |
| `--with-http_geoip_module` | GeoIP 变量 | MaxMind 库 |
| `--with-http_sub_module` | 响应字符串替换 | - |
| `--with-http_dav_module` | WebDAV 支持 | - |
| `--with-http_flv_module` | FLV 伪流媒体 | - |
| `--with-http_mp4_module` | MP4 伪流媒体 | - |
| `--with-http_gunzip_module` | 解压 gzip 响应 | zlib |
| `--with-http_gzip_static_module` | 发送预压缩 .gz 文件 | zlib |
| `--with-http_auth_request_module` | 子请求授权 | - |
| `--with-http_random_index_module` | 随机索引文件 | - |
| `--with-http_secure_link_module` | 安全链接 | - |
| `--with-http_degradation_module` | 降级支持 | - |
| `--with-http_slice_module` | 请求分片 | - |
| `--with-http_stub_status_module` | 状态信息 | - |
| `--with-http_perl_module` | 嵌入 Perl | Perl 环境 |

### HTTP 模块禁用参数 (`--without-*`)

默认启用的模块可通过以下参数禁用：

| 参数 | 说明 |
|------|------|
| `--without-http_charset_module` | 禁用字符集模块 |
| `--without-http_gzip_module` | 禁用 gzip 压缩 |
| `--without-http_ssi_module` | 禁用 SSI |
| `--without-http_userid_module` | 禁用用户标识 |
| `--without-http_access_module` | 禁用访问控制 |
| `--without-http_auth_basic_module` | 禁用基础认证 |
| `--without-http_mirror_module` | 禁用镜像 |
| `--without-http_autoindex_module` | 禁用自动索引 |
| `--without-http_geo_module` | 禁用 Geo 模块 |
| `--without-http_map_module` | 禁用 Map 模块 |
| `--without-http_split_clients_module` | 禁用客户端分割 |
| `--without-http_referer_module` | 禁用 Referer |
| `--without-http_rewrite_module` | 禁用 URL 重写 |
| `--without-http_proxy_module` | 禁用代理 |
| `--without-http_fastcgi_module` | 禁用 FastCGI |
| `--without-http_uwsgi_module` | 禁用 uwsgi |
| `--without-http_scgi_module` | 禁用 SCGI |
| `--without-http_grpc_module` | 禁用 gRPC |
| `--without-http_memcached_module` | 禁用 Memcached |
| `--without-http_limit_conn_module` | 禁用连接限制 |
| `--without-http_limit_req_module` | 禁用请求限制 |
| `--without-http_empty_gif_module` | 禁用空 GIF |
| `--without-http_browser_module` | 禁用浏览器检测 |
| `--without-http_upstream_hash_module` | 禁用 Hash 负载均衡 |
| `--without-http_upstream_ip_hash_module` | 禁用 IP Hash |
| `--without-http_upstream_least_conn_module` | 禁用最少连接 |
| `--without-http_upstream_random_module` | 禁用随机 |
| `--without-http_upstream_keepalive_module` | 禁用 Keepalive |
| `--without-http_upstream_zone_module` | 禁用共享内存 |
| `--without-http` | 禁用整个 HTTP 服务器 |

### Mail 模块参数

| 参数 | 说明 |
|------|------|
| `--with-mail` | 启用邮件代理服务器 |
| `--with-mail_ssl_module` | 邮件代理 SSL 支持 |
| `--without-mail_pop3_module` | 禁用 POP3 |
| `--without-mail_imap_module` | 禁用 IMAP |
| `--without-mail_smtp_module` | 禁用 SMTP |

### Stream 模块参数（TCP/UDP 代理）

| 参数 | 说明 |
|------|------|
| `--with-stream` | 启用 TCP/UDP 流模块 |
| `--with-stream_ssl_module` | 流模块 SSL 支持 |
| `--with-stream_realip_module` | PROXY 协议地址修改 |
| `--with-stream_geoip_module` | 流模块 GeoIP 支持 |
| `--with-stream_ssl_preread_module` | 提取 ClientHello 信息 |
| `--without-stream_limit_conn_module` | 禁用连接限制 |
| `--without-stream_access_module` | 禁用访问控制 |
| `--without-stream_geo_module` | 禁用 Geo 模块 |
| `--without-stream_map_module` | 禁用 Map 模块 |
| `--without-stream_split_clients_module` | 禁用客户端分割 |
| `--without-stream_return_module` | 禁用 Return 模块 |
| `--without-stream_set_module` | 禁用 Set 模块 |
| `--without-stream_upstream_hash_module` | 禁用 Hash 负载均衡 |
| `--without-stream_upstream_ip_hash_module` | 禁用 IP Hash |
| `--without-stream_upstream_least_conn_module` | 禁用最少连接 |
| `--without-stream_upstream_random_module` | 禁用随机 |
| `--without-stream_upstream_zone_module` | 禁用共享内存 |

### 其他编译参数

| 参数 | 说明 |
|------|------|
| `--with-threads` | 启用线程池 |
| `--with-file-aio` | 启用异步文件 I/O |
| `--with-debug` | 启用调试日志 |
| `--with-compat` | 动态模块兼容性 |
| `--add-module=path` | 启用外部静态模块 |
| `--add-dynamic-module=path` | 启用外部动态模块 |
| `--with-cc=path` | 设置 C 编译器 |
| `--with-cc-opt=parameters` | 追加 CFLAGS |
| `--with-ld-opt=parameters` | 追加链接参数 |
| `--with-cpu-opt=cpu` | 针对特定 CPU 构建 |
| `--with-pcre=path` | 指定 PCRE 源码路径 |
| `--with-pcre-jit` | 启用 PCRE JIT |
| `--with-zlib=path` | 指定 zlib 源码路径 |
| `--with-openssl=path` | 指定 OpenSSL 源码路径 |
| `--with-google_perftools_module` | 性能分析支持 |

---

## 5. 依赖库说明

| 库 | 用途 | 必需模块 |
|-----|------|----------|
| **OpenSSL** | SSL/TLS 支持 | http_ssl, mail_ssl, stream_ssl, http_v3 |
| **PCRE** | 正则表达式支持 | http_rewrite |
| **zlib** | gzip 压缩 | http_gzip |
| **libxml2 & libxslt** | XML 转换 | http_xslt |
| **libgd** | 图像处理 | http_image_filter |
| **MaxMind** | GeoIP 功能 | http_geoip, stream_geoip |

---

## 6. 编译示例

### 基础编译

```bash
./configure \
    --sbin-path=/usr/local/nginx/nginx \
    --conf-path=/usr/local/nginx/nginx.conf \
    --pid-path=/usr/local/nginx/nginx.pid \
    --with-http_ssl_module \
    --with-pcre=../pcre2-10.39 \
    --with-zlib=../zlib-1.3
```

### 完整功能编译

```bash
./configure \
    --prefix=/usr/local/nginx \
    --user=nginx \
    --group=nginx \
    --with-http_ssl_module \
    --with-http_v2_module \
    --with-http_v3_module \
    --with-http_realip_module \
    --with-http_gzip_static_module \
    --with-http_stub_status_module \
    --with-stream \
    --with-stream_ssl_module \
    --with-mail \
    --with-mail_ssl_module \
    --with-threads \
    --with-file-aio \
    --with-debug \
    --with-pcre-jit \
    --with-openssl=../openssl-3.0.0
```

### 动态模块编译

```bash
# 编译主程序
./configure --prefix=/usr/local/nginx --with-compat
make
make install

# 编译动态模块（单独）
./configure --prefix=/usr/local/nginx --with-compat --add-dynamic-module=/path/to/module
make modules
cp objs/ngx_*.so /usr/local/nginx/modules/
```

---

## 7. 验证安装

```bash
# 检查版本
nginx -v

# 检查编译参数
nginx -V

# 测试配置
nginx -t

# 启动服务
nginx

# 或使用 systemd
systemctl start nginx
systemctl enable nginx
systemctl status nginx
```