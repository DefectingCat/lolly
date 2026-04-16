# NGINX OpenID Connect 模块指南

## 1. OIDC 模块概述

### 什么是 OpenID Connect

OpenID Connect (OIDC) 是基于 OAuth 2.0 协议的身份认证层，允许客户端应用程序验证用户身份并获取用户基本信息。NGINX Plus 提供原生 OIDC 支持，可作为 **Relying Party (RP)** 与 Identity Provider (IdP) 集成，实现单点登录 (SSO) 和集中式身份认证。

### 模块用途

| 场景 | 说明 |
|------|------|
| **单点登录 (SSO)** | 用户一次登录，访问多个受保护应用 |
| **集中式认证** | 统一认证入口，后端服务无需处理认证逻辑 |
| **API 保护** | 验证 JWT Token，保护 API 端点 |
| **身份代理** | 将身份信息传递给后端应用 |
| **会话管理** | 集中管理用户会话生命周期 |

### 架构图

```
┌─────────┐         ┌─────────────┐         ┌─────────────────┐
│  Client │────────▶│  NGINX Plus │────────▶│   IdP Provider  │
│ (浏览器) │         │   (OIDC RP)  │         │ (Keycloak/Okta) │
└─────────┘         └──────┬──────┘         └─────────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   Backend    │
                    │ Applications │
                    └──────────────┘
```

**工作流程**：
1. 用户访问受保护资源，NGINX 检查会话状态
2. 无有效会话时，重定向到 IdP 登录页面
3. 用户在 IdP 完成认证，IdP 返回授权码
4. NGINX 使用授权码交换 ID Token 和 Access Token
5. 用户携带会话 Cookie 访问资源，NGINX 验证 JWT
6. 后端应用接收带有用户信息的请求

---

## 2. OIDC 认证流程

### 2.1 授权码流程 (Authorization Code Flow)

NGINX Plus 使用标准的 OAuth 2.0 授权码流程，支持 **PKCE** (Proof Key for Code Exchange) 增强安全性。

**完整流程图**：

```
用户                          NGINX Plus                    IdP Provider
 |                                │                             │
 |── 1. 访问 /app ───────────────▶│                             │
 |                                │── 2. 检查会话 ─────────────▶│
 |                                │                             │
 |◀─ 3. 重定向到 /authorize ─────│                             │
 |                                │                             │
 |── 4. 登录认证 ───────────────────────────────▶│              │
 |                                │                             │
 |◀─ 5. 返回授权码 ───────────────│                             │
 |                                │                             │
 |── 6. 携带 code 回调 ──────────▶│                             │
 |                                │── 7. Token 请求 ───────────▶│
 |                                │                             │
 |                                │◀─ 8. ID/Access/Refresh Token─│
 |                                │                             │
 |◀─ 9. 设置 Session Cookie ─────│                             │
 |                                │                             │
 |── 10. 携带 Cookie 访问 ───────▶│                             │
 |                                │── 11. 验证 JWT ────────────▶│
 |                                │                             │
 |                                │── 12. 代理到后端 ──────────▶│
```

### 2.2 支持的 IdP 提供商

| 提供商 | Discovery URL 格式 |
|--------|-------------------|
| **Keycloak** | `https://keycloak.example.com/realms/{realm}/.well-known/openid-configuration` |
| **Okta** | `https://{your-domain}.okta.com/.well-known/openid-configuration` |
| **Auth0** | `https://{your-domain}.auth0.com/.well-known/openid-configuration` |
| **Azure AD / Entra ID** | `https://login.microsoftonline.com/{tenant}/v2.0/.well-known/openid-configuration` |
| **Google** | `https://accounts.google.com/.well-known/openid-configuration` |
| **GitHub** | `https://token.actions.githubusercontent.com/.well-known/openid-configuration` |
| **自建 IdP** | `https://auth.example.com/.well-known/openid-configuration` |

---

## 3. 指令参考

### 3.1 核心指令

#### `auth_jwt`

启用 JWT 认证。

| 属性 | 说明 |
|------|------|
| **语法** | `auth_jwt "realm" [token=$variable] \| off;` |
| **默认值** | `off` |
| **上下文** | `http`, `server`, `location` |

```nginx
# 基本用法
location /protected/ {
    auth_jwt "API Access";
    auth_jwt_key_file /etc/nginx/jwks.json;
}

# 从 Cookie 读取 Token
location /api/ {
    auth_jwt "API Access" token=$cookie_auth_token;
    auth_jwt_key_request /_jwks_uri;
}

# 从 Header 读取 Token
location /api/ {
    auth_jwt "API Access" token=$http_authorization;
    auth_jwt_key_request /_jwks_uri;
}
```

#### `auth_jwt_key_file`

指定 JWKS (JSON Web Key Set) 文件路径，用于验证 JWT 签名。

| 属性 | 说明 |
|------|------|
| **语法** | `auth_jwt_key_file file;` |
| **默认值** | — |
| **上下文** | `http`, `server`, `location` |

```nginx
# 本地 JWKS 文件
auth_jwt_key_file /etc/nginx/oidc/jwks.json;

# 使用变量（动态配置）
auth_jwt_key_file $oidc_jwt_keyfile;
```

#### `auth_jwt_key_request`

从指定 location 动态获取 JWKS。

| 属性 | 说明 |
|------|------|
| **语法** | `auth_jwt_key_request uri;` |
| **默认值** | — |
| **上下文** | `http`, `server`, `location` |

