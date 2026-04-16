# NGINX 源码架构深度分析

基于 nginx 1.31.0 源码（lib/nginx）的架构分析文档。

---

## 1. 目录结构概览

```
lib/nginx/src/
├── core/           # 核心模块（40+ 文件）
│   ├── nginx.c     # 主程序入口
│   ├── ngx_cycle.c # 配置周期管理
│   ├── ngx_connection.c  # 连接管理
│   ├── ngx_module.c      # 模块系统
│   └── ...
├── event/          # 事件模块
│   ├── ngx_event.c      # 事件驱动核心
│   ├── ngx_event_openssl.c  # SSL（163KB）
│   ├── modules/         # IO 多路复用实现
│   │   ├── ngx_epoll_module.c
│   │   ├── ngx_kqueue_module.c
│   │   └── ...
│   └── quic/            # QUIC/HTTP3 支持（33 文件）
├── http/           # HTTP 模块
│   ├── ngx_http.c       # HTTP 模块核心
│   ├── ngx_http_core_module.c  # HTTP 核心（149KB）
│   ├── ngx_http_request.c      # 请求处理（105KB）
│   ├── ngx_http_upstream.c     # 上游代理（187KB）
│   ├── ngx_http_variables.c    # 变量系统（70KB）
│   ├── modules/         # HTTP 子模块（63 个）
│   ├── v2/              # HTTP/2 实现
│   └── v3/              # HTTP/3 实现
├── stream/         # TCP/UDP Stream 模块（32 文件）
├── mail/           # 邮件代理模块
└── os/             # 操作系统适配
```

**源码统计**：395 个 C/H 文件，HTTP 子模块 63 个，事件模块支持 epoll/kqueue/eventport/iocp 等。

---

## 2. 核心数据结构

### 2.1 ngx_cycle_t - 配置周期

nginx 运行时配置周期的核心结构，管理整个进程的生命周期状态。

```c
// src/core/ngx_cycle.h
struct ngx_cycle_s {
    void                  ****conf_ctx;      // 配置上下文层级
    ngx_pool_t               *pool;          // 内存池

    ngx_log_t                *log;           // 日志对象
    ngx_log_t                 new_log;       // 新日志（reload）

    ngx_connection_t        **files;         // 文件描述符->连接映射表
    ngx_connection_t         *free_connections;  // 空闲连接链表
    ngx_uint_t                free_connection_n;  // 空闲连接数

    ngx_module_t            **modules;       // 模块数组
    ngx_uint_t                modules_n;     // 模块数量

    ngx_queue_t               reusable_connections_queue;  // 可复用连接队列

    ngx_array_t               listening;     // 监听端口数组
    ngx_array_t               paths;         // 路径数组

    ngx_list_t                open_files;    // 打开的文件列表
    ngx_list_t                shared_memory; // 共享内存区域列表

    ngx_uint_t                connection_n;  // 总连接数
    ngx_connection_t         *connections;   // 连接数组
    ngx_event_t              *read_events;   // 读事件数组
    ngx_event_t              *write_events;  // 写事件数组

    ngx_cycle_t              *old_cycle;     // 旧周期（用于 upgrade）

    ngx_str_t                 conf_file;     // 配置文件路径
    ngx_str_t                 conf_prefix;   // 配置前缀
    ngx_str_t                 prefix;         // 安装前缀
    ngx_str_t                 error_log;      // 错误日志路径
    ngx_str_t                 lock_file;      // 锁文件
    ngx_str_t                 hostname;       // 主机名
};
```

**生命周期阶段**：
1. `ngx_init_cycle()` - 初始化新周期（读取配置、创建监听、分配连接）
2. 配置 reload 时创建新 cycle，旧 cycle 被保留用于回滚
3. 升级时新旧 cycle 共存（`ngx_old_cycles` 数组）

### 2.2 ngx_connection_t - 连接结构

表示一个网络连接（客户端或上游）。

