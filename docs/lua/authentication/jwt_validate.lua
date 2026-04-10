-- JWT 验证示例 - HMAC-SHA256 纯 Lua 实现
--
-- 功能：从 Authorization Header 提取 Bearer Token，
--       解码 JWT Header/Payload，使用 HMAC-SHA256 验证签名，
--       检查过期时间。
--
-- 依赖：OpenResty (ngx.*, cjson, resty.hmac)
--
-- 使用方式（在 nginx.conf 中）：
--   access_by_lua_file /path/to/jwt_validate.lua;

-- ==================== 配置 ====================

-- HMAC 签名密钥（生产环境应从环境变量或 Vault 读取）
local SECRET = "your-super-secret-key-change-in-production"

-- 是否跳过 OPTIONS 预检请求
local SKIP_PREFLIGHT = true

-- ==================== 工具函数 ====================

local cjson = require("cjson.safe")
local hmac = require("resty.hmac")

--- Base64URL 解码
-- 将 Base64URL 编码的字符串转为标准 Base64 并解码
local function base64url_decode(str)
    str = str:gsub("-", "+"):gsub("_", "/")
    local mod = #str % 4
    if mod == 2 then
        str = str .. "=="
    elseif mod == 3 then
        str = str .. "="
    end
    return ngx.decode_base64(str)
end

--- HMAC-SHA256 签名
local function hmac_sha256(key, message)
    local hm = hmac:new(key, hmac.ALGOS.SHA256)
    local ok = hm:update(message)
    if not ok then
        return nil, "hmac update failed"
    end
    local digest = hm:final()
    hm:close()
    return digest
end

--- Base64URL 编码（无填充）
local function base64url_encode(str)
    return (ngx.encode_base64(str):gsub("+", "-"):gsub("/", "_"):gsub("=", ""))
end

--- 验证 JWT 签名
local function verify_signature(header_b64, payload_b64, signature_b64)
    local signing_input = header_b64 .. "." .. payload_b64

    local expected_sig = hmac_sha256(SECRET, signing_input)
    if not expected_sig then
        return false, "signing failed"
    end

    local actual_sig, err = base64url_decode(signature_b64)
    if not actual_sig then
        return false, "decode signature failed: " .. err
    end

    return expected_sig == actual_sig, nil
end

-- ==================== 主逻辑 ====================

-- 跳过 OPTIONS 预检请求
if SKIP_PREFLIGHT and ngx.req.get_method() == "OPTIONS" then
    return
end

-- 提取 Authorization Header
local auth_header = ngx.req.get_headers()["Authorization"]
if not auth_header or not auth_header:match("^Bearer%s+") then
    ngx.status = 401
    ngx.header["Content-Type"] = "application/json"
    ngx.say(cjson.encode({
        error = "missing_or_invalid_token",
        message = "Authorization header with Bearer token required",
    }))
    return ngx.exit(401)
end

local token = auth_header:match("^Bearer%s+(.+)$")
if not token then
    ngx.status = 401
    ngx.header["Content-Type"] = "application/json"
    ngx.say(cjson.encode({
        error = "missing_or_invalid_token",
        message = "Invalid Bearer token format",
    }))
    return ngx.exit(401)
end

-- 分割 JWT 为三部分
local header_b64, payload_b64, signature_b64 = token:match("^(%S+)%.(%S+)%.(%S+)$")
if not header_b64 or not payload_b64 or not signature_b64 then
    ngx.status = 401
    ngx.header["Content-Type"] = "application/json"
    ngx.say(cjson.encode({
        error = "malformed_token",
        message = "JWT must have exactly 3 parts separated by '.'",
    }))
    return ngx.exit(401)
end

-- 验证签名
local sig_valid, sig_err = verify_signature(header_b64, payload_b64, signature_b64)
if not sig_valid then
    ngx.status = 401
    ngx.header["Content-Type"] = "application/json"
    ngx.say(cjson.encode({
        error = "invalid_signature",
        message = "Token signature verification failed: " .. (sig_err or ""),
    }))
    return ngx.exit(401)
end

-- 解码 Payload 并检查过期
local payload_decoded = base64url_decode(payload_b64)
if not payload_decoded then
    ngx.status = 401
    ngx.header["Content-Type"] = "application/json"
    ngx.say(cjson.encode({
        error = "decode_failed",
        message = "Failed to decode JWT payload",
    }))
    return ngx.exit(401)
end

local payload, err = cjson.decode(payload_decoded)
if not payload then
    ngx.status = 401
    ngx.header["Content-Type"] = "application/json"
    ngx.say(cjson.encode({
        error = "invalid_payload",
        message = "JWT payload is not valid JSON",
    }))
    return ngx.exit(401)
end

-- 检查过期时间
if payload.exp then
    if ngx.now() >= payload.exp then
        ngx.status = 401
        ngx.header["Content-Type"] = "application/json"
        ngx.say(cjson.encode({
            error = "token_expired",
            message = "JWT has expired",
        }))
        return ngx.exit(401)
    end
end

-- 验证通过，将解析后的信息存入 ngx.ctx 供后续阶段使用
ngx.ctx.jwt_payload = payload