```nginx
server {
    location /protected/ {
        auth_jwt "API Access" token=$cookie_auth_token;
        auth_jwt_key_request /_jwks_uri;
    }

    # 内部 JWKS 端点
    location = /_jwks_uri {
        internal;                    # 仅限内部子请求访问
        proxy_pass https://idp.example.com/.well-known/jwks.json;
        proxy_cache jwks_cache;
        proxy_cache_valid 200 1h;
    }
}
```

#### `auth_jwt_require`

配置 JWT 验证的额外要求。

| 属性 | 说明 |
|------|------|
| **语法** | `auth_jwt_require $claim [value] ...;` |
| **默认值** | — |
| **上下文** | `http`, `server`, `location` |

```nginx
# 要求特定的 issuer
location /protected/ {
    auth_jwt "API Access" token=$cookie_auth_token;
    auth_jwt_key_request /_jwks_uri;
    auth_jwt_require $jwt_claim_iss "https://idp.example.com";
}

# 要求特定的 audience
location /api/admin/ {
    auth_jwt "Admin Access" token=$cookie_auth_token;
    auth_jwt_key_request /_jwks_uri;
    auth_jwt_require $jwt_claim_aud "my-api-client";
}

# 组合条件
location /api/sensitive/ {
    auth_jwt "Sensitive Access" token=$cookie_auth_token;
    auth_jwt_key_request /_jwks_uri;
    auth_jwt_require $jwt_claim_iss "https://idp.example.com" $jwt_claim_aud "sensitive-api";
}
```

### 3.2 会话管理指令

#### `keyval_zone`

定义用于存储 OIDC 会话数据的共享内存区域。

| 属性 | 说明 |
|------|------|
| **语法** | `keyval_zone zone=name:size [state=file] [timeout=time];` |
| **默认值** | — |
| **上下文** | `http` |

```nginx
# 基本配置
keyval_zone zone=oidc_id_tokens:1M timeout=1h;
keyval_zone zone=oidc_access_tokens:1M timeout=1h;
keyval_zone zone=oidc_refresh_tokens:1M timeout=8h;

# 带状态持久化
keyval_zone zone=oidc_id_tokens:1M state=/var/lib/nginx/state/oidc_id_tokens.json timeout=1h;
keyval_zone zone=oidc_access_tokens:1M state=/var/lib/nginx/state/oidc_access_tokens.json timeout=1h;
```

**参数说明**：
- `zone=name:size` - 共享内存区域名称和大小
- `state=file` - 状态持久化文件路径
- `timeout=time` - 数据过期时间

#### `keyval`

定义键值对存储。

| 属性 | 说明 |
|------|------|
| **语法** | `keyval $variable $variable zone=name;` |
| **默认值** | — |
| **上下文** | `http` |

```nginx
# 定义存储变量
keyval $cookie_auth_token $session_jwt zone=oidc_id_tokens;
keyval $cookie_auth_token $access_token zone=oidc_access_tokens;
keyval $cookie_auth_token $refresh_token zone=oidc_refresh_tokens;
```

### 3.3 JavaScript 模块指令

#### `js_import`

导入 NGINX JavaScript (njs) 模块。

| 属性 | 说明 |
|------|------|
| **语法** | `js_import module [as namespace];` |
| **默认值** | — |
| **上下文** | `http` |

```nginx
# 导入 OIDC 处理脚本
js_import /etc/nginx/conf.d/openid_connect.js;

# 使用命名空间
js_import /etc/nginx/conf.d/openid_connect.js as oidc;
```

#### `js_set`

设置 JavaScript 变量。

| 属性 | 说明 |
|------|------|
| **语法** | `js_set $variable function;` |
| **默认值** | — |
| **上下文** | `http` |

```nginx
# 设置 OIDC 认证头
js_set $oidc_auth_header oidc.authHeader;

# 设置 ID Token
js_set $id_token oidc.idToken;
```

#### `js_content`

使用 JavaScript 生成响应内容。

| 属性 | 说明 |
|------|------|
| **语法** | `js_content function;` |
| **默认值** | — |
| **上下文** | `location` |

```nginx
location /login {
    js_content oidc.login;
}

location /logout {
    js_content oidc.logout;
}

location = /redirect_uri {
    js_content oidc.redirect;
}
```

### 3.4 代理配置指令

#### `auth_jwt_set`

将 JWT Claims 提取到变量。

| 属性 | 说明 |
|------|------|
| **语法** | `auth_jwt_set $variable claim;` |
| **默认值** | — |
| **上下文** | `http`, `server`, `location` |

```nginx
server {
    location /api/ {
        auth_jwt "API Access" token=$cookie_auth_token;
        auth_jwt_key_request /_jwks_uri;

        # 提取常用 Claims
        auth_jwt_set $jwt_sub sub;
        auth_jwt_set $jwt_email email;
        auth_jwt_set $jwt_name name;

        proxy_pass http://backend;
        proxy_set_header X-User-ID $jwt_sub;
        proxy_set_header X-User-Email $jwt_email;
        proxy_set_header X-User-Name $jwt_name;
    }
}
```

---

## 4. 变量参考

### 4.1 JWT Claims 变量

| 变量 | 说明 |
|------|------|
| `$jwt_claim_sub` | 用户唯一标识 (Subject) |
| `$jwt_claim_iss` | Token 签发者 (Issuer) |
| `$jwt_claim_aud` | Token 受众 (Audience) |
| `$jwt_claim_exp` | Token 过期时间 (Expiration) |
| `$jwt_claim_iat` | Token 签发时间 (Issued At) |
| `$jwt_claim_nbf` | Token 生效时间 (Not Before) |
| `$jwt_claim_jti` | Token 唯一标识 (JWT ID) |
| `$jwt_claim_email` | 用户邮箱 |
| `$jwt_claim_name` | 用户名称 |
| `$jwt_claim_preferred_username` | 首选用户名 |
| `$jwt_claim_groups` | 用户组/角色 |
| `$jwt_claim_scope` | 授权范围 |

