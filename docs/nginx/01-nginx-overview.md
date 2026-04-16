# NGINX 概述与基础指南

## 1. NGINX 简介

NGINX 是一个高性能的 HTTP 和反向代理服务器，同时也是一个 IMAP/POP3/SMTP 代理服务器。NGINX 以其高并发、低资源消耗、高可靠性而闻名。

### 架构特点

- **主进程 (Master Process)**：读取和评估配置，维护工作进程
- **工作进程 (Worker Processes)**：实际处理请求，基于事件模型和操作系统依赖机制高效分发请求
- **工作进程数量**：在配置文件中定义，可固定或自动调整为可用 CPU 核心数 (`worker_processes`)

### 核心优势

- 高并发连接处理能力（单机可支持数万并发连接）
- 低内存消耗
- 反向代理与负载均衡
- 静态文件高效服务
- SSL/TLS 支持
- 模块化架构

---

## 2. 配置文件结构

### 配置文件位置

默认 `nginx.conf` 位于以下路径之一：
- `/usr/local/nginx/conf`
- `/etc/nginx`
- `/usr/local/etc/nginx`

### 指令类型

**简单指令**：名称 + 参数，以分号 `;` 结尾
```nginx
worker_processes 1;
```

**块指令**：结构同简单指令，但以花括号 `{ }` 包裹额外指令
```nginx
http {
    server {
        location / {
            root /data/www;
        }
    }
}
```

### 上下文层级

```
main (主上下文)
  ├── events (事件处理)
  ├── http (HTTP 服务)
  │     ├── server (虚拟服务器)
  │     │     ├── location (请求路由)
  │     │     └── ...
  │     └── ...
  ├── stream (TCP/UDP 流)
  │     ├── server
  │     └── ...
  └── mail (邮件代理)
        ├── server
        └── ...
```

### 注释

`#` 符号后的内容为注释：
```nginx
# 这是一个注释
worker_processes auto;  # 自动匹配 CPU 核心数
```

---

## 3. 启动、停止和重载配置

### 基本控制命令

通过可执行文件配合 `-s` 参数控制 NGINX：

```bash
nginx -s <signal>
```

**信号类型**：

| 信号 | 说明 |
|------|------|
| `stop` | 快速关闭（立即终止） |
| `quit` | 优雅关闭（等待工作进程完成当前请求） |
| `reload` | 重载配置文件 |
| `reopen` | 重新打开日志文件 |

### 示例

```bash
# 启动 nginx
nginx

# 优雅停止
nginx -s quit

# 重载配置（修改配置后需执行）
nginx -s reload

# 重新打开日志文件（日志切割后）
nginx -s reopen
```

### 使用 kill 命令发送信号

```bash
# 获取主进程 PID
ps -ax | grep nginx
cat /usr/local/nginx/logs/nginx.pid

# 发送信号
kill -s QUIT 1628
kill -HUP `cat /usr/local/nginx/logs/nginx.pid`
```

---

## 4. 信号控制详解

### 主进程支持的信号

| 信号 | 功能描述 |
|------|----------|
| `TERM, INT` | 快速关闭 (Fast Shutdown) |
| `QUIT` | 优雅关闭 (Graceful Shutdown) |
| `HUP` | 更改配置（重载配置、时区更新、启动新 worker、优雅关闭旧 worker） |
| `USR1` | 重新打开日志文件（用于日志切割） |
| `USR2` | 升级可执行文件（热升级） |
| `WINCH` | 优雅关闭工作进程 |

### 工作进程支持的信号

| 信号 | 功能描述 |
|------|----------|
| `TERM, INT` | 快速关闭 |
| `QUIT` | 优雅关闭 |
| `USR1` | 重新打开日志文件 |
| `WINCH` | 异常终止（用于调试，需启用 `debug_points`） |

---

## 5. 配置重载流程

当向主进程发送 `HUP` 信号时：

1. 检查配置语法有效性
2. 尝试应用新配置（打开日志和新监听端口）
3. 若失败：回滚变更，继续使用旧配置
4. 若成功：
   - 启动新工作进程
   - 通知旧工作进程优雅关闭
5. 旧进程关闭监听端口，服务完现有请求后退出

```bash
kill -HUP `cat /usr/local/nginx/logs/nginx.pid`
```

---

## 6. 日志切割

### 操作步骤

```bash
# 1. 重命名当前日志文件
mv /usr/local/nginx/logs/access.log /usr/local/nginx/logs/access.log.old

# 2. 向主进程发送 USR1 信号
kill -USR1 `cat /usr/local/nginx/logs/nginx.pid`

# 3. 压缩旧日志文件（可选）
gzip /usr/local/nginx/logs/access.log.old
```

---

## 7. 平滑升级可执行文件

### 升级流程

```bash
# 1. 替换可执行文件
cp /path/to/new/nginx /usr/local/nginx/sbin/nginx

# 2. 发送 USR2 启动新主进程
kill -USR2 `cat /usr/local/nginx/logs/nginx.pid`
# 此时新旧进程同时运行，PID 文件变为 nginx.pid.oldbin

# 3. 优雅关闭旧工作进程
kill -WINCH `cat /usr/local/nginx/logs/nginx.pid.oldbin`

# 4. 确认新进程正常后，关闭旧主进程
kill -QUIT `cat /usr/local/nginx/logs/nginx.pid.oldbin`
```

### 回滚方案

如果新可执行文件工作异常：

**方案 A（恢复旧 worker）**：
```bash
kill -HUP `cat /usr/local/nginx/logs/nginx.pid.oldbin`  # 旧主进程启动新工作进程
kill -QUIT `cat /usr/local/nginx/logs/nginx.pid`        # 关闭新主进程
```

**方案 B（强制停止新进程）**：
```bash
kill -TERM `cat /usr/local/nginx/logs/nginx.pid`  # 新主进程立即退出
# 若未退出，使用 KILL 强制退出
```

---

## 8. 命令行参数

| 参数 | 说明 |
|------|------|
| `-?` / `-h` | 打印帮助信息 |
| `-c <file>` | 使用指定配置文件 |
| `-e <file>` | 使用指定错误日志文件（1.19.5+） |
| `-g <directives>` | 设置全局配置指令 |
| `-p <prefix>` | 设置路径前缀 |
| `-q` | 配置测试时抑制非错误消息 |
| `-s <signal>` | 向主进程发送信号 |
| `-t` | 测试配置文件语法 |
| `-T` | 测试配置并转储内容（1.9.2+） |
| `-v` | 打印版本 |
| `-V` | 打印版本和编译参数 |

### 示例

```bash
# 测试配置
nginx -t

# 测试配置并显示内容
nginx -T

# 查看编译参数
nginx -V

# 使用指定配置文件启动
nginx -c /etc/nginx/nginx.conf

# 设置全局指令
nginx -g "pid /var/run/nginx.pid; worker_processes auto;"
```

---

## 9. PID 文件位置

默认路径：`/usr/local/nginx/logs/nginx.pid`

可在配置文件中修改：
```nginx
pid /var/run/nginx.pid;
```