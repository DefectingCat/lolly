# NGINX 核心模块与事件模块详解

## 1. ngx_core_module (核心模块)

ngx_core_module 是 NGINX 的核心模块，负责配置与系统资源、进程管理相关的全局指令。这些指令只能在主上下文（main context）中使用。

### 1.1 进程管理指令

#### worker_processes

设置 worker 进程的数量。

**语法**：`worker_processes number | auto;`

**默认值**：`worker_processes 1;`

**上下文**：main

```nginx
worker_processes auto;    # 自动检测 CPU 核心数（推荐）
worker_processes 4;       # 固定 4 个进程
worker_processes 8;       # 固定 8 个进程
```

**说明**：
- `auto` 会根据系统可用的 CPU 核心数自动设置
- 通常设置为 CPU 核心数或核心数的倍数
- 对于 I/O 密集型任务，可以设置为 `CPU核心数 * 2`

---

#### worker_cpu_affinity

绑定 worker 进程到特定的 CPU 核心，实现 CPU 亲和性。

**语法**：`worker_cpu_affinity cpumask ...;`

**默认值**：无

**上下文**：main

```nginx
# 4 核 CPU，每个 worker 绑定一个核心
worker_processes 4;
worker_cpu_affinity 0001 0010 0100 1000;

# 8 核 CPU，每个 worker 绑定一个核心
worker_processes 8;
worker_cpu_affinity 00000001 00000010 00000100 00001000 00010000 00100000 01000000 10000000;

# 使用 auto 模式（自动绑定）
worker_cpu affinity auto;
```

**位掩码说明**：
- 每一位代表一个 CPU 核心，从右到左（LSB 到 MSB）
- `0001` 表示绑定到 CPU0
- `0010` 表示绑定到 CPU1
- `0100` 表示绑定到 CPU2
- `1000` 表示绑定到 CPU3

**优势**：
- 减少 CPU 缓存失效（cache miss）
- 避免进程在不同核心间迁移
- 提升缓存命中率

---

#### worker_rlimit_nofile

设置每个 worker 进程可以打开的最大文件描述符数量。

**语法**：`worker_rlimit_nofile number;`

**默认值**：无（使用系统默认值）

**上下文**：main

```nginx
worker_rlimit_nofile 65535;
worker_rlimit_nofile 100000;
```

**计算建议**：
```
worker_rlimit_nofile = worker_connections * 2 + 系统保留
```

**注意**：不能超过系统的 `fs.file-max` 限制。

---

#### worker_rlimit_core

设置每个 worker 进程核心转储文件（core dump）的最大大小。

**语法**：`worker_rlimit_core size;`

**默认值**：无

**上下文**：main

```nginx
worker_rlimit_core 500M;
worker_rlimit_core 1G;
```

---

#### worker_shutdown_timeout

设置 worker 进程优雅关闭的超时时间。

**语法**：`worker_shutdown_timeout time;`

**默认值**：无

**上下文**：main

```nginx
worker_shutdown_timeout 10s;
```

---

### 1.2 日志与调试指令

#### error_log

配置错误日志的路径和级别。

**语法**：`error_log file [level];` 或 `error_log stderr [level];`

**默认值**：`error_log logs/error.log error;`

**上下文**：main, http, mail, stream, server, location

```nginx
# 基本配置
error_log /var/log/nginx/error.log;
error_log /var/log/nginx/error.log warn;
error_log stderr;                    # 输出到标准错误
error_log stderr debug;              # 调试级别
error_log off;                       # 关闭错误日志

# 不同上下文设置不同级别
error_log /var/log/nginx/error.log error;
http {
    error_log /var/log/nginx/http_error.log warn;
    server {
        error_log /var/log/nginx/server_error.log info;
    }
}
```

**日志级别**（从低到高）：