### 4.2 会话变量

| 变量 | 说明 |
|------|------|
| `$cookie_auth_token` | 会话 Cookie 值 |
| `$session_jwt` | 存储的 ID Token |
| `$access_token` | 存储的 Access Token |
| `$refresh_token` | 存储的 Refresh Token |

---

## 5. IdP 集成配置示例

### 5.1 Keycloak 集成

**Keycloak 配置要点**：
- 创建 Client，启用 `Standard Flow` (Authorization Code)
- 配置 Valid Redirect URIs: `https://app.example.com/redirect_uri`
- 启用 `Client authentication`，记录 Client Secret
- 配置 Web Origins: `https://app.example.com`

**NGINX 配置**：

```nginx
http {
    # 加载 JavaScript 模块
    load_module modules/ngx_http_js_module.so;

    # 会话存储
    keyval_zone zone=oidc_id_tokens:1M state=/var/lib/nginx/state/oidc_id_tokens.json timeout=1h;
    keyval_zone zone=oidc_access_tokens:1M state=/var/lib/nginx/state/oidc_access_tokens.json timeout=1h;
    keyval_zone zone=oidc_refresh_tokens:1M state=/var/lib/nginx/state/oidc_refresh_tokens.json timeout=8h;

    keyval $cookie_auth_token $session_jwt zone=oidc_id_tokens;
    keyval $cookie_auth_token $access_token zone=oidc_access_tokens;
    keyval $cookie_auth_token $refresh_token zone=oidc_refresh_tokens;

    # IdP 端点配置
    map $host $oidc_authz_endpoint {
        default "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/auth";
    }

    map $host $oidc_token_endpoint {
        default "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/token";
    }

    map $host $oidc_jwks_uri {
        default "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/certs";
    }

    map $host $oidc_userinfo_endpoint {
        default "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/userinfo";
    }

    map $host $oidc_end_session_endpoint {
        default "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/logout";
    }

    # Client 配置
    map $host $oidc_client {
        default "my-nginx-client";
    }

    map $host $oidc_client_secret {
        default "your-client-secret-here";
    }

    map $host $oidc_scopes {
        default "openid+profile+email";
    }

    map $host $oidc_pkce_enable {
        default "1";
    }

    # 导入 OIDC 脚本
    js_import /etc/nginx/conf.d/openid_connect.js;

    # 缓存配置
    proxy_cache_path /var/cache/nginx/jwks levels=1:2 keys_zone=jwks_cache:1m max_size=10m inactive=60m use_temp_path=off;

    server {
        listen 443 ssl;
        server_name app.example.com;

        ssl_certificate /etc/nginx/ssl/app.example.com.crt;
        ssl_certificate_key /etc/nginx/ssl/app.example.com.key;

        # 会话 Cookie 配置
        set $session_cookie "auth_token";
        set $session_cookie_flags "Path=/; Secure; HttpOnly; SameSite=Strict";

        location / {
            # JWT 认证
            auth_jwt "Keycloak SSO" token=$cookie_auth_token;
            auth_jwt_key_request /_jwks_uri;

            # 提取用户信息
            auth_jwt_set $jwt_sub sub;
            auth_jwt_set $jwt_email email;

            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_set_header X-User-ID $jwt_sub;
            proxy_set_header X-User-Email $jwt_email;
            proxy_set_header Authorization "Bearer $access_token";
        }

        # OIDC 端点
        location /login {
            js_content openid_connect.login;
        }

        location /logout {
            js_content openid_connect.logout;
        }

        location = /redirect_uri {
            js_content openid_connect.redirect;
        }

        # 内部 JWKS 端点
        location = /_jwks_uri {
            internal;
            proxy_pass $oidc_jwks_uri;
            proxy_cache jwks_cache;
            proxy_cache_valid 200 1h;
            proxy_cache_use_stale error timeout updating;
            proxy_ssl_server_name on;
        }
    }
}
```

### 5.2 Okta 集成

**Okta 配置要点**：
- 创建 App Integration，选择 `OIDC - OpenID Connect`
- Application type: `Web Application`
- Sign-in redirect URIs: `https://app.example.com/redirect_uri`
- Sign-out redirect URIs: `https://app.example.com/`
- Grant type: `Authorization Code` + `Refresh Token`

**NGINX 配置**：