```c
// src/core/ngx_connection.h
struct ngx_connection_s {
    void               *data;           // 模块私有数据（HTTP 请求等）
    ngx_event_t        *read;           // 读事件
    ngx_event_t        *write;          // 写事件

    ngx_socket_t        fd;             // 文件描述符

    ngx_recv_pt         recv;           // 接收函数指针
    ngx_send_pt         send;           // 发送函数指针
    ngx_recv_chain_pt   recv_chain;     // 链式接收
    ngx_send_chain_pt   send_chain;     // 链式发送

    ngx_listening_t    *listening;      // 关联的监听端口

    off_t               sent;           // 已发送字节数

    ngx_log_t          *log;            // 日志对象

    ngx_pool_t         *pool;           // 内存池

    int                 type;           // 连接类型（SOCK_STREAM/SOCK_DGRAM）

    struct sockaddr    *sockaddr;       // 远端地址
    socklen_t           socklen;        // 地址长度
    ngx_str_t           addr_text;      // 地址文本

    ngx_proxy_protocol_t  *proxy_protocol;  // PROXY 协议信息

#if (NGX_QUIC || NGX_COMPAT)
    ngx_quic_stream_t     *quic;        // QUIC 流
#endif

#if (NGX_SSL || NGX_COMPAT)
    ngx_ssl_connection_t  *ssl;         // SSL 连接
#endif

    ngx_udp_connection_t  *udp;         // UDP 连接

    struct sockaddr    *local_sockaddr; // 本端地址
    socklen_t           local_socklen;

    ngx_buf_t          *buffer;         // 缓冲区

    ngx_queue_t         queue;          // 连接队列节点

    ngx_atomic_uint_t   number;         // 连接序号（全局唯一）
    ngx_msec_t          start_time;     // 连接开始时间
    ngx_uint_t          requests;       // 请求数（HTTP keepalive）

    unsigned            buffered:8;     // 缓冲状态位

    unsigned            log_error:3;    // 错误日志级别

    unsigned            timedout:1;     // 是否超时
    unsigned            error:1;        // 是否错误
    unsigned            destroyed:1;    // 是否已销毁
    unsigned            pipeline:1;     // 是否 pipeline 模式

    unsigned            idle:1;         // 是否空闲（keepalive）
    unsigned            reusable:1;     // 是否可复用
    unsigned            close:1;        // 是否需要关闭
    unsigned            shared:1;       // 是否共享

    unsigned            sendfile:1;     // 是否启用 sendfile
    unsigned            tcp_nodelay:2;  // TCP_NODELAY 状态
    unsigned            tcp_nopush:2;   // TCP_NOPUSH 状态
};
```

### 2.3 ngx_event_t - 事件结构

事件驱动模型的核心。

```c
// src/event/ngx_event.h
struct ngx_event_s {
    void            *data;           // 关联的连接或其他数据

    unsigned         write:1;        // 是否写事件
    unsigned         accept:1;       // 是否接受连接事件

    unsigned         instance:1;     // 实例标记（防止 stale event）

    unsigned         active:1;       // 是否已添加到事件驱动
    unsigned         disabled:1;     // 是否禁用

    unsigned         ready:1;        // 是否就绪（可读/可写）
    unsigned         oneshot:1;      // 是否一次性事件

    unsigned         complete:1;     // AIO 操作是否完成

    unsigned         eof:1;          // 是否 EOF
    unsigned         error:1;        // 是否错误

    unsigned         timedout:1;     // 是否超时
    unsigned         timer_set:1;    // 是否设置了定时器

    unsigned         delayed:1;      // 是否延迟

    unsigned         posted:1;       // 是否已投递到 posted 队列

    ngx_event_handler_pt  handler;   // 事件处理函数

    ngx_rbtree_node_t   timer;       // 定时器红黑树节点

    ngx_queue_t      queue;          // posted 队列节点
};
```

### 2.4 ngx_http_request_t - HTTP 请求结构