| 级别 | 说明 |
|------|------|
| `debug` | 调试信息（需要 `--with-debug` 编译） |
| `info` | 信息性消息 |
| `notice` | 正常但重要的消息 |
| `warn` | 警告消息 |
| `error` | 处理请求时的错误 |
| `crit` | 临界条件 |
| `alert` | 必须立即处理的条件 |
| `emerg` | 系统不可用 |

---

#### debug_points

设置调试行为。

**语法**：`debug_points abort | stop;`

**默认值**：无

**上下文**：main

```nginx
debug_points abort;     # 遇到调试点时产生核心转储
debug_points stop;      # 遇到调试点时停止进程
```

---

#### master_process

启用或禁用 master 进程模式。

**语法**：`master_process on | off;`

**默认值**：`master_process on;`

**上下文**：main

```nginx
master_process off;     # 单进程模式（仅用于开发调试）
```

**注意**：生产环境必须保持 `on`，禁用 master 进程会导致无法热重载配置。

---

#### daemon

设置是否以守护进程模式运行。

**语法**：`daemon on | off;`

**默认值**：`daemon on;`

**上下文**：main

```nginx
daemon off;             # 前台运行（用于 Docker 或 systemd）
```

---

### 1.3 进程标识指令

#### pid

设置存储 NGINX master 进程 PID 的文件路径。

**语法**：`pid file;`

**默认值**：`pid logs/nginx.pid;`

**上下文**：main

```nginx
pid /var/run/nginx.pid;
pid /run/nginx/nginx.pid;
```

---

#### lock_file

设置锁文件的路径。

**语法**：`lock_file file;`

**默认值**：编译时指定

**上下文**：main

```nginx
lock_file /var/run/nginx.lock;
```

---

### 1.4 用户权限指令

#### user

设置 worker 进程运行的用户和组。

**语法**：`user user [group];`

**默认值**：`user nobody nobody;`

**上下文**：main

```nginx
user nginx;                    # 仅指定用户，组与用户同名
user nginx nginx;              # 指定用户和组
user www-data www-data;        # Debian/Ubuntu 默认
```

**注意**：master 进程以 root 运行，worker 进程以此处指定的用户运行。

---

#### group

单独设置 worker 进程的组（从 1.21.5 版本开始）。

**语法**：`group group;`

**默认值**：无

**上下文**：main

```nginx
group nginx;
```

---

### 1.5 环境变量指令

#### env

定义环境变量，允许 NGINX 保留或修改这些变量。

**语法**：`env variable[=value];`

**默认值**：`env TZ;`

**上下文**：main

```nginx
env MYPATH;
env MYPATH=/usr/local/bin;
env PERL5LIB=/path/to/perl/lib;
env OPENSSL_ALLOW_PROXY_CERTS=1;
```

---

### 1.6 配置文件指令

#### include

包含其他配置文件。

**语法**：`include file | mask;`

**上下文**：任意

```nginx
# 包含单个文件
include /etc/nginx/mime.types;

# 包含通配符匹配的文件
include /etc/nginx/conf.d/*.conf;
include /etc/nginx/sites-enabled/*;

# 包含多个特定文件
include /etc/nginx/conf.d/http.conf;
include /etc/nginx/conf.d/stream.conf;
```

---

### 1.7 系统相关指令

#### timer_resolution

设置系统调用 `gettimeofday()` 的时间戳解析度，减少调用次数。

**语法**：`timer_resolution interval;`

**默认值**：无

**上下文**：main

```nginx
timer_resolution 100ms;     # 每 100ms 更新一次时间戳
timer_resolution 1s;        # 每 1s 更新一次时间戳
```

**作用**：
- 减少 `gettimeofday()` 系统调用次数
- 降低 CPU 使用率
- 适用于高并发场景

**注意**：日志时间戳和限速功能可能不够精确。

---

#### working_directory

设置 worker 进程的工作目录，用于写入核心转储文件。

**语法**：`working_directory directory;`

**默认值**：编译时前缀目录

**上下文**：main