```nginx
http {
    load_module modules/ngx_http_js_module.so;

    # 会话存储
    keyval_zone zone=oidc_id_tokens:1M state=/var/lib/nginx/state/oidc_id_tokens.json timeout=1h;
    keyval_zone zone=oidc_access_tokens:1M state=/var/lib/nginx/state/oidc_access_tokens.json timeout=1h;
    keyval_zone zone=oidc_refresh_tokens:1M state=/var/lib/nginx/state/oidc_refresh_tokens.json timeout=8h;

    keyval $cookie_auth_token $session_jwt zone=oidc_id_tokens;
    keyval $cookie_auth_token $access_token zone=oidc_access_tokens;
    keyval $cookie_auth_token $refresh_token zone=oidc_refresh_tokens;

    # Okta 端点配置
    map $host $oidc_authz_endpoint {
        default "https://myorg.okta.com/oauth2/default/v1/authorize";
        # 或使用自定义授权服务器
        # default "https://myorg.okta.com/oauth2/ausxxxxxx/v1/authorize";
    }

    map $host $oidc_token_endpoint {
        default "https://myorg.okta.com/oauth2/default/v1/token";
    }

    map $host $oidc_jwks_uri {
        default "https://myorg.okta.com/oauth2/default/v1/keys";
    }

    map $host $oidc_userinfo_endpoint {
        default "https://myorg.okta.com/oauth2/default/v1/userinfo";
    }

    map $host $oidc_end_session_endpoint {
        default "https://myorg.okta.com/oauth2/default/v1/logout";
    }

    map $host $oidc_client {
        default "0oaxxxxxxxxxxx";
    }

    map $host $oidc_client_secret {
        default "your-client-secret";
    }

    map $host $oidc_scopes {
        default "openid+profile+email+groups";
    }

    map $host $oidc_pkce_enable {
        default "1";
    }

    js_import /etc/nginx/conf.d/openid_connect.js;

    proxy_cache_path /var/cache/nginx/jwks levels=1:2 keys_zone=jwks_cache:1m max_size=10m inactive=60m;

    server {
        listen 443 ssl;
        server_name app.example.com;

        ssl_certificate /etc/nginx/ssl/app.example.com.crt;
        ssl_certificate_key /etc/nginx/ssl/app.example.com.key;

        location / {
            auth_jwt "Okta SSO" token=$cookie_auth_token;
            auth_jwt_key_request /_jwks_uri;

            auth_jwt_set $jwt_sub sub;
            auth_jwt_set $jwt_email email;
            auth_jwt_set $jwt_groups groups;

            proxy_pass http://backend;
            proxy_set_header X-User-ID $jwt_sub;
            proxy_set_header X-User-Email $jwt_email;
            proxy_set_header X-User-Groups $jwt_groups;
            proxy_set_header Authorization "Bearer $access_token";
        }

        location /login {
            js_content openid_connect.login;
        }

        location /logout {
            js_content openid_connect.logout;
        }

        location = /redirect_uri {
            js_content openid_connect.redirect;
        }

        location = /_jwks_uri {
            internal;
            proxy_pass $oidc_jwks_uri;
            proxy_cache jwks_cache;
            proxy_cache_valid 200 1h;
            proxy_ssl_server_name on;
        }
    }
}
```

### 5.3 Auth0 集成

**Auth0 配置要点**：
- 创建 Application，类型选择 `Regular Web Application`
- Allowed Callback URLs: `https://app.example.com/redirect_uri`
- Allowed Logout URLs: `https://app.example.com/`
- Allowed Web Origins: `https://app.example.com`
- 启用 `Refresh Token` 轮换（可选）

**NGINX 配置**：

```nginx
http {
    load_module modules/ngx_http_js_module.so;

    # 会话存储
    keyval_zone zone=oidc_id_tokens:1M state=/var/lib/nginx/state/oidc_id_tokens.json timeout=1h;
    keyval_zone zone=oidc_access_tokens:1M state=/var/lib/nginx/state/oidc_access_tokens.json timeout=1h;
    keyval_zone zone=oidc_refresh_tokens:1M state=/var/lib/nginx/state/oidc_refresh_tokens.json timeout=8h;

    keyval $cookie_auth_token $session_jwt zone=oidc_id_tokens;
    keyval $cookie_auth_token $access_token zone=oidc_access_tokens;
    keyval $cookie_auth_token $refresh_token zone=oidc_refresh_tokens;

    # Auth0 端点配置
    map $host $oidc_authz_endpoint {
        default "https://myapp.auth0.com/authorize";
    }

    map $host $oidc_token_endpoint {
        default "https://myapp.auth0.com/oauth/token";
    }

    map $host $oidc_jwks_uri {
        default "https://myapp.auth0.com/.well-known/jwks.json";
    }

    map $host $oidc_userinfo_endpoint {
        default "https://myapp.auth0.com/userinfo";
    }

    map $host $oidc_end_session_endpoint {
        default "https://myapp.auth0.com/v2/logout";
    }

    map $host $oidc_client {
        default "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx";
    }

    map $host $oidc_client_secret {
        default "your-client-secret";
    }

    map $host $oidc_scopes {
        default "openid+profile+email";
    }

    map $host $oidc_pkce_enable {
        default "1";
    }

    # Auth0 特定：返回首页参数
    map $host $oidc_logout_redirect {
        default "https://app.example.com/";
    }

    js_import /etc/nginx/conf.d/openid_connect.js;

    proxy_cache_path /var/cache/nginx/jwks levels=1:2 keys_zone=jwks_cache:1m max_size=10m inactive=60m;

    server {
        listen 443 ssl;
        server_name app.example.com;

        ssl_certificate /etc/nginx/ssl/app.example.com.crt;
        ssl_certificate_key /etc/nginx/ssl/app.example.com.key;

        location / {
            auth_jwt "Auth0 SSO" token=$cookie_auth_token;
            auth_jwt_key_request /_jwks_uri;

            auth_jwt_set $jwt_sub sub;
            auth_jwt_set $jwt_email email;
            auth_jwt_set $jwt_name name;
            auth_jwt_set $jwt_nickname nickname;

            proxy_pass http://backend;
            proxy_set_header X-User-ID $jwt_sub;
            proxy_set_header X-User-Email $jwt_email;
            proxy_set_header X-User-Name $jwt_name;
            proxy_set_header Authorization "Bearer $access_token";
        }

        location /login {
            js_content openid_connect.login;
        }

        location /logout {
            js_content openid_connect.logout;
        }

        location = /redirect_uri {
            js_content openid_connect.redirect;
        }

        location = /_jwks_uri {
            internal;
            proxy_pass $oidc_jwks_uri;
            proxy_cache jwks_cache;
            proxy_cache_valid 200 1h;
            proxy_ssl_server_name on;
        }
    }
}
```