HTTP 请求处理的核心结构。

```c
// src/http/ngx_http_request.h
struct ngx_http_request_s {
    uint32_t                          signature;    /* "HTTP" */

    ngx_connection_t                 *connection;   // 底层连接

    void                            **ctx;          // 模块上下文数组
    void                            **main_conf;    // main 配置
    void                            **srv_conf;     // server 配置
    void                            **loc_conf;     // location 配置

    ngx_http_event_handler_pt         read_event_handler;
    ngx_http_event_handler_pt         write_event_handler;

#if (NGX_HTTP_CACHE)
    ngx_http_cache_t                 *cache;        // 缓存对象
#endif

    ngx_http_upstream_t              *upstream;     // 上游对象
    ngx_array_t                      *upstream_states;  // 上游状态记录

    ngx_pool_t                       *pool;         // 请求内存池
    ngx_buf_t                        *header_in;    // 请求头缓冲

    ngx_http_headers_in_t             headers_in;   // 请求头结构
    ngx_http_headers_out_t            headers_out;  // 响应头结构

    ngx_http_request_body_t          *request_body; // 请求体

    time_t                            start_sec;    // 请求开始时间
    ngx_msec_t                        start_msec;

    ngx_uint_t                        method;       // HTTP 方法
    ngx_uint_t                        http_version; // HTTP 版本

    ngx_str_t                         request_line; // 请求行
    ngx_str_t                         uri;          // URI
    ngx_str_t                         args;         // 参数
    ngx_str_t                         exten;        // 扩展名
    ngx_str_t                         unparsed_uri; // 原始 URI

    ngx_chain_t                      *out;          // 输出链

    ngx_http_request_t               *main;         // 主请求
    ngx_http_request_t               *parent;       // 父请求（子请求）
    ngx_http_postponed_request_t     *postponed;    // 延迟请求队列
    ngx_http_posted_request_t        *posted_requests;  // 投递请求队列

    ngx_int_t                         phase_handler; // 阶段处理位置
    ngx_http_handler_pt               content_handler; // content 处理函数

    ngx_http_variable_value_t        *variables;    // 变量值数组

#if (NGX_PCRE)
    ngx_uint_t                        ncaptures;    // 正则捕获数
    int                              *captures;     // 捕获数组
#endif

    size_t                            limit_rate;   // 限速
    off_t                             request_length; // 请求长度

    ngx_uint_t                        err_status;   // 错误状态码

    ngx_http_connection_t            *http_connection; // HTTP 连接
    ngx_http_v2_stream_t             *stream;       // HTTP/2 流
    ngx_http_v3_parse_t              *v3_parse;     // HTTP/3 解析

    unsigned                          count:16;     // 引用计数
    unsigned                          subrequests:8; // 子请求限制
    unsigned                          blocked:8;    // 阻塞计数

    unsigned                          http_state:4; // HTTP 状态

    unsigned                          pipeline:1;   // pipeline 模式
    unsigned                          chunked:1;    // chunked 编码
    unsigned                          header_only:1; // 只需头
    unsigned                          keepalive:1;  // keepalive
    unsigned                          internal:1;   // 内部请求
    unsigned                          header_sent:1; // 头已发送
    unsigned                          response_sent:1; // 响应已发送
};
```

---

## 3. 启动流程

### 3.1 main() 函数流程

```c
// src/core/nginx.c
int main(int argc, char *const *argv)
{
    // 1. 解析命令行参数
    ngx_get_options(argc, argv);

    // 2. 初始化时间、日志、内存池
    ngx_time_init();
    ngx_log_init();

    // 3. 初始化 cycle（核心）
    cycle = ngx_init_cycle(&init_cycle);

    // 4. 根据运行模式启动
    if (ngx_process == NGX_PROCESS_SINGLE) {
        // 单进程模式
        ngx_single_process_cycle(cycle);
    } else {
        // master-worker 模式
        ngx_master_process_cycle(cycle);
    }
}
```