```nginx
working_directory /var/lib/nginx;
working_directory /var/crash/nginx;
```

---

#### pcre_jit

启用 PCRE JIT（Just-In-Time）编译，加速正则表达式处理。

**语法**：`pcre_jit on | off;`

**默认值**：`pcre_jit off;`

**上下文**：main

```nginx
pcre_jit on;
```

**要求**：需要 PCRE 库支持 JIT 编译。

---

### 1.8 线程池指令

#### thread_pool

定义用于多线程读取和发送文件的线程池配置。

**语法**：`thread_pool name threads=number [max_queue=number];`

**默认值**：`thread_pool default threads=32 max_queue=65536;`

**上下文**：main

```nginx
thread_pool default threads=32 max_queue=65536;
thread_pool io_pool threads=64 max_queue=131072;

# 在 location 中使用
location /videos/ {
    aio threads=io_pool;
}
```

---

## 2. ngx_events_module (事件模块)

ngx_events_module 是 NGINX 的事件处理核心模块，配置在独立的 `events` 块中。

### 2.1 基础配置结构

```nginx
events {
    # 事件模块配置
    worker_connections 1024;
    use epoll;
    multi_accept on;
}
```

**注意**：`events` 块只能出现在主上下文中，且每个配置文件只能有一个。

---

### 2.2 连接管理指令

#### worker_connections

设置每个 worker 进程可以同时处理的最大连接数。

**语法**：`worker_connections number;`

**默认值**：`worker_connections 512;`

**上下文**：events

```nginx
events {
    worker_connections 1024;    # 开发环境
    worker_connections 4096;    # 生产环境
    worker_connections 10240;   # 高并发环境
}
```

**连接数计算**：
```
总并发连接数 = worker_processes * worker_connections
```

**注意**：一个连接可能占用 2-3 个文件描述符（客户端连接 + 上游连接）。

---

### 2.3 事件处理指令

#### use

指定使用的事件处理方法。

**语法**：`use method;`

**默认值**：自动检测（根据操作系统选择最优方法）

**上下文**：events

```nginx
events {
    use epoll;        # Linux 2.6+
    use kqueue;       # FreeBSD/macOS
    use /dev/poll;    # Solaris
    use eventport;    # Solaris 10+
    use select;       # 通用（效率低）
    use poll;         # 通用（效率低）
}
```

**可用方法**：
- Linux：`epoll`, `select`, `poll`
- FreeBSD/macOS：`kqueue`, `select`, `poll`
- Solaris：`/dev/poll`, `eventport`, `select`, `poll`

---

#### multi_accept

设置 worker 进程是否一次接受多个新连接。

**语法**：`multi_accept on | off;`

**默认值**：`multi_accept off;`

**上下文**：events

```nginx
events {
    multi_accept on;      # 一次 accept 所有可用连接
    multi_accept off;     # 一次 accept 一个连接（默认）
}
```

**说明**：
- `on`：在 `epoll`/`kqueue` 触发时，尽可能多地接受新连接
- `off`：每次只接受一个新连接，然后回到事件循环

**适用场景**：
- 高并发短连接：建议开启
- 长连接/WebSocket：建议关闭

---

#### accept_mutex

启用 worker 进程间的 accept 互斥锁，防止惊群问题。

**语法**：`accept_mutex on | off;`

**默认值**：`accept_mutex off;`（1.11.3+）

**上下文**：events

```nginx
events {
    accept_mutex on;      # 启用互斥锁（旧版本默认）
    accept_mutex off;     # 禁用互斥锁（现代 Linux 推荐）
}
```

**说明**：
- Linux 2.6.39+ 使用 `EPOLLEXCLUSIVE`/`SO_REUSEPORT`，不需要互斥锁
- 旧版本系统建议开启，防止惊群问题
- 现代系统建议关闭，减少锁竞争

---

#### accept_mutex_delay

设置 worker 进程尝试重新获取 accept 互斥锁的间隔时间。