### 5.4 Azure AD / Entra ID 集成

**Azure AD 配置要点**：
- 在 Azure Portal 注册应用
- 添加平台配置：Web
- 重定向 URI: `https://app.example.com/redirect_uri`
- 启用 `ID tokens` (用于隐式和混合流)
- 创建 Client Secret

**NGINX 配置**：

```nginx
http {
    load_module modules/ngx_http_js_module.so;

    # 会话存储
    keyval_zone zone=oidc_id_tokens:1M state=/var/lib/nginx/state/oidc_id_tokens.json timeout=1h;
    keyval_zone zone=oidc_access_tokens:1M state=/var/lib/nginx/state/oidc_access_tokens.json timeout=1h;
    keyval_zone zone=oidc_refresh_tokens:1M state=/var/lib/nginx/state/oidc_refresh_tokens.json timeout=8h;

    keyval $cookie_auth_token $session_jwt zone=oidc_id_tokens;
    keyval $cookie_auth_token $access_token zone=oidc_access_tokens;
    keyval $cookie_auth_token $refresh_token zone=oidc_refresh_tokens;

    # Azure AD 端点配置
    map $host $oidc_authz_endpoint {
        default "https://login.microsoftonline.com/{tenant-id}/oauth2/v2.0/authorize";
    }

    map $host $oidc_token_endpoint {
        default "https://login.microsoftonline.com/{tenant-id}/oauth2/v2.0/token";
    }

    map $host $oidc_jwks_uri {
        default "https://login.microsoftonline.com/{tenant-id}/discovery/v2.0/keys";
    }

    map $host $oidc_userinfo_endpoint {
        default "https://graph.microsoft.com/oidc/userinfo";
    }

    map $host $oidc_end_session_endpoint {
        default "https://login.microsoftonline.com/{tenant-id}/oauth2/v2.0/logout";
    }

    map $host $oidc_client {
        default "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx";
    }

    map $host $oidc_client_secret {
        default "your-client-secret";
    }

    # Azure AD 使用空格分隔 scopes
    map $host $oidc_scopes {
        default "openid+profile+email";
    }

    map $host $oidc_pkce_enable {
        default "1";
    }

    js_import /etc/nginx/conf.d/openid_connect.js;

    proxy_cache_path /var/cache/nginx/jwks levels=1:2 keys_zone=jwks_cache:1m max_size=10m inactive=60m;

    server {
        listen 443 ssl;
        server_name app.example.com;

        ssl_certificate /etc/nginx/ssl/app.example.com.crt;
        ssl_certificate_key /etc/nginx/ssl/app.example.com.key;

        location / {
            auth_jwt "Azure AD SSO" token=$cookie_auth_token;
            auth_jwt_key_request /_jwks_uri;

            auth_jwt_set $jwt_sub sub;
            auth_jwt_set $jwt_email email;
            auth_jwt_set $jwt_name name;
            auth_jwt_set $jwt_oid oid;  # Azure AD Object ID

            proxy_pass http://backend;
            proxy_set_header X-User-ID $jwt_sub;
            proxy_set_header X-User-Email $jwt_email;
            proxy_set_header X-User-Name $jwt_name;
            proxy_set_header X-Azure-Object-ID $jwt_oid;
            proxy_set_header Authorization "Bearer $access_token";
        }

        location /login {
            js_content openid_connect.login;
        }

        location /logout {
            js_content openid_connect.logout;
        }

        location = /redirect_uri {
            js_content openid_connect.redirect;
        }

        location = /_jwks_uri {
            internal;
            proxy_pass $oidc_jwks_uri;
            proxy_cache jwks_cache;
            proxy_cache_valid 200 1h;
            proxy_ssl_server_name on;
        }
    }
}
```

---

## 6. JWT 验证与 Token 刷新

### 6.1 JWT 验证机制

**验证流程**：

```
1. 提取 Token ──▶ 2. 解析 Header ──▶ 3. 获取公钥 ──▶ 4. 验证签名
                                              │
                                              ▼
5. 验证 Claims ◀── 6. 检查过期 ◀── 4. 验证签名
```

**关键验证点**：
- **签名验证**：使用 IdP 的公钥验证 JWT 签名
- **Issuer 验证**：确认 Token 来自正确的 IdP
- **Audience 验证**：确认 Token 为此应用签发
- **过期验证**：检查 `exp` Claim
- **生效时间**：检查 `nbf` (Not Before) Claim

### 6.2 JWKS 缓存配置

```nginx
# JWKS 缓存
proxy_cache_path /var/cache/nginx/jwks levels=1:2 keys_zone=jwks_cache:1m 
                 max_size=10m inactive=60m use_temp_path=off;

server {
    location = /_jwks_uri {
        internal;
        proxy_pass $oidc_jwks_uri;
        proxy_cache jwks_cache;
        proxy_cache_valid 200 1h;
        proxy_cache_use_stale error timeout updating;
        proxy_ssl_server_name on;
        
        # 连接超时设置
        proxy_connect_timeout 5s;
        proxy_send_timeout 5s;
        proxy_read_timeout 5s;
    }
}
```

### 6.3 Token 刷新机制

**自动刷新流程**：

