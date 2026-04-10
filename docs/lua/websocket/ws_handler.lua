-- WebSocket 处理器
-- 用于 Nginx Lua 沙箱环境中的 WebSocket 连接管理
--
-- 功能：
--   1. 连接验证（Token 校验）
--   2. 消息路由与处理
--   3. 心跳保活
--   4. 连接状态管理

local cjson = require "cjson"

-- ==========================================
-- 配置
-- ==========================================
local config = {
    -- Token 验证
    token_param = "token",
    token_header = "X-WS-Token",

    -- 心跳设置
    heartbeat_interval = 30,     -- 心跳间隔（秒）
    heartbeat_timeout = 90,      -- 心跳超时断开（秒）
    heartbeat_opcode = 9,        -- WebSocket Ping 操作码

    -- 消息限制
    max_message_size = 65536,    -- 最大消息大小 64KB
    max_connections_per_ip = 50, -- 单 IP 最大连接数

    -- 日志
    log_level = ngx.INFO,
}

-- ==========================================
-- 连接验证
-- ==========================================
local function validate_connection()
    local dict = ngx.shared.ws_tokens

    -- 优先从查询参数获取 Token
    local token = ngx.var.arg_token
    if not token then
        -- 从请求头获取
        token = ngx.req.get_headers()[config.token_header]
    end

    if not token or token == "" then
        ngx.log(ngx.WARN, "WebSocket connection rejected: missing token, client=", ngx.var.remote_addr)
        ngx.exit(ngx.HTTP_UNAUTHORIZED)
        return false
    end

    -- 验证 Token 有效性
    local ok, err = dict:get(token)
    if not ok then
        ngx.log(ngx.WARN, "WebSocket connection rejected: invalid token, client=", ngx.var.remote_addr)
        ngx.exit(ngx.HTTP_UNAUTHORIZED)
        return false
    end

    -- 检查单 IP 连接数限制
    local conn_key = "conn:" .. ngx.var.remote_addr
    local count = dict:get(conn_key) or 0
    if count >= config.max_connections_per_ip then
        ngx.log(ngx.WARN, "WebSocket connection rejected: max connections exceeded, client=", ngx.var.remote_addr)
        ngx.exit(ngx.HTTP_TOO_MANY_REQUESTS)
        return false
    end

    -- 记录连接
    dict:incr(conn_key, 1, 0, config.heartbeat_timeout)

    ngx.log(config.log_level, "WebSocket connection accepted, client=", ngx.var.remote_addr, " token=", token)
    return true
end

-- ==========================================
-- 消息处理框架
-- ==========================================
local handlers = {}

-- 注册消息处理器
-- @param msg_type  消息类型字符串
-- @param handler_fn  处理函数 function(msg_data, ctx)
local function register_handler(msg_type, handler_fn)
    handlers[msg_type] = handler_fn
end

-- 默认处理器：未注册的消息类型
local function default_handler(msg_data, ctx)
    ngx.log(ngx.WARN, "Unhandled message type: ", msg_data.type, " from ", ctx.client_ip)
    return {
        status = "error",
        message = "Unknown message type: " .. tostring(msg_data.type),
    }
end

-- 分派消息到对应处理器
-- @param raw_msg  原始消息字符串
-- @param ctx  连接上下文
local function dispatch_message(raw_msg, ctx)
    -- 检查消息大小
    if #raw_msg > config.max_message_size then
        return {
            status = "error",
            message = "Message too large (max " .. config.max_message_size .. " bytes)",
        }
    end

    -- 解析 JSON 消息
    local ok, msg_data = pcall(cjson.decode, raw_msg)
    if not ok then
        ngx.log(ngx.ERR, "Failed to decode message: ", msg_data)
        return {
            status = "error",
            message = "Invalid JSON message",
        }
    end

    if not msg_data.type then
        return {
            status = "error",
            message = "Missing message type",
        }
    end

    -- 查找并调用处理器
    local handler = handlers[msg_data.type] or default_handler
    local status, result = pcall(handler, msg_data, ctx)
    if not status then
        ngx.log(ngx.ERR, "Handler error for type '", msg_data.type, "': ", result)
        return {
            status = "error",
            message = "Internal handler error",
        }
    end

    return result
end

-- ==========================================
-- 内置消息处理器
-- ==========================================

-- 心跳响应
register_handler("ping", function(msg_data, ctx)
    local dict = ngx.shared.ws_connections
    dict:set(ctx.connection_id, ngx.time(), config.heartbeat_timeout)

    return {
        type = "pong",
        timestamp = ngx.time(),
    }
end)

-- 认证消息（连接后二次验证）
register_handler("auth", function(msg_data, ctx)
    local token = msg_data.token
    if not token then
        return { status = "error", message = "Missing auth token" }
    end

    local dict = ngx.shared.ws_tokens
    local valid = dict:get(token)
    if not valid then
        return { status = "error", message = "Invalid auth token" }
    end

    ctx.authenticated = true
    ngx.log(config.log_level, "Client authenticated: ", ctx.client_ip)

    return {
        status = "ok",
        message = "Authenticated successfully",
    }
end)

-- 示例：Echo 处理器
register_handler("echo", function(msg_data, ctx)
    return {
        type = "echo",
        data = msg_data.data,
        timestamp = ngx.time(),
    }
end)

-- ==========================================
-- 连接上下文管理
-- ==========================================
local function build_context()
    local connection_id = ngx.md5(ngx.time() .. ngx.var.remote_addr .. ngx.var.request_id)

    return {
        connection_id = connection_id,
        client_ip = ngx.var.remote_addr,
        user_agent = ngx.var.http_user_agent,
        connected_at = ngx.time(),
        authenticated = false,
        last_activity = ngx.time(),
    }
end

-- ==========================================
-- 主入口（用于 access_by_lua_file）
-- ==========================================
-- 注意：实际的 WebSocket 消息循环需要在后端服务或使用
-- lua-resty-websocket 等库在 content 阶段处理。
-- 此处理器主要用于连接验证和预处理。

local ctx = build_context()
local valid = validate_connection()

if valid then
    -- 连接验证通过，允许继续代理到后端
    -- 实际的消息处理在后端 WebSocket 服务中进行
    ngx.log(config.log_level, "WebSocket proxy authorized for ", ctx.connection_id)
end

-- 导出模块供其他 Lua 脚本使用
return {
    config = config,
    validate_connection = validate_connection,
    dispatch_message = dispatch_message,
    register_handler = register_handler,
    build_context = build_context,
    handlers = handlers,
}