**语法**：`accept_mutex_delay time;`

**默认值**：`accept_mutex_delay 500ms;`

**上下文**：events

```nginx
events {
    accept_mutex on;
    accept_mutex_delay 100ms;    # 快速重试
    accept_mutex_delay 1s;       # 较慢重试
}
```

---

### 2.4 调试指令

#### debug_connection

启用对特定客户端连接的调试日志。

**语法**：`debug_connection address | CIDR | unix:;`

**默认值**：无

**上下文**：events

```nginx
events {
    # 调试特定 IP
    debug_connection 192.168.1.1;

    # 调试网段
    debug_connection 192.168.1.0/24;

    # 调试本地 socket
    debug_connection unix:;

    # 调试多个来源
    debug_connection 127.0.0.1;
    debug_connection 10.0.0.0/8;
}
```

**要求**：NGINX 必须使用 `--with-debug` 编译。

---

## 3. 连接处理方法详解

### 3.1 epoll (Linux)

`epoll` 是 Linux 2.6 内核引入的高效 I/O 多路复用机制。

#### 工作原理

```
┌─────────────────────────────────────────┐
│              用户空间                    │
│  ┌──────────┐      ┌───────────────┐   │
│  │ Worker 1 │      │ epoll_wait()  │   │
│  │ Worker 2 │      │               │   │
│  │ Worker 3 │      │ 获取就绪事件   │   │
│  └──────────┘      └───────────────┘   │
│         │              │                │
│         ▼              ▼                │
│  ┌───────────────────────────────┐     │
│  │       epoll 实例（内核）       │     │
│  │  ┌─────┐ ┌─────┐ ┌─────┐     │     │
│  │  │ FD1 │ │ FD2 │ │ FD3 │ ... │     │
│  │  └─────┘ └─────┘ └─────┘     │     │
│  └───────────────────────────────┘     │
└─────────────────────────────────────────┘
```

#### 优势

- **无文件描述符数量限制**：仅受系统内存限制
- **O(1) 复杂度**：添加、删除、查询都是常数时间
- **边缘触发（ET）和水平触发（LT）**：支持两种模式
- **内核态存储**：不需要在每次调用时传递整个 fd 集合

#### 触发模式

**水平触发（Level Triggered, LT）**：
```c
// 只要 fd 处于可读/可写状态，epoll_wait 就会返回
epoll_ctl(epfd, EPOLL_CTL_ADD, fd, {EPOLLIN, ...});
```

**边缘触发（Edge Triggered, ET）**：
```c
// 仅在状态变化时通知，需要一次性读取所有数据
epoll_ctl(epfd, EPOLL_CTL_ADD, fd, {EPOLLIN | EPOLLET, ...});
```

NGINX 使用边缘触发模式，要求：
- 必须设置 socket 为非阻塞模式
- 必须循环读取直到 `EAGAIN`

---

### 3.2 kqueue (FreeBSD/macOS)

`kqueue` 是 FreeBSD 引入的高性能事件通知机制，macOS 也支持。

#### 工作原理

```
┌─────────────────────────────────────────┐
│              用户空间                    │
│  ┌──────────┐      ┌───────────────┐   │
│  │ Worker 1 │      │ kevent()      │   │
│  │ Worker 2 │      │               │   │
│  │ Worker 3 │      │ 获取 kevent   │   │
│  └──────────┘      └───────────────┘   │
│         │              │                │
│         ▼              ▼                │
│  ┌───────────────────────────────┐     │
│  │       kqueue 实例（内核）      │     │
│  │  ┌─────────────────────┐      │     │
│  │  │  内核事件队列        │      │     │
│  │  │  EVFILT_READ        │      │     │
│  │  │  EVFILT_WRITE       │      │     │
│  │  │  EVFILT_TIMER       │      │     │
│  │  │  EVFILT_SIGNAL      │      │     │
│  │  └─────────────────────┘      │     │
│  └───────────────────────────────┘     │
└─────────────────────────────────────────┘
```