```javascript
// openid_connect.js 中的刷新逻辑
function refreshToken(r) {
    // 1. 检查 Token 是否即将过期
    var token = r.variables.session_jwt;
    var payload = JSON.parse(jwtPayload(token));
    var exp = payload.exp;
    var now = Math.floor(Date.now() / 1000);
    
    // 2. 提前 60 秒刷新
    if (exp - now < 60) {
        // 3. 使用 Refresh Token 获取新 Token
        var refreshToken = r.variables.refresh_token;
        return exchangeRefreshToken(r, refreshToken);
    }
    
    return token;
}
```

**配置示例**：

```nginx
server {
    location / {
        # 使用 JavaScript 处理 Token 刷新
        js_set $validated_token oidc.validateAndRefresh;
        
        auth_jwt "API" token=$validated_token;
        auth_jwt_key_request /_jwks_uri;

        proxy_pass http://backend;
    }
}
```

### 6.4 多 IdP 支持

```nginx
http {
    # 根据域名选择 IdP
    map $host $oidc_config {
        default                "keycloak";
        "app1.example.com"     "keycloak";
        "app2.example.com"     "okta";
        "app3.example.com"     "auth0";
    }

    # Keycloak 配置
    map $oidc_config $keycloak_authz_endpoint {
        "keycloak" "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/auth";
        default    "";
    }

    map $oidc_config $keycloak_token_endpoint {
        "keycloak" "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/token";
        default    "";
    }

    # Okta 配置
    map $oidc_config $okta_authz_endpoint {
        "okta"  "https://myorg.okta.com/oauth2/default/v1/authorize";
        default "";
    }

    map $oidc_config $okta_token_endpoint {
        "okta"  "https://myorg.okta.com/oauth2/default/v1/token";
        default "";
    }

    # 统一端点变量
    map $oidc_config $oidc_authz_endpoint {
        "keycloak" $keycloak_authz_endpoint;
        "okta"     $okta_authz_endpoint;
    }

    map $oidc_config $oidc_token_endpoint {
        "keycloak" $keycloak_token_endpoint;
        "okta"     $okta_token_endpoint;
    }
}
```

---

## 7. JavaScript 模块 (njs) 实现

### 7.1 基础 openid_connect.js

```javascript
// openid_connect.js
var qs = require('querystring');
var jwt = require('jwt');

// 生成随机状态码
function generateState() {
    return Math.random().toString(36).substring(2, 15) + 
           Math.random().toString(36).substring(2, 15);
}

// 生成 PKCE code_verifier
function generateCodeVerifier() {
    var bytes = [];
    for (var i = 0; i < 32; i++) {
        bytes.push(Math.floor(Math.random() * 256));
    }
    return bytesToBase64Url(bytes);
}

// Base64 URL 编码
function bytesToBase64Url(bytes) {
    var base64 = '';
    var chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
    for (var i = 0; i < bytes.length; i += 3) {
        var b1 = bytes[i];
        var b2 = bytes[i + 1] || 0;
        var b3 = bytes[i + 2] || 0;
        var bitmap = (b1 << 16) | (b2 << 8) | b3;
        base64 += chars[(bitmap >> 18) & 63];
        base64 += chars[(bitmap >> 12) & 63];
        base64 += (i + 1 < bytes.length) ? chars[(bitmap >> 6) & 63] : '=';
        base64 += (i + 2 < bytes.length) ? chars[bitmap & 63] : '=';
    }
    return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

// 登录处理
function login(r) {
    var state = generateState();
    var redirectUri = 'https://' + r.headersIn['Host'] + '/redirect_uri';
    
    var params = {
        response_type: 'code',
        client_id: r.variables.oidc_client,
        redirect_uri: redirectUri,
        scope: r.variables.oidc_scopes.replace(/\+/g, ' '),
        state: state
    };
    
    // 启用 PKCE
    if (r.variables.oidc_pkce_enable === '1') {
        var codeVerifier = generateCodeVerifier();
        params.code_challenge = codeVerifier;
        params.code_challenge_method = 'S256';
        // 存储 code_verifier 用于后续 token 交换
        r.variables.code_verifier = codeVerifier;
    }
    
    var authUrl = r.variables.oidc_authz_endpoint + '?' + qs.stringify(params);
    r.return(302, authUrl);
}

// Token 交换处理
function redirect(r) {
    var args = r.args;
    
    // 检查错误
    if (args.error) {
        r.return(500, 'Authentication error: ' + args.error_description);
        return;
    }
    
    var code = args.code;
    var redirectUri = 'https://' + r.headersIn['Host'] + '/redirect_uri';
    
    // Token 请求
    var tokenReq = {
        method: 'POST',
        body: qs.stringify({
            grant_type: 'authorization_code',
            code: code,
            redirect_uri: redirectUri,
            client_id: r.variables.oidc_client,
            client_secret: r.variables.oidc_client_secret
        }),
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded'
        }
    };
    
    // 发送 Token 请求
    r.subrequest('/_token', tokenReq, function(res) {
        if (res.status !== 200) {
            r.return(500, 'Token exchange failed');
            return;
        }
        
        var tokenData = JSON.parse(res.responseBody);
        var sessionId = generateState();
        
        // 存储 Token
        r.variables.session_jwt = tokenData.id_token;
        r.variables.access_token = tokenData.access_token;
        r.variables.refresh_token = tokenData.refresh_token || '';
        
        // 设置 Cookie 并重定向
        r.headersOut['Set-Cookie'] = 'auth_token=' + sessionId + '; ' + 
                                      r.variables.session_cookie_flags;
        r.return(302, '/');
    });
}

// 登出处理
function logout(r) {
    var sessionId = r.variables.cookie_auth_token;
    
    // 清除存储的 Token
    if (sessionId) {
        delete r.variables.session_jwt;
        delete r.variables.access_token;
        delete r.variables.refresh_token;
    }
    
    // 清除 Cookie
    r.headersOut['Set-Cookie'] = 'auth_token=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT';
    
    // 如果配置了 end_session_endpoint，重定向到 IdP 登出
    if (r.variables.oidc_end_session_endpoint) {
        var logoutUrl = r.variables.oidc_end_session_endpoint + 
                       '?post_logout_redirect_uri=' + 
                       encodeURIComponent('https://' + r.headersIn['Host'] + '/');
        r.return(302, logoutUrl);
    } else {
        r.return(302, '/');
    }
}

// Token 验证和刷新
function validateAndRefresh(r) {
    var token = r.variables.session_jwt;
    if (!token) {
        return '';
    }
    
    try {
        var payload = jwt.decode(token).payload;
        var exp = payload.exp;
        var now = Math.floor(Date.now() / 1000);
        
        // Token 即将过期，尝试刷新
        if (exp - now < 60 && r.variables.refresh_token) {
            // 异步刷新 Token
            refreshAccessToken(r);
        }
        
        return token;
    } catch (e) {
        return '';
    }
}

// 导出函数
export default { login, logout, redirect, validateAndRefresh };
```