### 3.2 ngx_init_cycle() - 配置周期初始化

```c
// src/core/ngx_cycle.c
ngx_cycle_t *ngx_init_cycle(ngx_cycle_t *old_cycle)
{
    // 1. 创建内存池
    pool = ngx_create_pool(NGX_CYCLE_POOL_SIZE, log);

    // 2. 分配 cycle 结构
    cycle = ngx_pcalloc(pool, sizeof(ngx_cycle_t));

    // 3. 保存命令行参数
    cycle->conf_file = old_cycle->conf_file;

    // 4. 解析配置文件
    conf.ctx = ngx_conf_create_context(cycle);
    ngx_conf_parse(&conf, &cycle->conf_file);

    // 5. 打开监听端口
    ngx_open_listening_sockets(cycle);

    // 6. 配置监听 socket 参数
    ngx_configure_listening_sockets(cycle);

    // 7. 分配连接和事件数组
    cycle->connections = ngx_alloc(sizeof(ngx_connection_t) * n, log);
    cycle->read_events = ngx_alloc(sizeof(ngx_event_t) * n, log);
    cycle->write_events = ngx_alloc(sizeof(ngx_event_t) * n, log);

    // 8. 初始化模块
    for (i = 0; cycle->modules[i]; i++) {
        if (cycle->modules[i]->init_module) {
            cycle->modules[i]->init_module(cycle);
        }
    }

    // 9. 初始化共享内存
    ngx_init_zones(cycle, old_cycle);

    // 10. 创建 PID 文件
    ngx_create_pidfile(&cycle->pid, log);
}
```

---

## 4. 事件驱动模型

### 4.1 ngx_event_actions_t - 事件操作接口

```c
// src/event/ngx_event.h
typedef struct {
    ngx_int_t  (*add)(ngx_event_t *ev, ngx_int_t event, ngx_uint_t flags);
    ngx_int_t  (*del)(ngx_event_t *ev, ngx_int_t event, ngx_uint_t flags);

    ngx_int_t  (*enable)(ngx_event_t *ev, ngx_int_t event, ngx_uint_t flags);
    ngx_int_t  (*disable)(ngx_event_t *ev, ngx_int_t event, ngx_uint_t flags);

    ngx_int_t  (*add_conn)(ngx_connection_t *c);
    ngx_int_t  (*del_conn)(ngx_connection_t *c, ngx_uint_t flags);

    ngx_int_t  (*notify)(ngx_event_handler_pt handler);

    ngx_int_t  (*process_events)(ngx_cycle_t *cycle, ngx_msec_t timer,
                                 ngx_uint_t flags);

    ngx_int_t  (*init)(ngx_cycle_t *cycle, ngx_msec_t timer);
    void       (*done)(ngx_cycle_t *cycle);
} ngx_event_actions_t;
```

### 4.2 事件处理主循环

```c
// src/event/ngx_event.c
void ngx_process_events_and_timers(ngx_cycle_t *cycle)
{
    // 1. 计算定时器超时
    timer = ngx_event_find_timer();

    // 2. 尝试获取 accept mutex（防止惊群）
    if (ngx_trylock_accept_mutex(cycle) == NGX_OK) {
        // 获得 mutex，设置 accept 事件
        flags |= NGX_POST_EVENTS;
    }

    // 3. 处理事件（调用 epoll_wait 等）
    ngx_process_events(cycle, timer, flags);

    // 4. 处理 posted 事件队列
    if (ngx_accept_mutex_held) {
        ngx_unlock_accept_mutex();
    }

    ngx_event_process_posted(cycle, &ngx_posted_accept_events);
    ngx_event_process_posted(cycle, &ngx_posted_events);

    // 5. 处理定时器
    ngx_event_expire_timers();
}
```

### 4.3 epoll 实现（Linux）