#### 特点

- **高效的事件过滤**：支持多种事件类型（socket、文件、进程、信号、定时器）
- **原子操作**：添加和获取事件是原子操作
- **无惊群问题**：支持 `EV_DISPATCH` 模式

#### 事件类型

| 过滤器 | 说明 |
|--------|------|
| `EVFILT_READ` | 文件描述符可读 |
| `EVFILT_WRITE` | 文件描述符可写 |
| `EVFILT_TIMER` | 定时器到期 |
| `EVFILT_SIGNAL` | 信号到达 |
| `EVFILT_PROC` | 进程事件 |

---

### 3.3 /dev/poll (Solaris)

Solaris 特有的 I/O 多路复用机制。

#### 使用方法

```c
// 打开 /dev/poll 设备
int dpfd = open("/dev/poll", O_RDWR);

// 写入要监视的文件描述符
write(dpfd, &pollfd_array, sizeof(pollfd_array));

// 获取就绪事件
ioctl(dpfd, DP_POLL, &dvpoll);
```

#### 特点

- **状态持久化**：写入的 fd 会一直监视，直到被显式移除
- **避免重复传递 fd 集合**：与 `poll()` 不同，不需要每次传递整个集合
- **Solaris 原生支持**：在该平台性能优异

---

### 3.4 eventport (Solaris)

Solaris 10+ 引入的高性能事件端口机制。

#### 使用方法

```c
// 创建事件端口
int port = port_create();

// 关联文件描述符
port_associate(port, PORT_SOURCE_FD, fd, POLLIN, user_data);

// 获取事件
port_get(port, &event, NULL);
```

#### 特点

- **自动重新关联**：支持 `PORT_SOURCE_FD` 自动重新关联
- **多事件源**：支持文件、进程、信号、定时器等多种事件源
- **Solaris 推荐**：Solaris 10+ 的首选事件机制

---

### 3.5 select/poll (通用)

标准的 POSIX I/O 多路复用机制，几乎所有平台都支持。

#### select

```c
fd_set readfds;
FD_ZERO(&readfds);
FD_SET(fd, &readfds);
select(fd + 1, &readfds, NULL, NULL, &timeout);
```

**限制**：
- 文件描述符数量限制（通常是 1024）
- O(n) 线性扫描
- 每次调用需要重新设置 fd_set

#### poll

```c
struct pollfd fds[] = {{fd, POLLIN, 0}};
poll(fds, nfds, timeout);
```

**改进**：
- 无 fd 数量限制
- 但仍然是 O(n) 线性扫描

#### 适用场景

- 低并发环境
- 需要跨平台兼容性
- 嵌入式系统

---

## 4. 各平台最佳配置

### 4.1 Linux (2.6.39+)

```nginx
# /etc/nginx/nginx.conf
user nginx;
worker_processes auto;
worker_cpu_affinity auto;
worker_rlimit_nofile 65535;

error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 10240;
    use epoll;
    multi_accept on;
    accept_mutex off;           # Linux 2.6.39+ 不需要
}

http {
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    # ...
}
```

### 4.2 FreeBSD

```nginx
# /usr/local/etc/nginx/nginx.conf
user www;
worker_processes auto;
worker_rlimit_nofile 65535;

events {
    worker_connections 10240;
    use kqueue;
    multi_accept on;
    accept_mutex off;
}
```

### 4.3 macOS (开发环境)

```nginx
# /usr/local/etc/nginx/nginx.conf
user nobody;
worker_processes auto;
worker_rlimit_nofile 10240;

events {
    worker_connections 1024;
    use kqueue;
    multi_accept off;           # 开发环境建议关闭
}
```

### 4.4 Solaris

```nginx
# /etc/nginx/nginx.conf
user webservd;
worker_processes auto;
worker_rlimit_nofile 65535;

events {
    worker_connections 10240;
    use /dev/poll;              # 或 eventport
    multi_accept on;
}
```