### 7.2 高级功能扩展

```javascript
// 前端频道登出处理
function frontChannelLogout(r) {
    var sid = r.args.sid;
    if (sid) {
        // 根据 sid 查找并清除会话
        clearSessionBySid(r, sid);
    }
    r.return(200, 'OK');
}

// 用户信息服务
function userInfo(r) {
    var token = r.variables.access_token;
    if (!token) {
        r.return(401, 'Unauthorized');
        return;
    }
    
    r.subrequest('/_userinfo', {
        method: 'GET',
        headers: {
            'Authorization': 'Bearer ' + token
        }
    }, function(res) {
        r.headersOut['Content-Type'] = 'application/json';
        r.return(res.status, res.responseBody);
    });
}

// 会话检查
function checkSession(r) {
    var token = r.variables.session_jwt;
    if (!token) {
        r.return(401, JSON.stringify({ active: false }));
        return;
    }
    
    try {
        var payload = jwt.decode(token).payload;
        r.return(200, JSON.stringify({
            active: true,
            sub: payload.sub,
            exp: payload.exp
        }));
    } catch (e) {
        r.return(401, JSON.stringify({ active: false }));
    }
}
```

---

## 8. 与 Lolly 项目的关系和建议

### 8.1 Lolly 项目概述

Lolly 是使用 Go 语言编写的高性能 HTTP 服务器与反向代理，与 NGINX 有相似的功能定位：

| 特性 | NGINX Plus | Lolly |
|------|-----------|-------|
| **语言** | C | Go |
| **OIDC 支持** | 内置 (auth_jwt, njs) | 待实现 |
| **配置** | nginx.conf | YAML |
| **扩展** | njs / C 模块 | Go 中间件 |
| **性能** | 极高 (C + 事件驱动) | 高 (Go + fasthttp) |
| **HTTP/3** | 实验性 | 原生支持 |
| **热重载** | 支持 | 支持 |

### 8.2 Lolly OIDC 实现建议

基于 NGINX OIDC 模块的设计，建议 Lolly 实现以下功能：

#### 配置结构示例

```yaml
# Lolly OIDC 配置
oidc:
  enabled: true
  providers:
    - name: keycloak
      issuer: "https://keycloak.example.com/realms/myrealm"
      client_id: "lolly-client"
      client_secret: "${KEYCLOAK_SECRET}"
      scopes: ["openid", "profile", "email"]
      pkce_enabled: true
      redirect_uri: "https://app.example.com/auth/callback"
      
      # JWKS 配置
      jwks:
        url: "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/certs"
        cache_ttl: "1h"
        
      # 会话配置
      session:
        cookie_name: "auth_token"
        cookie_secure: true
        cookie_http_only: true
        cookie_same_site: "Strict"
        ttl: "1h"
        
      # Token 刷新
      refresh:
        enabled: true
        before_expiry: "5m"
        
      # 用户信息转发
      claims_forward:
        - claim: "sub"
          header: "X-User-ID"
        - claim: "email"
          header: "X-User-Email"
        - claim: "name"
          header: "X-User-Name"

server:
  listen: ":443"
  
  # 全局 OIDC 保护
  oidc:
    provider: keycloak
    
  routes:
    # 公开路径（无需认证）
    - path: "/health"
      public: true
      static:
        response: '{"status":"ok"}'
        
    # 受保护 API
    - path: "/api/*"
      oidc:
        provider: keycloak
        require_claims:
          - claim: "groups"
            values: ["api-users"]
      proxy:
        target: "http://backend:8080"
        
    # 管理后台（更严格）
    - path: "/admin/*"
      oidc:
        provider: keycloak
        require_claims:
          - claim: "groups"
            values: ["admin"]
      proxy:
        target: "http://admin:8080"
```

#### 建议的实现架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Lolly Server                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │  OIDC Middleware │  │  Session Store   │  │  Token Validator │  │
│  │                  │  │  - In-Memory     │  │  - JWKS Fetch    │  │
│  │  - Login Handler │  │  - Redis         │  │  - JWT Verify    │  │
│  │  - Callback      │  │  - Cookie        │  │  - Claims Extract│  │
│  │  - Logout        │  │                  │  │                  │  │
│  │  - Refresh       │  │                  │  │                  │  │
│  └──────────────┘  └──────────────┘  └──────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