```c
// src/event/modules/ngx_epoll_module.c
static ngx_int_t ngx_epoll_process_events(ngx_cycle_t *cycle,
    ngx_msec_t timer, ngx_uint_t flags)
{
    // 调用 epoll_wait
    events = epoll_wait(ep, event_list, nevents, timer);

    for (i = 0; i < events; i++) {
        c = event_list[i].data.ptr;

        // 判断事件类型
        if ((event_list[i].events & EPOLLIN) && rev->active) {
            rev->ready = 1;
            if (flags & NGX_POST_EVENTS) {
                // 投递到队列
                ngx_post_event(rev, queue);
            } else {
                // 直接调用处理函数
                rev->handler(rev);
            }
        }

        if ((event_list[i].events & EPOLLOUT) && wev->active) {
            wev->ready = 1;
            // 同上处理...
        }
    }
}
```

### 4.4 定时器实现（红黑树）

```c
// src/event/ngx_event_timer.c
void ngx_event_add_timer(ngx_event_t *ev, ngx_msec_t timer)
{
    key = ngx_current_msec + timer;

    // 插入红黑树
    ngx_rbtree_insert(&ngx_event_timer_rbtree, &ev->timer);
}

ngx_msec_int_t ngx_event_find_timer(void)
{
    // 返回最小节点的超时时间
    node = ngx_rbtree_min(root, sentinel);
    timer = (ngx_msec_int_t) (node->key - ngx_current_msec);
    return timer > 0 ? timer : 0;
}
```

---

## 5. HTTP 请求处理

### 5.1 请求处理阶段

```c
// src/http/ngx_http_request.c
// HTTP 状态枚举
typedef enum {
    NGX_HTTP_INITING_REQUEST_STATE = 0,   // 初始化
    NGX_HTTP_READING_REQUEST_STATE,       // 读取请求
    NGX_HTTP_PROCESS_REQUEST_STATE,       // 处理请求

    NGX_HTTP_CONNECT_UPSTREAM_STATE,      // 连接上游
    NGX_HTTP_WRITING_UPSTREAM_STATE,      // 写入上游
    NGX_HTTP_READING_UPSTREAM_STATE,      // 读取上游

    NGX_HTTP_WRITING_REQUEST_STATE,       // 写入响应
    NGX_HTTP_LINGERING_CLOSE_STATE,       // lingering 关闭
    NGX_HTTP_KEEPALIVE_STATE              // keepalive
} ngx_http_state_e;
```

### 5.2 HTTP 处理阶段（PHASE）

nginx 的 HTTP 处理分为多个阶段，每个阶段可注册多个 handler：

| 阶段 | 名称 | 说明 |
|------|------|------|
| 0 | NGX_HTTP_POST_READ_PHASE | 读取请求头后 |
| 1 | NGX_HTTP_SERVER_REWRITE_PHASE | server 级 rewrite |
| 2 | NGX_HTTP_FIND_CONFIG_PHASE | 查找 location（内部） |
| 3 | NGX_HTTP_REWRITE_PHASE | location 级 rewrite |
| 4 | NGX_HTTP_POST_REWRITE_PHASE | rewrite 后处理（内部） |
| 5 | NGX_HTTP_PREACCESS_PHASE | access 前处理 |
| 6 | NGX_HTTP_ACCESS_PHASE | 访问控制（allow/deny/auth） |
| 7 | NGX_HTTP_POST_ACCESS_PHASE | access 后处理（内部） |
| 8 | NGX_HTTP_PRECONTENT_PHASE | content 前处理 |
| 9 | NGX_HTTP_CONTENT_PHASE | 内容生成 |
| 10 | NGX_HTTP_LOG_PHASE | 日志记录 |

### 5.3 location 匹配算法

```c
// src/http/ngx_http_core_module.c
ngx_http_core_find_location(ngx_http_request_t *r)
{
    // 使用 location tree（radix tree + 前缀匹配）
    // 精确匹配 > 正则匹配 > 前缀匹配
}
```