---

## 5. 性能调优建议

### 5.1 Worker 进程优化

```nginx
# 匹配 CPU 核心数
worker_processes auto;
worker_cpu_affinity auto;

# 提高文件描述符限制
worker_rlimit_nofile 65535;

# 开发调试时
# master_process off;         # 单进程模式（仅开发）
# daemon off;                 # 前台运行（Docker/systemd）
```

### 5.2 事件处理优化

```nginx
events {
    # 高并发场景
    worker_connections 10240;

    # 使用最优事件机制
    use epoll;                  # Linux
    # use kqueue;              # FreeBSD/macOS

    # 连接接受策略
    multi_accept on;            # 高并发短连接
    multi_accept off;           # 长连接/WebSocket

    # 互斥锁（现代 Linux 不需要）
    accept_mutex off;
}
```

### 5.3 内核参数优化

```bash
# /etc/sysctl.conf

# 连接队列
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 65535
net.ipv4.tcp_max_syn_backlog = 65535

# TCP 优化
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_keepalive_time = 1200
net.ipv4.tcp_keepalive_intvl = 15
net.ipv4.tcp_keepalive_probes = 5

# 端口范围
net.ipv4.ip_local_port_range = 1024 65535

# 文件描述符
fs.file-max = 2097152
fs.nr_open = 2097152
```

### 5.4 文件描述符限制

```bash
# /etc/security/limits.conf
nginx   soft    nofile  65535
nginx   hard    nofile  65535

# 或使用 systemd
# /etc/systemd/system/nginx.service.d/override.conf
[Service]
LimitNOFILE=65535
```

---

## 6. 连接数计算公式

### 6.1 基本公式

```
总并发连接数 = worker_processes × worker_connections
```

### 6.2 详细计算

#### Web 服务器场景

```
所需 worker_connections = (预期并发连接数 / worker_processes) × 2

# 例如：预期 100,000 并发，8 个 worker
# worker_connections = (100000 / 8) × 2 = 25,000
```

**乘以 2 的原因**：
- 客户端连接占用 1 个 fd
- 如果反向代理，上游连接占用 1 个 fd

#### 反向代理场景

```
所需 worker_connections = (
    预期并发连接数 +
    (upstream_keepalive × upstream_count)
) / worker_processes × 2

# 例如：预期 50,000 并发，4 个 worker
# 2 个 upstream，每个 keepalive 32
# worker_connections = (50000 + 64) / 4 × 2 ≈ 25,032
```

### 6.3 系统限制检查

```bash
# 检查系统文件描述符限制
cat /proc/sys/fs/file-max

# 检查进程限制
ulimit -n

# 检查 NGINX 实际使用
ss -s
cat /proc/$(pgrep -o nginx)/limits | grep "Max open files"
```

### 6.4 配置示例

```nginx
# 支持 100,000 并发连接的完整配置
user nginx;
worker_processes 16;                    # 16 核服务器
worker_cpu_affinity auto;
worker_rlimit_nofile 200000;            # 必须 > worker_connections * 2

events {
    worker_connections 65535;           # 100000/16 ≈ 6250，留足余量
    use epoll;
    multi_accept on;
    accept_mutex off;
}

http {
    # 长连接优化
    keepalive_timeout 60s;
    keepalive_requests 10000;

    # 上游连接池
    upstream backend {
        server 192.168.1.1:8080;
        server 192.168.1.2:8080;
        keepalive 256;
        keepalive_timeout 60s;
        keepalive_requests 10000;
    }
}
```

### 6.5 监控指标

```nginx
# 启用 stub_status 监控
server {
    location /nginx_status {
        stub_status;
        allow 127.0.0.1;
        deny all;
    }
}
```

**关键指标解读**：

