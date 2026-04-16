# NGINX 安全深度指南

## 目录

1. [Bot 检测与防护](#1-bot-检测与防护)
2. [WAF 配置深度指南（ModSecurity）](#2-waf-配置深度指南modsecurity)
3. [DDoS 防护策略](#3-ddos-防护策略)
4. [OWASP Top 10 防护](#4-owasp-top-10-防护)
5. [安全响应头完整配置](#5-安全响应头完整配置)
6. [CVE 历史漏洞与修复版本](#6-cve-历史漏洞与修复版本)
7. [安全审计日志配置](#7-安全审计日志配置)
8. [完整安全配置示例](#8-完整安全配置示例)

---

## 1. Bot 检测与防护

### 1.1 基于 User-Agent 的 Bot 识别

```nginx
map $http_user_agent $bot_type {
    default "human";

    # 善意爬虫（白名单）
    ~*googlebot "good_bot";
    ~*bingbot "good_bot";
    ~*duckduckbot "good_bot";
    ~*baiduspider "good_bot";
    ~*yandexbot "good_bot";
    ~*facebookexternalhit "good_bot";
    ~*twitterbot "good_bot";
    ~*linkedinbot "good_bot";
    ~*whatsapp "good_bot";

    # 恶意爬虫/扫描器
    ~*sqlmap "bad_bot";
    ~*nikto "bad_bot";
    ~*masscan "bad_bot";
    ~*zgrab "bad_bot";
    ~*nmap "bad_bot";
    ~*nessus "bad_bot";
    ~*burp "bad_bot";
    ~*gobuster "bad_bot";
    ~*dirbuster "bad_bot";
    ~*wfuzz "bad_bot";
    ~*crawler4j "bad_bot";
    ~*scrapy "bad_bot";
    ~*python-requests "bad_bot";

    # 自动化工具
    ~*curl "tool";
    ~*wget "tool";
    ~*python-urllib "tool";
    ~*java "tool";
    ~*libwww-perl "tool";
    ~*httpclient "tool";
}

# 基于行为的检测
map $http_user_agent $suspicious_behavior {
    default 0;
    ~*bot 1;
    ~*spider 1;
    ~*crawl 1;
    ~*scan 1;
    ~*probe 1;
}
```

### 1.2 验证爬虫真实性

```nginx
server {
    listen 80;
    server_name example.com;

    # 白名单爬虫验证（反向 DNS 验证）
    location / {
        # 仅对白名单爬虫进行验证
        if ($bot_type = "good_bot") {
            # 验证 Googlebot
            if ($http_user_agent ~* googlebot) {
                # 实际生产环境需使用 Lua/NJS 进行反向 DNS 验证
                # 此处仅示例
                access_log /var/log/nginx/verified_bots.log;
            }
        }

        # 拦截恶意爬虫
        if ($bot_type = "bad_bot") {
            return 444;  # 无响应断开连接
        }

        # 限制自动化工具
        if ($bot_type = "tool") {
            limit_req zone=tool_limit burst=1 nodelay;
        }

        proxy_pass http://backend;
    }
}
```

### 1.3 基于行为的 Bot 检测

```nginx
# 定义 Bot 检测区域
limit_req_zone $binary_remote_addr zone=bot_detect:10m rate=1r/s;

server {
    # 无 User-Agent 检测
    if ($http_user_agent = "") {
        return 403;
    }

    # 高频请求检测
    location / {
        # 检查请求频率异常
        limit_req zone=bot_detect burst=5 nodelay;

        # 检测无 referer 的 POST 请求（常见于 CSRF/Bot）
        if ($request_method = POST) {
            set $post_check 1;
        }
        if ($http_referer = "") {
            set $post_check "${post_check}1";
        }
        if ($post_check = "11") {
            return 403 "Suspicious request detected";
        }

        # 检测异常 header 组合
        if ($http_accept = "") {
            return 403;
        }

        proxy_pass http://backend;
    }
}
```

### 1.4 挑战-响应机制（使用 Lua）

```nginx
# 需要 ngx_http_lua_module
server {
    location / {
        access_by_lua_block {
            local bot_type = ngx.var.bot_type

            if bot_type == "suspicious" then
                -- 发送 JavaScript 挑战
                ngx.header.content_type = "text/html"
                ngx.say([[
                    <html>
                    <script>
                        document.cookie = "js_enabled=1; path=/";
                        window.location.reload();
                    </script>
                    </html>
                ]])
                ngx.exit(ngx.HTTP_OK)
            end

            -- 验证 cookie
            local cookies = ngx.var.http_cookie
            if not cookies or not cookies:match("js_enabled=1") then
                ngx.exit(ngx.HTTP_FORBIDDEN)
            end
        }

        proxy_pass http://backend;
    }
}
```

### 1.5 Bot 管理策略表

| 类别 | 特征 | 处理方式 |
|------|------|----------|
| **善意爬虫** | Google/Bing/Baidu 等搜索引擎 | 验证真实性后放行，设置独立 rate limit |
| **监控 Bot** | Uptime 检查、SEO 工具 | IP 白名单，设置宽松的限制 |
| **恶意爬虫** | 扫描器、暴力破解工具 | 立即阻断，记录日志 |
| **自动化工具** | curl/wget/脚本 | 严格限制频率，验证码挑战 |
| **未知 Bot** | 未识别的 User-Agent | 行为分析，可疑则挑战验证 |

---

## 2. WAF 配置深度指南（ModSecurity）

### 2.1 ModSecurity 编译安装

```bash
# 安装依赖
apt-get install -y libpcre3-dev libxml2-dev libcurl4-openssl-dev \
    liblua5.3-dev libssl-dev libyajl-dev

# 下载 ModSecurity v3
cd /usr/src
git clone --depth 1 -b v3/master https://github.com/SpiderLabs/ModSecurity
cd ModSecurity
git submodule init
git submodule update

# 编译安装
./build.sh
./configure --with-yajl --with-ssdeep --with-lua --with-maxmind
make -j$(nproc)
make install

# 下载 NGINX 连接器
cd /usr/src
git clone --depth 1 https://github.com/SpiderLabs/ModSecurity-nginx

# 编译 NGINX 模块（需 NGINX 源码）
cd /usr/src/nginx-1.24.0
./configure --add-module=/usr/src/ModSecurity-nginx [其他配置选项]
make modules
```

### 2.2 核心配置

```nginx
# modsecurity.conf - 主配置文件
load_module modules/ngx_http_modsecurity_module.so;

http {
    # 全局 ModSecurity 配置
    modsecurity on;
    modsecurity_rules_file /etc/nginx/modsecurity/modsecurity.conf;

    server {
        listen 80;
        server_name example.com;

        location / {
            # 可为特定 location 启用/禁用
            modsecurity on;

            # 内联规则
            modsecurity_rules '
                SecRuleEngine On
                SecRule REQUEST_URI "@contains /admin" \
                    "id:1000,phase:1,deny,status:403,msg:'Admin access restricted'"
            ';

            proxy_pass http://backend;
        }

        # 静态资源跳过 WAF
        location ~* \\.(jpg|jpeg|png|gif|css|js|woff|woff2)$ {
            modsecurity off;
            proxy_pass http://backend;
        }
    }
}
```

### 2.3 ModSecurity 核心规则集（CRS）

```ini
# /etc/nginx/modsecurity/crs-setup.conf

# 规则引擎模式
SecRuleEngine DetectionOnly    # 仅检测模式（测试阶段）
# SecRuleEngine On            # 阻断模式（生产环境）

# 异常评分阈值
SecAction "id:900001,phase:1,nolog,pass,t:none,setvar:tx.inbound_anomaly_score_threshold=5"
SecAction "id:900002,phase:1,nolog,pass,t:none,setvar:tx.outbound_anomaly_score_threshold=4"

# 检测 Paranoia Level（0-4，越高越严格）
SecAction "id:900003,phase:1,nolog,pass,t:none,setvar:tx.paranoia_level=2"

# 允许特定路径（排除规则）
SecRule REQUEST_URI "@beginsWith /api/webhook" \
    "id:1001,phase:1,pass,nolog,ctl:ruleEngine=Off"

# 自定义白名单
SecRule REQUEST_HEADERS:User-Agent "@contains MyInternalApp" \
    "id:1002,phase:1,pass,nolog,ctl:ruleRemoveById=913100"

# 自定义规则 - SQL 注入增强
SecRule REQUEST_COOKIES|REQUEST_COOKIES_NAMES|REQUEST_FILENAME|ARGS_NAMES|ARGS|XML:/* \
    "@rx (?i:(?:select\\s*\\*\\s*from|union\\s*select.*from|insert\\s+into|delete\\s+from|drop\\s+table))" \
    "id:1003,phase:2,deny,status:403,msg:'SQL Injection Detected',logdata:'Matched Data: %{MATCHED_VAR} found within %{MATCHED_VAR_NAME}'"
```

### 2.4 规则集加载配置

```nginx
# 在 nginx.conf 中加载 CRS
modsecurity_rules_file /etc/nginx/modsecurity/crs-setup.conf;
modsecurity_rules '
    # 加载 CRS 规则
    Include /usr/share/modsecurity-crs/crs-setup.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-901-INITIALIZATION.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-903.9001-DRUPAL-EXCLUSION-RULES.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-903.9002-WORDPRESS-EXCLUSION-RULES.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-905-COMMON-EXCEPTIONS.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-910-IP-REPUTATION.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-911-METHOD-ENFORCEMENT.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-912-DOS-PROTECTION.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-913-SCANNER-DETECTION.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-920-PROTOCOL-ENFORCEMENT.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-921-PROTOCOL-ATTACK.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-930-APPLICATION-ATTACK-LFI.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-931-APPLICATION-ATTACK-RFI.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-932-APPLICATION-ATTACK-RCE.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-933-APPLICATION-ATTACK-PHP.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-934-APPLICATION-ATTACK-GENERIC.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-941-APPLICATION-ATTACK-XSS.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-942-APPLICATION-ATTACK-SQLI.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-943-APPLICATION-ATTACK-SESSION-FIXATION.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-944-APPLICATION-ATTACK-JAVA.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-949-BLOCKING-EVALUATION.conf
    Include /usr/share/modsecurity-crs/rules/RESPONSE-950-DATA-LEAKAGES.conf
    Include /usr/share/modsecurity-crs/rules/RESPONSE-951-DATA-LEAKAGES-SQL.conf
    Include /usr/share/modsecurity-crs/rules/RESPONSE-952-DATA-LEAKAGES-JAVA.conf
    Include /usr/share/modsecurity-crs/rules/RESPONSE-953-DATA-LEAKAGES-PHP.conf
    Include /usr/share/modsecurity-crs/rules/RESPONSE-954-DATA-LEAKAGES-IIS.conf
    Include /usr/share/modsecurity-crs/rules/RESPONSE-959-BLOCKING-EVALUATION.conf
    Include /usr/share/modsecurity-crs/rules/RESPONSE-980-CORRELATION.conf
    Include /usr/share/modsecurity-crs/rules/REQUEST-999-EXCLUSION-RULES-AFTER-CRS.conf
';
```

### 2.5 虚拟补丁配置

```ini
# /etc/nginx/modsecurity/virtual-patches.conf

# CVE-2021-44228 (Log4j)
SecRule REQUEST_HEADERS|REQUEST_HEADERS_NAMES|REQUEST_BODY|ARGS|ARGS_NAMES \
    "@rx \\$\\{.*jndi:(ldap|ldaps|dns|rmi|iiop|http|https|corba|nis|nds)" \
    "id:2001,phase:2,deny,status:403,msg:'CVE-2021-44228 Log4j RCE attempt',logdata:'Matched Data: %{MATCHED_VAR} found within %{MATCHED_VAR_NAME}'"

# CVE-2017-9805 (Struts REST XStream)
SecRule REQUEST_HEADERS:Content-Type "@contains application/x-www-form-urlencoded" \
    "id:2002,phase:1,deny,status:403,msg:'CVE-2017-9805 Struts REST XStream',chain"
    SecRule REQUEST_BODY "@contains <java." \
        "id:2002-1,phase:2,deny,status:403"

# CVE-2018-11776 (Apache Struts RCE)
SecRule REQUEST_URI "@rx /?action:.*\\$\\{.*\\}" \
    "id:2003,phase:1,deny,status:403,msg:'CVE-2018-11776 Struts RCE'"

# CVE-2019-19781 (Citrix ADC/NetScaler RCE)
SecRule REQUEST_URI "@contains /vpns/" \
    "id:2004,phase:1,deny,status:403,msg:'CVE-2019-19781 Citrix RCE',chain"
    SecRule REQUEST_URI "@rx /\\.\\./" \
        "id:2004-1,phase:1,deny,status:403"

# CVE-2021-41773/42013 (Apache Path Traversal)
SecRule REQUEST_URI "@rx \\%2e{2,}[/\\\\]" \
    "id:2005,phase:1,deny,status:403,msg:'CVE-2021-41773 Apache Path Traversal'"
```

### 2.6 ModSecurity 日志配置

```ini
# 审计日志
SecAuditEngine RelevantOnly
SecAuditLogRelevantStatus "^(?:5|4(?!04))"
SecAuditLogParts ABIJDEFHZ
SecAuditLogType Serial
SecAuditLog /var/log/modsec/audit.log

# 调试日志（仅测试时使用）
# SecDebugLog /var/log/modsec/debug.log
# SecDebugLogLevel 9

# 自定义日志格式
SecAuditLogFormat JSON
```

---

## 3. DDoS 防护策略

### 3.1 多层限流架构

```nginx
http {
    # Layer 1: 全局连接限制
    limit_conn_zone $binary_remote_addr zone=global_conn:10m;
    limit_conn_zone $server_name zone=server_conn:10m;

    # Layer 2: 请求速率限制
    limit_req_zone $binary_remote_addr zone=ip_req:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=api_req:10m rate=5r/s;
    limit_req_zone $binary_remote_addr zone=login_req:10m rate=1r/m;
    limit_req_zone $server_name zone=server_req:100m rate=1000r/s;

    # Layer 3: 带宽限制
    limit_rate_after 1m;
    limit_rate 100k;

    server {
        listen 80;

        # 全局连接限制
        limit_conn global_conn 20;
        limit_conn server_conn 1000;
        limit_conn_log_level warn;
        limit_conn_status 503;

        location / {
            # 一般请求限流
            limit_req zone=ip_req burst=20 nodelay;
            limit_req zone=server_req burst=100;
            limit_req_log_level warn;
            limit_req_status 429;

            proxy_pass http://backend;
        }

        location /api/ {
            # API 更严格限流
            limit_req zone=api_req burst=10 nodelay;
            limit_req zone=server_req burst=500;

            proxy_pass http://backend;
        }

        location /auth/ {
            # 登录接口最严格
            limit_req zone=login_req burst=3 nodelay;
            limit_req_status 429;

            proxy_pass http://backend;
        }

        location /download/ {
            # 下载限速
            limit_rate 500k;
            limit_rate_after 5m;

            proxy_pass http://backend;
        }
    }
}
```

### 3.2 基于地理位置的防护

```nginx
http {
    # GeoIP 模块（需 ngx_http_geoip_module 或 ngx_http_geoip2_module）
    geoip_country /usr/share/GeoIP/GeoIP.dat;
    geoip_city /usr/share/GeoIP/GeoLiteCity.dat;

    map $geoip_country_code $allowed_country {
        default yes;
        CN yes;
        US yes;
        JP yes;
        KR yes;
        GB yes;
        DE yes;
        FR yes;
        # 屏蔽高风险地区
        RU no;
        KP no;
        IR no;
        SY no;
        CU no;
    }

    map $geoip_country_code $country_limit {
        default "ip_req";
        CN "ip_req_cn";
        US "ip_req_us";
        RU "ip_req_block";
    }

    # 针对不同地区的限流区域
    limit_req_zone $binary_remote_addr zone=ip_req:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=ip_req_cn:50m rate=50r/s;
    limit_req_zone $binary_remote_addr zone=ip_req_us:30m rate=30r/s;
    limit_req_zone $binary_remote_addr zone=ip_req_block:1m rate=0r/s;

    server {
        # 地区阻断
        if ($allowed_country = no) {
            return 403 "Access denied from your region";
        }

        location / {
            # 应用地区特定限流
            limit_req zone=$country_limit burst=20 nodelay;
            proxy_pass http://backend;
        }
    }
}
```

### 3.3 SYN Flood 防护（系统级别）

```bash
# /etc/sysctl.conf - 内核参数调优

# SYN Flood 防护
net.ipv4.tcp_syncookies = 1
net.ipv4.tcp_max_syn_backlog = 2048
net.ipv4.tcp_synack_retries = 2
net.ipv4.tcp_syn_retries = 5

# IP Spoofing 防护
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1

# ICMP 广播防护
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.icmp_ignore_bogus_error_responses = 1

# 连接跟踪
net.netfilter.nf_conntrack_max = 1000000
net.netfilter.nf_conntrack_tcp_timeout_established = 600

# 应用配置
sysctl -p
```

### 3.4 IP 黑名单/白名单管理

```nginx
http {
    # 已知恶意 IP 列表
    geo $bad_ip {
        default 0;
        include /etc/nginx/conf.d/bad_ips.conf;
    }

    # 白名单 IP
    geo $whitelist_ip {
        default 0;
        192.168.1.0/24 1;
        10.0.0.0/8 1;
        172.16.0.0/12 1;
        include /etc/nginx/conf.d/whitelist.conf;
    }

    # 实时黑名单（配合 fail2ban）
    geo $blocked_by_fail2ban {
        default 0;
        include /var/lib/nginx/fail2ban-ip.list;
    }

    map $blocked_by_fail2ban $fail2ban_action {
        0 "pass";
        1 "block";
    }

    server {
        listen 80;

        # fail2ban 阻断
        if ($blocked_by_fail2ban) {
            return 444;
        }

        # 已知恶意 IP 阻断
        if ($bad_ip) {
            return 403;
        }

        location / {
            # 白名单 IP 不限流
            if ($whitelist_ip) {
                set $limit_key "";
            }

            proxy_pass http://backend;
        }
    }
}
```

### 3.5 慢速攻击防护

```nginx
http {
    # 慢速攻击防护配置
    client_body_timeout 10s;
    client_header_timeout 10s;
    send_timeout 10s;
    client_body_buffer_size 16k;
    client_header_buffer_size 1k;
    large_client_header_buffers 4 8k;

    # 零字节攻击防护
    client_max_body_size 10m;

    server {
        listen 80;

        # 慢速loris防护（使用 limit_conn）
        limit_conn_zone $binary_remote_addr zone=addr:10m;

        location / {
            # 限制单 IP 连接数
            limit_conn addr 10;

            # 代理超时设置
            proxy_connect_timeout 5s;
            proxy_send_timeout 10s;
            proxy_read_timeout 10s;

            proxy_pass http://backend;
        }
    }
}
```

---

## 4. OWASP Top 10 防护

### 4.1 A01:2021 - 访问控制失效

```nginx
# 路径遍历防护
location ~* \\.(git|svn|env|config|log|sql|backup|bak|swp|old)$ {
    deny all;
    return 404;
}

# 隐藏文件访问防护
location ~ /\\. {
    deny all;
    return 404;
}

# 敏感目录防护
location ~ ^/(api/|admin/|manage/|console/|config/) {
    # IP 白名单
    allow 192.168.1.0/24;
    allow 10.0.0.0/8;
    deny all;

    # 额外认证
    auth_basic "Restricted Area";
    auth_basic_user_file /etc/nginx/.htpasswd;

    proxy_pass http://backend;
}

# 方法限制
if ($request_method !~ ^(GET|HEAD|POST|PUT|DELETE|PATCH)$) {
    return 405;
}
```

### 4.2 A03:2021 - 注入攻击防护

```nginx
# SQL 注入基础防护
if ($request_uri ~* "(union.*select|select.*from|insert.*into|delete.*from|drop.*table|exec\\(|eval\\(|system\\()") {
    return 403;
}

if ($query_string ~* "(union|select|insert|delete|drop|truncate|update|set|create|alter|exec|script|eval)") {
    return 403;
}

# 命令注入防护
if ($query_string ~* "(\\;|\\||\\`|\\$\\(|\\$\\{|&&|\\|\\|)") {
    return 403;
}

# LDAP 注入防护
if ($request_uri ~* "[\\*\\(\\)\\&\\|]") {
    return 403;
}

# XML/XXE 注入防护
if ($request_body ~* "(<!ENTITY.*SYSTEM|file://|php://|expect://|data://)") {
    return 403;
}

# 在 server/location 中配置
location /api/ {
    # 内容类型验证
    if ($content_type !~ ^(application/json|application/x-www-form-urlencoded|multipart/form-data)$) {
        return 415;
    }

    # 参数长度限制
    if ($request_uri ~ "^.{1000,}$") {
        return 414;
    }

    proxy_pass http://backend;
}
```

### 4.3 A04:2021 - 不安全设计

```nginx
# CORS 安全配置
map $http_origin $cors_allow_origin {
    default "";
    "https://app.example.com" $http_origin;
    "https://admin.example.com" $http_origin;
}

server {
    location /api/ {
        # CORS 预检
        if ($request_method = OPTIONS) {
            add_header Access-Control-Allow-Origin $cors_allow_origin always;
            add_header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, OPTIONS" always;
            add_header Access-Control-Allow-Headers "Authorization, Content-Type, X-Requested-With" always;
            add_header Access-Control-Allow-Credentials "true" always;
            add_header Access-Control-Max-Age 86400 always;
            return 204;
        }

        add_header Access-Control-Allow-Origin $cors_allow_origin always;
        add_header Access-Control-Allow-Credentials "true" always;

        # CSRF Token 验证（可选）
        if ($http_x_csrf_token = "") {
            return 403 "CSRF token required";
        }

        proxy_pass http://backend;
    }
}
```

### 4.4 A07:2021 - 身份认证失效

```nginx
# 会话安全
add_header Set-Cookie "Path=/; HttpOnly; Secure; SameSite=Strict" always;

# JWT 令牌传输安全
location /api/ {
    # 禁止不安全的令牌传输
    if ($http_authorization ~* "Bearer.*[\\s\\\\t]") {
        return 400 "Invalid token format";
    }

    # 强制 HTTPS 传输令牌
    if ($scheme = http) {
        return 301 https://$host$request_uri;
    }

    proxy_pass http://backend;
}

# 登录接口强化
location /api/login {
    # 严格的限流
    limit_req zone=login_req burst=3 nodelay;

    # 方法限制
    if ($request_method != POST) {
        return 405;
    }

    # 内容类型限制
    if ($content_type != "application/json") {
        return 415;
    }

    proxy_pass http://backend;
}

# 密码重置保护
location /api/reset-password {
    limit_req zone=reset_req burst=2 nodelay;
    proxy_pass http://backend;
}
```

### 4.5 A08:2021 - 软件和数据完整性故障

```nginx
# 反序列化防护
location /api/ {
    # 阻止 Java 序列化对象
    if ($content_type = "application/x-java-serialized-object") {
        return 403;
    }

    # 阻止 .NET 序列化
    if ($content_type = "application/octet-stream") {
        if ($request_uri ~* \\.aspx|\.asmx|\.rem|\.soap) {
            return 403;
        }
    }

    # 请求体大小限制（防止大型序列化攻击）
    client_max_body_size 1m;

    proxy_pass http://backend;
}

# 子资源完整性检查
add_header X-Content-Type-Options "nosniff" always;
```

---

## 5. 安全响应头完整配置

### 5.1 完整安全响应头模板

```nginx
# 安全响应头配置文件 /etc/nginx/conf.d/security-headers.conf

# 基础安全头
add_header X-Frame-Options "SAMEORIGIN" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-XSS-Protection "1; mode=block" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Permissions-Policy "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), midi=(), sync-xhr=(), accelerometer=(), ambient-light-sensor=(), autoplay=(), encrypted-media=(), fullscreen=(), picture-in-picture=(), screen-wake-lock=(), web-share=()" always;

# HSTS（仅 HTTPS）
add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

# CSP（内容安全策略）- 基础版
add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' *.trusted-cdn.com; style-src 'self' 'unsafe-inline' *.trusted-cdn.com; img-src 'self' data: blob: *.trusted-cdn.com; font-src 'self' *.trusted-cdn.com; connect-src 'self' *.api.example.com; media-src 'self'; object-src 'none'; frame-src 'self'; frame-ancestors 'self'; form-action 'self'; base-uri 'self'; upgrade-insecure-requests;" always;

# 高级 CSP（报告模式）
add_header Content-Security-Policy-Report-Only "default-src 'self'; script-src 'self'; report-uri /csp-report;" always;

# 其他安全头
add_header Cross-Origin-Embedder-Policy "require-corp" always;
add_header Cross-Origin-Opener-Policy "same-origin" always;
add_header Cross-Origin-Resource-Policy "same-origin" always;
```

### 5.2 按路径定制 CSP

```nginx
server {
    listen 443 ssl;
    server_name example.com;

    # 默认严格 CSP
    add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self';" always;

    # 管理后台放宽策略（需要内联脚本）
    location /admin/ {
        add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:;" always;
        proxy_pass http://admin_backend;
    }

    # API 端点
    location /api/ {
        # API 不需要 CSP
        add_header Content-Security-Policy "" always;
        add_header X-Content-Type-Options "nosniff" always;
        proxy_pass http://api_backend;
    }

    # 静态资源
    location ~* \\.(css|js|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        add_header Cache-Control "public, max-age=31536000, immutable" always;
        # 静态资源不需要所有安全头
        expires 1y;
    }
}
```

### 5.3 CSP 策略参考表

| 指令 | 说明 | 示例值 |
|------|------|--------|
| `default-src` | 默认加载策略 | `'self'` |
| `script-src` | JavaScript 来源 | `'self' 'unsafe-inline' *.cdn.com` |
| `style-src` | CSS 来源 | `'self' 'unsafe-inline'` |
| `img-src` | 图片来源 | `'self' data: blob: https:` |
| `font-src` | 字体来源 | `'self' fonts.gstatic.com` |
| `connect-src` | XHR/WebSocket | `'self' wss://api.example.com` |
| `frame-src` | iframe 来源 | `'self' https://trusted.com` |
| `frame-ancestors` | 谁可以嵌入 | `'none'` / `'self'` |
| `form-action` | 表单提交目标 | `'self'` |
| `base-uri` | base 标签限制 | `'self'` |
| `object-src` | 插件内容 | `'none'` |

### 5.4 安全响应头测试

```bash
# 检查响应头
curl -I https://example.com

# 完整安全头检查
curl -s -D- -o /dev/null https://example.com | grep -E "^((Strict-Transport-Security|Content-Security-Policy|X-Frame-Options|X-Content-Type-Options|Referrer-Policy|Permissions-Policy|X-XSS-Protection|Cross-Origin-Embedder-Policy|Cross-Origin-Opener-Policy|Cross-Origin-Resource-Policy):)"

# 使用安全测试网站
# https://securityheaders.com/
# https://observatory.mozilla.org/
```

---

## 6. CVE 历史漏洞与修复版本

### 6.1 高危 CVE 漏洞表

| CVE ID | 影响版本 | 严重程度 | 漏洞描述 | 修复版本 |
|--------|----------|----------|----------|----------|
| CVE-2024-24990 | 1.25.4 | High | HTTP/3 队头阻塞 | 1.25.4+ |
| CVE-2024-24989 | 1.25.4 | Medium | HTTP/3 拒绝服务 | 1.25.4+ |
| CVE-2023-44487 | 1.25.0-1.25.2 | High | HTTP/2 快速重置攻击 | 1.25.3+ |
| CVE-2023-4276 | 1.25.0-1.25.1 | Medium | HTTP/2 内存泄漏 | 1.25.2+ |
| CVE-2022-41742 | 1.23.0-1.23.1 | High | mp4 模块内存越界 | 1.23.2+ |
| CVE-2022-41741 | 1.23.0-1.23.1 | High | mp4 模块内存越界 | 1.23.2+ |
| CVE-2021-23017 | 0.6.18-1.20.0 | High | DNS 解析器缓冲区溢出 | 1.20.1+ |
| CVE-2019-9511 | 1.9.5-1.17.2 | High | HTTP/2 空字节流拒绝服务 | 1.17.3+ |
| CVE-2019-9513 | 1.9.5-1.17.2 | High | HTTP/2 优先队列资源耗尽 | 1.17.3+ |
| CVE-2019-9516 | 1.9.5-1.17.2 | High | HTTP/2 零长度头部攻击 | 1.17.3+ |
| CVE-2018-16843 | 1.1.3-1.15.5 | Medium | HTTP/2 内存消耗 | 1.15.6+ |
| CVE-2018-16844 | 1.1.3-1.15.5 | Medium | HTTP/2 大流ID导致CPU耗尽 | 1.15.6+ |
| CVE-2017-7529 | 1.5.6-1.13.2 | Medium | 整数溢出导致信息泄露 | 1.13.3+ |
| CVE-2016-1247 | 1.6.0-1.10.1 | Low | 日志目录权限问题 | 1.10.2+ |
| CVE-2016-0742 | 1.9.0-1.9.9 | Medium | 拒绝服务漏洞 | 1.9.10+ |
| CVE-2014-0133 | 1.5.6-1.5.10 | High | 工作进程内存损坏 | 1.5.11+ |
| CVE-2013-4547 | 0.8.41-1.5.6 | High | 安全限制绕过 | 1.5.7+ |

### 6.2 版本安全建议

```bash
# 检查当前版本
nginx -v

# 查看编译模块和配置参数
nginx -V

# 建议使用的安全版本
# 主线版本: 1.25.x（最新功能）
# 稳定版本: 1.24.x（推荐生产环境）

# 自动更新检查脚本
#!/bin/bash
CURRENT=$(nginx -v 2>&1 | grep -oP 'nginx/\\K[0-9.]+')
LATEST=$(curl -s https://nginx.org/en/download.html | grep -oP 'Stable version.*?nginx-[0-9.]+' | grep -oP '[0-9.]+$' | head -1)

if [ "$CURRENT" != "$LATEST" ]; then
    echo "警告: NGINX 版本 $CURRENT 需要更新到 $LATEST"
    # 发送通知...
fi
```

### 6.3 安全更新流程

```bash
# 1. 备份当前配置
cp /etc/nginx/nginx.conf /etc/nginx/nginx.conf.backup.$(date +%Y%m%d)
tar -czf /backup/nginx-config-$(date +%Y%m%d).tar.gz /etc/nginx/

# 2. 测试新配置
nginx -t

# 3. 平滑升级（热升级）
# 下载并编译新版本
make upgrade

# 或使用包管理器
apt-get update && apt-get install --only-upgrade nginx

# 4. 验证升级
nginx -v
systemctl status nginx

# 5. 回滚准备（保留旧二进制）
mv /usr/sbin/nginx /usr/sbin/nginx.old
```

---

## 7. 安全审计日志配置

### 7.1 安全审计日志格式

```nginx
# /etc/nginx/conf.d/audit-log.conf

# 安全审计专用日志格式
log_format security '$remote_addr - $remote_user [$time_local] '
                   '"$request" $status $body_bytes_sent '
                   '"$http_referer" "$http_user_agent" '
                   'rt=$request_time uct="$upstream_connect_time" '
                   'uht="$upstream_header_time" urt="$upstream_response_time" '
                   'ssl_protocol=$ssl_protocol ssl_cipher=$ssl_cipher '
                   'req_id=$request_id '
                   'xff=$http_x_forwarded_for '
                   'real_ip=$http_x_real_ip '
                   'cc=$geoip_country_code '
                   'city=$geoip_city ';

# 攻击检测日志格式
log_format attack '$time_iso8601|$remote_addr|$request_method|$request_uri|'
                  '$status|$http_user_agent|$http_referer|'
                  '$request_time|$upstream_response_time|'
                  '$http_x_forwarded_for|$geoip_country_code|'
                  '$connection_requests|$msec';

# 安全事件日志
log_format security_event 'time=$time_iso8601 '
                          'client=$remote_addr '
                          'method=$request_method '
                          'uri="$request_uri" '
                          'status=$status '
                          'request_length=$request_length '
                          'body_bytes_sent=$body_bytes_sent '
                          'referer="$http_referer" '
                          'user_agent="$http_user_agent" '
                          'cookie=$http_cookie '
                          'authorization=$http_authorization '
                          'content_type=$content_type '
                          'content_length=$content_length '
                          'host=$host '
                          'server_name=$server_name '
                          'request_id=$request_id '
                          'ssl_session_id=$ssl_session_id '
                          'ssl_session_reused=$ssl_session_reused';
```

### 7.2 条件日志记录

```nginx
http {
    # 仅记录特定状态码
    map $status $loggable {
        ~^[23] 0;    # 2xx/3xx 不记录
        default 1;    # 其他记录
    }

    # 仅记录慢请求
    map $request_time $slow_request {
        ~^[0-2]\\. 0;    # < 3s 不记录
        default 1;         # >= 3s 记录
    }

    # 合并条件
    map "${loggable}${slow_request}" $security_log {
        "00" 0;
        default 1;
    }

    server {
        access_log /var/log/nginx/security.log security if=$security_log;

        location / {
            proxy_pass http://backend;
        }
    }
}
```

### 7.3 安全事件过滤

```nginx
# 记录特定安全事件
map "$status:$request_method:$http_user_agent" $security_event {
    default 0;

    # 记录所有 401/403
    "~^401" 1;
    "~^403" 1;
    "~^404" 1;

    # 记录攻击特征
    "~*sqlmap" 1;
    "~*nikto" 1;
    "~*nmap" 1;

    # 记录 POST 到敏感路径
    "~*:POST:.*/(admin|config|setup|install|phpmyadmin)" 1;
}

server {
    access_log /var/log/nginx/security-events.log attack if=$security_event;

    # 错误日志分离
    error_log /var/log/nginx/security-error.log warn;
}
```

### 7.4 日志轮转与保留

```bash
# /etc/logrotate.d/nginx-security
/var/log/nginx/security*.log {
    daily
    missingok
    rotate 90
    compress
    delaycompress
    notifempty
    create 0640 www-data adm
    sharedscripts
    postrotate
        if [ -f /var/run/nginx.pid ]; then
            kill -USR1 $(cat /var/run/nginx.pid)
        fi
    endscript
}

# 攻击日志长期保留
/var/log/nginx/attack*.log {
    weekly
    missingok
    rotate 520    # 保留 10 年
    compress
    delaycompress
    create 0640 www-data adm
    sharedscripts
    postrotate
        kill -USR1 $(cat /var/run/nginx.pid)
    endscript
}
```

### 7.5 实时安全监控脚本

```bash
#!/bin/bash
# /usr/local/bin/nginx-security-monitor.sh

LOG_FILE="/var/log/nginx/security.log"
ALERT_THRESHOLD=100      # 每分钟请求数阈值
BLOCK_THRESHOLD=10       # 触发封禁的异常请求数

# 实时监控
tail -F $LOG_FILE | while read line; do
    # 提取 IP
    ip=$(echo "$line" | awk '{print $1}')

    # 检查攻击模式
    if echo "$line" | grep -qE "(union|select|script|alert|eval\\(|system\\()"; then
        echo "$(date): SQL/XSS 攻击检测: $ip - $line" >> /var/log/nginx/alerts.log
        # 可选：自动添加到黑名单
        # echo "deny $ip;" >> /etc/nginx/conf.d/auto-block.conf
    fi

    # 检查扫描行为
    if echo "$line" | grep -qE "(\\.env|\\.git|config\\.xml|phpmyadmin|wp-admin)"; then
        echo "$(date): 敏感路径扫描: $ip - $line" >> /var/log/nginx/alerts.log
    fi
done
```

---

## 8. 完整安全配置示例

### 8.1 生产环境安全配置

```nginx
# /etc/nginx/nginx.conf - 安全强化主配置

user nginx;
worker_processes auto;
worker_rlimit_nofile 65535;

error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

# 加载动态模块
load_module modules/ngx_http_geoip_module.so;
load_module modules/ngx_http_modsecurity_module.so;

events {
    worker_connections 4096;
    use epoll;
    multi_accept on;
}

http {
    # 基础设置
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # 隐藏版本号
    server_tokens off;

    # 字符编码
    charset utf-8;

    # 日志格式
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for" '
                    'rt=$request_time uct="$upstream_connect_time" '
                    'uht="$upstream_header_time" urt="$upstream_response_time"';

    log_format security '$remote_addr - $time_iso8601 - "$request" - $status - '
                        '"$http_user_agent" - "$http_referer" - '
                        'req_time=$request_time - xff="$http_x_forwarded_for"';

    access_log /var/log/nginx/access.log main;

    # 性能优化
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;

    # Gzip 压缩
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css text/xml application/json application/javascript application/rss+xml application/atom+xml image/svg+xml;

    # 客户端限制
    client_max_body_size 10m;
    client_body_buffer_size 128k;
    client_header_buffer_size 1k;
    large_client_header_buffers 4 8k;
    client_body_timeout 12s;
    client_header_timeout 12s;

    # 缓冲区设置
    proxy_buffer_size 4k;
    proxy_buffers 8 4k;
    proxy_busy_buffers_size 8k;

    # GeoIP
    geoip_country /usr/share/GeoIP/GeoIP.dat;
    geoip_city /usr/share/GeoIP/GeoLiteCity.dat;

    # 限流区域
    limit_req_zone $binary_remote_addr zone=req_limit:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=api_limit:10m rate=50r/s;
    limit_req_zone $binary_remote_addr zone=login_limit:10m rate=1r/m;
    limit_conn_zone $binary_remote_addr zone=conn_limit:10m;

    # Bot 检测
    map $http_user_agent $bot_type {
        default "human";
        ~*googlebot "good_bot";
        ~*bingbot "good_bot";
        ~*sqlmap "bad_bot";
        ~*nikto "bad_bot";
        ~*nmap "bad_bot";
        ~*curl "tool";
        ~*wget "tool";
    }

    # 国家代码映射
    map $geoip_country_code $allowed_country {
        default yes;
        CN yes;
        US yes;
        JP yes;
        RU no;
        KP no;
        IR no;
    }

    # 包含其他配置
    include /etc/nginx/conf.d/*.conf;
    include /etc/nginx/sites-enabled/*;
}
```

### 8.2 HTTPS 站点安全配置

```nginx
# /etc/nginx/sites-available/example.com

server {
    listen 80;
    server_name example.com www.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name example.com www.example.com;

    # SSL 证书
    ssl_certificate /etc/nginx/ssl/example.com.crt;
    ssl_certificate_key /etc/nginx/ssl/example.com.key;

    # SSL 配置
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_dhparam /etc/nginx/ssl/dhparam.pem;

    # SSL 会话
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    # OCSP Stapling
    ssl_stapling on;
    ssl_stapling_verify on;
    ssl_trusted_certificate /etc/nginx/ssl/example.com.chain.crt;
    resolver 8.8.8.8 8.8.4.4 valid=300s;
    resolver_timeout 5s;

    # 安全响应头
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
    add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' https:; connect-src 'self' https://api.example.com; frame-ancestors 'self'; form-action 'self'; base-uri 'self';" always;
    add_header Cross-Origin-Embedder-Policy "require-corp" always;
    add_header Cross-Origin-Opener-Policy "same-origin" always;
    add_header Cross-Origin-Resource-Policy "same-origin" always;

    # 根目录
    root /var/www/example.com;
    index index.html index.htm;

    # 日志
    access_log /var/log/nginx/example.com-access.log main;
    error_log /var/log/nginx/example.com-error.log warn;

    # 全局限制
    limit_conn conn_limit 20;
    limit_req zone=req_limit burst=20 nodelay;

    # 安全过滤
    if ($bad_ip) {
        return 444;
    }

    if ($allowed_country = no) {
        return 403;
    }

    # 敏感文件保护
    location ~ /\\. {
        deny all;
        return 404;
    }

    location ~* \\.(git|svn|env|config|log|sql|backup|bak|swp|old|orig|save)$ {
        deny all;
        return 404;
    }

    # 静态资源
    location ~* \\.(jpg|jpeg|png|gif|ico|css|js|woff|woff2|ttf|eot|svg)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
        add_header X-Frame-Options "SAMEORIGIN" always;
        access_log off;
    }

    # API 路由
    location /api/ {
        limit_req zone=api_limit burst=50 nodelay;

        # 内容类型验证
        if ($content_type !~ ^(application/json|application/x-www-form-urlencoded|multipart/form-data)$) {
            return 415;
        }

        # 方法限制
        if ($request_method !~ ^(GET|POST|PUT|DELETE|PATCH|OPTIONS)$) {
            return 405;
        }

        # CORS
        if ($request_method = OPTIONS) {
            add_header Access-Control-Allow-Origin "https://example.com" always;
            add_header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, PATCH, OPTIONS" always;
            add_header Access-Control-Allow-Headers "Authorization, Content-Type, X-Requested-With" always;
            add_header Access-Control-Allow-Credentials "true" always;
            add_header Access-Control-Max-Age 86400 always;
            return 204;
        }

        add_header Access-Control-Allow-Origin "https://example.com" always;
        add_header Access-Control-Allow-Credentials "true" always;

        proxy_pass http://api_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
    }

    # 认证路由
    location /auth/ {
        limit_req zone=login_limit burst=3 nodelay;

        proxy_pass http://auth_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    # 主应用
    location / {
        try_files $uri $uri/ /index.html;

        # 安全响应头（静态页面）
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
    }
}
```

### 8.3 WAF 完整配置

```nginx
# /etc/nginx/conf.d/waf.conf

# ModSecurity 全局配置
modsecurity on;
modsecurity_rules_file /etc/nginx/modsecurity/main.conf;

# 特定路径规则
server {
    location / {
        # 启用完整 CRS
        modsecurity_rules '
            SecRuleEngine On
            SecRequestBodyAccess On
            SecResponseBodyAccess On
            SecResponseBodyLimit 524288
            SecResponseBodyLimitAction ProcessPartial

            # 文件上传限制
            SecRequestBodyLimit 13107200
            SecRequestBodyNoFilesLimit 131072

            # 允许文件上传类型
            SecRule REQUEST_FILENAME "@rx \\.(jpg|jpeg|png|gif|pdf|doc|docx)$" \
                "id:2000,phase:1,pass,nolog,ctl:requestBodyLimit=52428800"

            # 加载 CRS
            Include /usr/share/modsecurity-crs/crs-setup.conf
            Include /usr/share/modsecurity-crs/rules/*.conf

            # 自定义排除
            SecRule REQUEST_URI "@beginsWith /api/webhook" \
                "id:9000,phase:1,pass,nolog,ctl:ruleRemoveById=920350"
        ';

        proxy_pass http://backend;
    }

    # 静态资源跳过 WAF
    location ~* \\.(jpg|jpeg|png|gif|css|js|woff|woff2|ttf|eot|svg|ico)$ {
        modsecurity off;
        expires 1y;
        access_log off;
        proxy_pass http://backend;
    }
}
```

### 8.4 安全配置检查清单

```markdown
## 部署前检查清单

### 基础安全
- [ ] 使用最新稳定版 NGINX
- [ ] server_tokens 设置为 off
- [ ] 禁用不安全的 SSL 协议（SSLv2/SSLv3/TLSv1.0/TLSv1.1）
- [ ] 配置安全的加密套件
- [ ] 启用 HSTS
- [ ] 配置所有安全响应头

### 访问控制
- [ ] 管理后台 IP 白名单
- [ ] 敏感文件/目录访问限制
- [ ] 地理位置访问控制（如需要）
- [ ] User-Agent 过滤

### 请求限制
- [ ] 请求速率限制（通用）
- [ ] API 专用限流
- [ ] 登录接口严格限流
- [ ] 连接数限制
- [ ] 带宽限制

### WAF 防护
- [ ] ModSecurity 安装配置
- [ ] CRS 规则集加载
- [ ] 虚拟补丁规则
- [ ] 自定义业务规则
- [ ] 排除规则测试

### 日志监控
- [ ] 安全审计日志配置
- [ ] 日志轮转设置
- [ ] 实时监控脚本
- [ ] fail2ban 集成

### 性能与安全平衡
- [ ] 静态资源缓存策略
- [ ] 安全头对静态资源的影响
- [ ] WAF 对性能的损耗评估
- [ ] 限流阈值合理性测试

### 测试验证
- [ ] SSL Labs A+ 评分
- [ ] Security Headers 检查
- [ ] OWASP ZAP 扫描
- [ ] 渗透测试
- [ ] 性能基准测试
```

---

## 附录：常用安全测试命令

```bash
# SSL/TLS 测试
openssl s_client -connect example.com:443 -tls1_2
openssl s_client -connect example.com:443 -tls1_3
testssl.sh example.com

# 安全响应头检查
curl -I -s https://example.com | grep -E "^(Strict-Transport-Security|Content-Security-Policy|X-Frame-Options)"

# 漏洞扫描基础
curl -s https://example.com/.env && echo "DANGER: .env exposed!"
curl -s https://example.com/.git/HEAD && echo "DANGER: .git exposed!"
curl -s https://example.com/config.xml && echo "DANGER: config exposed!"

# 压力测试（限流验证）
ab -n 1000 -c 100 https://example.com/api/test
siege -c 50 -t 30s https://example.com/
```
