-- Basic Auth 验证示例
--
-- 功能：解析 Authorization 头中的 Basic 凭据，
--       校验用户名密码，失败返回 401。
--
-- 依赖：OpenResty (ngx.*, cjson)
--
-- 使用方式（在 nginx.conf 中）：
--   access_by_lua_file /path/to/basic_auth.lua;

-- ==================== 配置 ====================

-- 用户凭据表（生产环境应使用 Redis / 数据库）
local USERS = {
    ["admin"]   = "admin-secret",
    ["reader"]  = "read-only-pass",
}

-- 认证域名称（浏览器弹窗显示的名称）
local REALM = "Lolly API Gateway"

-- 是否跳过 OPTIONS 预检请求
local SKIP_PREFLIGHT = true

-- ==================== 工具函数 ====================

local cjson = require("cjson.safe")

--- 校验用户名密码
local function authenticate(username, password)
    local expected = USERS[username]
    if not expected then
        return false
    end
    return expected == password
end

--- 返回 401 响应，附带 WWW-Authenticate 头
local function challenge(err_msg)
    ngx.status = 401
    ngx.header["WWW-Authenticate"] = 'Basic realm="' .. REALM .. '", charset="UTF-8"'
    ngx.header["Content-Type"] = "application/json"
    ngx.say(cjson.encode({
        error = "unauthorized",
        message = err_msg or "Valid credentials required",
    }))
    return ngx.exit(401)
end

-- ==================== 主逻辑 ====================

-- 跳过 OPTIONS 预检请求
if SKIP_PREFLIGHT and ngx.req.get_method() == "OPTIONS" then
    return
end

-- 提取 Authorization Header
local auth_header = ngx.req.get_headers()["Authorization"]
if not auth_header then
    return challenge("Missing Authorization header")
end

-- 验证是否为 Basic 类型
local credentials = auth_header:match("^%s*Basic%s+(.+)$")
if not credentials then
    return challenge("Invalid Authorization scheme, expected Basic")
end

-- 解码 Base64
local decoded = ngx.decode_base64(credentials)
if not decoded then
    return challenge("Invalid Base64 encoding in credentials")
end

-- 分割 username:password
local username, password = decoded:match("^([^:]+):(.+)$")
if not username or not password then
    return challenge("Credentials must be in username:password format")
end

-- 校验凭据
if not authenticate(username, password) then
    return challenge("Invalid username or password")
end

-- 验证通过，将用户名存入 ngx.ctx 供后续阶段使用
ngx.ctx.auth_user = username