---

## 6. Upstream 负载均衡

### 6.1 ngx_http_upstream_t - 上游结构

```c
// src/http/ngx_http_upstream.h
struct ngx_http_upstream_s {
    ngx_http_upstream_handler_pt     read_event_handler;
    ngx_http_upstream_handler_pt     write_event_handler;

    ngx_peer_connection_t            peer;      // 对端连接

    ngx_event_pipe_t                *pipe;      // 管道（流式传输）

    ngx_chain_t                     *request_bufs;  // 请求缓冲

    ngx_http_upstream_conf_t        *conf;      // upstream 配置
    ngx_http_upstream_srv_conf_t    *upstream;  // upstream 组配置

    ngx_http_upstream_headers_in_t   headers_in;  // 上游响应头

    ngx_buf_t                        buffer;    // 响应缓冲

    ngx_int_t                      (*create_request)(ngx_http_request_t *r);
    ngx_int_t                      (*reinit_request)(ngx_http_request_t *r);
    ngx_int_t                      (*process_header)(ngx_http_request_t *r);
    void                           (*finalize_request)(ngx_http_request_t *r,
                                         ngx_int_t rc);

    ngx_msec_t                       start_time;

    ngx_http_upstream_state_t       *state;     // 状态信息

    unsigned                         store:1;
    unsigned                         cacheable:1;
    unsigned                         buffering:1;
    unsigned                         keepalive:1;
    unsigned                         upgrade:1;
};
```

### 6.2 Round Robin 数据结构

```c
// src/http/ngx_http_upstream_round_robin.h
struct ngx_http_upstream_rr_peer_s {
    ngx_str_t                       name;       // 服务器名称
    ngx_addr_t                     *addrs;      // 地址数组
    ngx_uint_t                      naddrs;     // 地址数

    ngx_uint_t                      weight;     // 权重
    ngx_uint_t                      effective_weight;  // 有效权重
    ngx_uint_t                      current_weight;    // 当前权重

    ngx_uint_t                      max_conns;  // 最大连接数
    ngx_uint_t                      conns;      // 当前连接数

    ngx_uint_t                      max_fails;  // 最大失败次数
    time_t                          fail_timeout; // 失败超时
    time_t                          accessed;   // 最后访问时间
    time_t                          checked;    // 最后检查时间

    ngx_uint_t                      fails;      // 失败次数

    ngx_uint_t                      down;       // 是否下线

    unsigned                         backup:1;   // 是否备份服务器
};

struct ngx_http_upstream_rr_peers_s {
    ngx_uint_t                      number;     // 服务器数量

    ngx_uint_t                      total_weight; // 总权重
    ngx_uint_t                      next_weight;  // 下一个权重

    ngx_http_upstream_rr_peer_t    *peer;       // 服务器数组

    ngx_http_upstream_rr_peers_t   *next;       // 下一组（backup）

    ngx_uint_t                      weighted;   // 是否加权

    ngx_str_t                      *name;       // 组名

#if (NGX_HTTP_UPSTREAM_ZONE)
    ngx_shm_zone_t                 *shm_zone;   // 共享内存区
    ngx_slab_pool_t                *shpool;     // slab 池
#endif
};
```

### 6.3 负载均衡算法实现

#### Weighted Round Robin（默认）

```c
// src/http/ngx_http_upstream_round_robin.c
ngx_http_upstream_get_peer(ngx_peer_connection_t *pc, void *data)
{
    // 平滑加权轮询算法
    // 每次选择 current_weight 最大的服务器
    // 然后将其 current_weight 减去 total_weight

    for (p = peers->peer; p; p++) {
        if (p->current_weight > best->current_weight) {
            best = p;
        }
    }

    best->current_weight -= peers->total_weight;

    return best;
}
```

#### Least Connections