#### 推荐依赖库

| 功能 | 推荐库 | 说明 |
|------|--------|------|
| **OIDC Client** | `coreos/go-oidc` | 官方推荐，功能完整 |
| **JWT 解析** | `golang-jwt/jwt` | 最流行的 Go JWT 库 |
| **OAuth2** | `golang.org/x/oauth2` | 标准 OAuth2 实现 |
| **Session** | `gorilla/sessions` | 成熟的 Session 管理 |

#### 实现优先级建议

1. **Phase 1 - 基础功能**
   - [ ] JWT 验证中间件
   - [ ] JWKS 获取和缓存
   - [ ] 基本 Claims 提取

2. **Phase 2 - 完整认证**
   - [ ] 授权码流程
   - [ ] Session Cookie 管理
   - [ ] 登录/登出端点

3. **Phase 3 - 高级功能**
   - [ ] Token 自动刷新
   - [ ] PKCE 支持
   - [ ] 多 IdP 支持

4. **Phase 4 - 企业特性**
   - [ ] 前端频道登出
   - [ ] 用户信息服务
   - [ ] 细粒度权限控制

### 8.3 迁移建议

从 NGINX Plus OIDC 迁移到 Lolly：

1. **配置转换**：使用工具将 nginx.conf 的 map 转换为 YAML
2. **IdP 配置复用**：Issuer、Client ID、Secret 保持不变
3. **后端适配**：验证 Header 名称一致性
4. **Session 迁移**：逐步迁移，支持双写
5. **灰度切换**：基于域名或路径逐步切换

---

## 9. 故障排查

### 9.1 常见问题

#### Token 验证失败

```nginx
# 启用详细日志
error_log /var/log/nginx/error.log debug;

# 检查 JWKS 是否获取成功
location = /_jwks_uri {
    internal;
    proxy_pass $oidc_jwks_uri;
    
    # 记录响应
    add_header X-JWKS-Status $upstream_status always;
    add_header X-JWKS-Cache $upstream_cache_status always;
}
```

#### Cookie 问题

```nginx
# 确保 Cookie 配置正确
set $session_cookie "auth_token";
set $session_cookie_flags "Path=/; Secure; HttpOnly; SameSite=Lax";

# 检查 Cookie 是否设置
add_header X-Debug-Cookie $cookie_auth_token always;
```

### 9.2 调试端点

```nginx
server {
    # 健康检查端点
    location /auth/health {
        auth_jwt off;
        default_type application/json;
        return 200 '{"status":"ok","provider":"$oidc_config"}';
    }
    
    # Token 信息端点（仅调试）
    location /auth/debug {
        auth_jwt "Debug" token=$cookie_auth_token;
        auth_jwt_key_request /_jwks_uri;
        
        auth_jwt_set $jwt_sub sub;
        auth_jwt_set $jwt_exp exp;
        
        default_type application/json;
        return 200 '{
            "sub": "$jwt_sub",
            "exp": "$jwt_exp",
            "provider": "$oidc_config"
        }';
    }
}
```

### 9.3 日志分析

```bash
# 查看认证相关日志
grep "auth_jwt\|oidc\|openid" /var/log/nginx/error.log | tail -100

# 监控 Token 刷新
awk '/token.*refresh/ {print $0}' /var/log/nginx/access.log
```

---

## 10. 最佳实践

### 10.1 安全建议

1. **始终启用 HTTPS**
   ```nginx
   # 拒绝 HTTP 访问
   server {
       listen 80;
       return 301 https://$host$request_uri;
   }
   ```

2. **使用 Secure Cookie**
   ```nginx
   set $session_cookie_flags "Path=/; Secure; HttpOnly; SameSite=Strict";
   ```

3. **启用 PKCE**
   ```nginx
   map $host $oidc_pkce_enable {
       default "1";
   }
   ```

4. **定期轮换 Client Secret**

### 10.2 性能优化

1. **JWKS 缓存**
   ```nginx
   proxy_cache_valid 200 1h;
   proxy_cache_use_stale error timeout updating;
   ```

2. **会话存储优化**
   ```nginx
   # 共享内存区域大小根据并发用户数调整
   keyval_zone zone=oidc_id_tokens:10M timeout=1h;
   ```

3. **连接池**
   ```nginx
   upstream idp_backend {
       server keycloak.example.com:443;
       keepalive 32;
   }
   ```

### 10.3 高可用配置

```nginx
# JWKS 多源备份
upstream jwks_upstream {
    server keycloak-primary.example.com:443;
    server keycloak-backup.example.com:443 backup;
}

location = /_jwks_uri {
    internal;
    proxy_pass https://jwks_upstream/realms/myrealm/protocol/openid-connect/certs;
    proxy_cache jwks_cache;
    proxy_cache_valid 200 1h;
}
```

---

## 参考资料

- [NGINX Plus OIDC Reference Implementation](https://github.com/nginxinc/nginx-openid-connect)
- [OpenID Connect Core 1.0 Specification](https://openid.net/specs/openid-connect-core-1_0.html)
- [OAuth 2.0 Authorization Framework](https://tools.ietf.org/html/rfc6749)
- [JSON Web Token (JWT) Specification](https://tools.ietf.org/html/rfc7519)
- [Proof Key for Code Exchange (PKCE)](https://tools.ietf.org/html/rfc7636)
- [Lolly 项目文档](./README.md)