```
Active connections: 291              # 当前活跃连接数
server accepts handled requests      # 总接受/处理/请求数
 16630948 16630948 31070465
Reading: 6 Writing: 128 Waiting: 157 # 读/写/等待状态连接数
```

**连接状态说明**：
- `Reading`：正在读取请求头
- `Writing`：正在处理请求或发送响应
- `Waiting`：保持连接（keep-alive），等待新请求

---

## 7. 完整配置示例

### 7.1 高性能 Web 服务器

```nginx
user nginx;
worker_processes auto;
worker_cpu_affinity auto;
worker_rlimit_nofile 100000;

error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 65535;
    use epoll;
    multi_accept on;
    accept_mutex off;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for" '
                    'rt=$request_time uct="$upstream_connect_time" '
                    'uht="$upstream_header_time" urt="$upstream_response_time"';

    access_log /var/log/nginx/access.log main buffer=32k flush=5s;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    keepalive_requests 10000;

    open_file_cache max=10000 inactive=20s;
    open_file_cache_valid 30s;
    open_file_cache_min_uses 2;

    gzip on;
    gzip_comp_level 6;
    gzip_min_length 1000;
    gzip_types text/plain text/css application/json application/javascript;

    server {
        listen 80 backlog=65535;
        server_name example.com;

        location / {
            root /var/www/html;
            try_files $uri $uri/ =404;
        }
    }
}
```

### 7.2 高性能反向代理

```nginx
user nginx;
worker_processes auto;
worker_rlimit_nofile 200000;

error_log /var/log/nginx/error.log warn;

events {
    worker_connections 65535;
    use epoll;
    multi_accept on;
    accept_mutex off;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # 上游服务器
    upstream backend {
        zone upstream_backend 64k;
        server 192.168.1.10:8080 weight=5;
        server 192.168.1.11:8080 weight=5;
        server 192.168.1.12:8080 backup;

        keepalive 256;
        keepalive_timeout 60s;
        keepalive_requests 10000;
    }

    # 代理优化
    proxy_cache_path /var/cache/nginx levels=1:2 keys_zone=main:100m max_size=1g;

    server {
        listen 80 backlog=65535;

        location / {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Connection "";

            proxy_buffering on;
            proxy_buffer_size 8k;
            proxy_buffers 8 32k;
            proxy_busy_buffers_size 64k;

            proxy_connect_timeout 5s;
            proxy_send_timeout 30s;
            proxy_read_timeout 30s;

            proxy_cache main;
            proxy_cache_key $scheme$request_method$host$request_uri;
            proxy_cache_valid 200 10m;
            proxy_cache_valid 404 1m;
        }
    }
}
```

---

## 8. 常见问题排查

### 8.1 "too many open files" 错误

**原因**：
- `worker_rlimit_nofile` 设置过低
- 系统 `fs.file-max` 限制

**解决**：
```nginx
worker_rlimit_nofile 65535;
```

```bash
# 永久修改
echo "fs.file-max = 2097152" >> /etc/sysctl.conf
sysctl -p

# 检查
ulimit -n
```

### 8.2 "worker_connections are not enough" 错误

**原因**：并发连接数超过 `worker_connections` 限制

**解决**：
```nginx
events {
    worker_connections 10240;    # 增加连接数
}
```

### 8.3 性能下降排查

```bash
# 检查 worker 进程是否均匀分布
ps -eo pid,psr,comm | grep nginx

# 检查连接状态
ss -ant | awk '{print $1}' | sort | uniq -c

# 检查系统负载
top -p $(pgrep -d',' nginx)

# 查看文件描述符使用
cat /proc/$(pgrep -o nginx)/limits
```

### 8.4 热升级失败

**原因**：
- PID 文件路径错误
- 权限不足

**检查**：
```nginx
pid /var/run/nginx.pid;    # 确保路径正确
```

```bash
# 检查 PID 文件
ls -la /var/run/nginx.pid

# 手动指定 PID 路径
nginx -s reload -p /etc/nginx -c nginx.conf
```