```c
// src/http/modules/ngx_http_upstream_least_conn_module.c
ngx_http_upstream_get_least_conn_peer(ngx_peer_connection_t *pc, void *data)
{
    // 选择活动连接数最少的服务器
    // conns/effective_weight 作为比较因子

    for (p = peers->peer; p; p++) {
        if (p->conns * best->effective_weight
            > best->conns * p->effective_weight) {
            best = p;
        }
    }
}
```

#### Hash / Consistent Hash

```c
// src/http/modules/ngx_http_upstream_hash_module.c
// 一致性哈希实现
typedef struct {
    uint32_t                            hash;
    ngx_str_t                          *server;
} ngx_http_upstream_chash_point_t;

typedef struct {
    ngx_uint_t                          number;
    ngx_http_upstream_chash_point_t     point[1];
} ngx_http_upstream_chash_points_t;

// 虚拟节点分布在 0-2^32 的圆环上
// 根据 key 的 hash 值查找最近的节点
ngx_http_upstream_find_chash_point(points, hash);
```

---

## 7. SSL/TLS 实现

### 7.1 SSL 连接结构

```c
// src/event/ngx_event_openssl.h
typedef struct {
    ngx_ssl_conn_t           *connection;  // SSL 连接

    SSL_CTX                  *session_ctx; // 会话上下文

    ngx_int_t                 last;        // 最后操作结果

    ngx_buf_t                *buf;         // 缓冲区

    ngx_connection_handler_pt handler;     // 处理函数

    ngx_event_t              *read;        // 读事件
    ngx_event_t              *write;       // 写事件

    ngx_uint_t                buffer_size; // 缓冲区大小

    ngx_str_t                 session;     // 会话 ID

    unsigned                  handshaked:1; // 是否握手完成
    unsigned                  renegotiation:1; // 是否重协商
    unsigned                  buffer:1;    // 是否缓冲
    unsigned                  no_wait_shutdown:1;
    unsigned                  no_send_shutdown:1;
} ngx_ssl_connection_t;
```

### 7.2 SSL 握手流程

```c
// src/event/ngx_event_openssl.c
ngx_int_t ngx_ssl_handshake(ngx_connection_t *c)
{
    // 调用 SSL_do_handshake
    n = SSL_do_handshake(c->ssl->connection);

    if (n == 1) {
        // 握手完成
        c->ssl->handshaked = 1;
        return NGX_OK;
    }

    // 处理 WANT_READ/WANT_WRITE
    sslerr = SSL_get_error(c->ssl->connection, n);
}
```

---

## 8. HTTP/2 与 HTTP/3 (QUIC)

### 8.1 HTTP/2 模块结构

```
src/http/v2/
├── ngx_http_v2.c           # HTTP/2 核心逻辑
├── ngx_http_v2_filter_module.c  # 响应过滤器
├── ngx_http_v2_module.c    # 配置指令处理
├── ngx_http_v2_table.c     # HPACK 动态表
├── ngx_http_v2_encode.c    # HPACK 编码
└── ngx_http_v2.h           # 数据结构定义
```

### 8.2 HTTP/3 + QUIC 模块结构

```
src/http/v3/
├── ngx_http_v3.c           # HTTP/3 核心
├── ngx_http_v3_filter_module.c  # 响应过滤器
├── ngx_http_v3_parse.c     # QPACK 解析
├── ngx_http_v3_encode.c    # QPACK 编码
├── ngx_http_v3_table.c     # QPACK 动态表
├── ngx_http_v3_request.c   # 请求处理
├── ngx_http_v3_uni.c       # 单向流处理

src/event/quic/
├── ngx_event_quic.c        # QUIC 连接核心
├── ngx_event_quic_transport.c  # QUIC 包传输
├── ngx_event_quic_protection.c # 加密保护
├── ngx_event_quic_frames.c # QUIC 帧
├── ngx_event_quic_streams.c # QUIC 流
├── ngx_event_quic_ssl.c    # QUIC TLS 握手
├── ngx_event_quic_output.c # 输出处理
├── ngx_event_quic_ack.c    # ACK 处理
├── ngx_event_quic_migration.c # 连接迁移
├── ngx_event_quic_connid.c # 连接 ID
└── ...                     # 其他模块
```

