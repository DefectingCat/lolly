# 部署到 FreeBSD

本示例演示如何将 Lolly 交叉编译并部署到 FreeBSD 服务器。

## 前置条件

- 本地开发机已安装 Go 1.26+
- 目标 FreeBSD 服务器可通过 SSH 访问
- 本地已配置 SSH 密钥登录（免密码）

## 部署步骤

### 1. 交叉编译

```bash
make build-freebsd
# 输出: bin/lolly-freebsd-amd64
```

### 2. 上传到服务器

```bash
scp -P 29888 bin/lolly-freebsd-amd64 root@192.168.1.15:/tmp/
```

### 3. 安装

SSH 登录到 FreeBSD 服务器执行：

```bash
# 创建目录
mkdir -p /usr/local/etc/lolly
mkdir -p /var/log/lolly
mkdir -p /var/db/lolly
mkdir -p /var/www/lolly

# 安装二进制
mv /tmp/lolly-freebsd-amd64 /usr/local/sbin/lolly
chmod 755 /usr/local/sbin/lolly

# 复制配置文件
cp /path/to/your/lolly.yaml /usr/local/etc/lolly/lolly.yaml

# 安装启动脚本
cp examples/deploy-freebsd/lolly.rc.d /usr/local/etc/rc.d/lolly
chmod 755 /usr/local/etc/rc.d/lolly

# 启用开机启动
sysrc lolly_enable="YES"

# 启动服务
service lolly start
```

## 服务管理

| 命令 | 说明 |
|------|------|
| `service lolly start` | 启动 |
| `service lolly stop` | 停止 |
| `service lolly restart` | 重启 |
| `service lolly status` | 查看状态 |
| `service lolly reload` | 热重载配置 (HUP) |
| `service lolly rotate` | 重新打开日志 (USR1) |

## 目录结构

| 路径 | 用途 |
|------|------|
| `/usr/local/sbin/lolly` | 二进制程序 |
| `/usr/local/etc/lolly/lolly.yaml` | 配置文件 |
| `/usr/local/etc/rc.d/lolly` | 启动脚本 |
| `/var/log/lolly/` | 日志目录 |
| `/var/db/lolly/` | 数据目录 |
| `/var/www/lolly/` | Web 根目录 |
| `/var/run/lolly.pid` | PID 文件 |

## 配置文件示例

最小可用配置：

```yaml
servers:
  - listen: ":8080"
    name: "localhost"
    static:
      - path: "/"
        root: "/var/www/lolly"
        index:
          - "index.html"

logging:
  format: "text"
  access:
    path: "/var/log/lolly/access.log"
  error:
    path: "/var/log/lolly/error.log"
    level: "info"
```

## 故障排查

### 检查服务状态

```bash
service lolly status
ps aux | grep lolly
sockstat -4 -l | grep 8080
```

### 查看日志

```bash
tail -f /var/log/lolly/error.log
tail -f /var/log/lolly/access.log
```

### 测试配置

```bash
/usr/local/sbin/lolly -c /usr/local/etc/lolly/lolly.yaml
# 前台运行，Ctrl+C 停止
```

### 权限问题

如果启动失败提示 Permission denied：

```bash
# 确保配置文件可读
chmod 644 /usr/local/etc/lolly/lolly.yaml
chown root:wheel /usr/local/etc/lolly/lolly.yaml

# 确保日志目录存在且可写
chown -R root:wheel /var/log/lolly
```

## 参考

- [FreeBSD Handbook - Init System](https://docs.freebsd.org/en/books/handbook/boot/)
- [rc.d 脚本规范](https://docs.freebsd.org/en/articles/rc-scripting/)