---

## 9. Stream 模块（TCP/UDP）

### 9.1 Stream 模块结构

```
src/stream/
├── ngx_stream.c            # Stream 模块核心
├── ngx_stream_core_module.c  # 核心配置
├── ngx_stream_handler.c    # 连接处理
├── ngx_stream_proxy_module.c  # 代理模块
├── ngx_stream_upstream.c   # upstream 管理
├── ngx_stream_upstream_round_robin.c  # 负载均衡
├── ngx_stream_ssl_module.c # SSL 模块
├── ngx_stream_ssl_preread_module.c  # SSL preread（SNI 路由）
└── modules/                # 其他子模块
```

---

## 10. 模块系统架构

### 10.1 模块结构定义

```c
// src/core/ngx_module.h
struct ngx_module_s {
    ngx_uint_t              ctx_index;   // 模块类别内索引
    ngx_uint_t              index;       // 全局索引

    char                   *name;        // 模块名称

    ngx_uint_t              version;     // 版本（NGX_MODULE_V1）

    void                   *ctx;         // 模块上下文

    ngx_command_t          *commands;    // 配置指令数组

    ngx_uint_t              type;        // 模块类型

    ngx_int_t             (*init_master)(ngx_log_t *log);
    ngx_int_t             (*init_module)(ngx_cycle_t *cycle);
    ngx_int_t             (*init_process)(ngx_cycle_t *cycle);
    ngx_int_t             (*init_thread)(ngx_cycle_t *cycle);
    void                  (*exit_thread)(ngx_cycle_t *cycle);
    void                  (*exit_process)(ngx_cycle_t *cycle);
    void                  (*exit_master)(ngx_cycle_t *cycle);
};
```

### 10.2 HTTP 模块上下文

```c
// src/http/ngx_http_config.h
typedef struct {
    ngx_int_t   (*preconfiguration)(ngx_conf_t *cf);
    ngx_int_t   (*postconfiguration)(ngx_conf_t *cf);

    void       *(*create_main_conf)(ngx_conf_t *cf);
    char       *(*init_main_conf)(ngx_conf_t *cf, void *conf);

    void       *(*create_srv_conf)(ngx_conf_t *cf);
    char       *(*merge_srv_conf)(ngx_conf_t *cf, void *prev, void *conf);

    void       *(*create_loc_conf)(ngx_conf_t *cf);
    char       *(*merge_loc_conf)(ngx_conf_t *cf, void *prev, void *conf);
} ngx_http_module_t;
```

---

## 11. 内存管理

### 11.1 内存池结构

```c
// src/core/ngx_palloc.h
typedef struct {
    u_char               *last;    // 已分配末尾
    u_char               *end;     // 块末尾
    ngx_pool_t           *next;    // 下一个块
    ngx_uint_t            failed;  // 分配失败次数
} ngx_pool_data_t;

struct ngx_pool_s {
    ngx_pool_data_t       d;       // 数据区
    size_t                max;     // 最大分配大小
    ngx_pool_t           *current; // 当前块
    ngx_chain_t          *chain;   // 缓冲链
    ngx_pool_large_t     *large;   // 大块内存链
    ngx_pool_cleanup_t   *cleanup; // 清理函数链
    ngx_log_t            *log;
};
```

---

## 12. 参考资源

- nginx 源码目录：`lib/nginx/src/`
- nginx 版本：1.31.0（开发版）
- 源码文件数：395 个 C/H 文件
- HTTP 子模块：63 个
- 支持的事件机制：epoll、kqueue、eventport、iocp、poll、select
- 支持 HTTP 版本：HTTP/1.0、HTTP/1.1、HTTP/2、HTTP/3（QUIC）