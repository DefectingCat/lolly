-- gateway.lua - API 网关主逻辑
-- 负责：路由匹配、认证、限流、错误处理

local cjson = require("cjson.safe")
local upstream = require("upstream")

local _M = {}

-- ============================================================
-- 路由规则配置
-- ============================================================

local routes = {
    -- 用户服务
    ["/v1/users"] = {
        method = {"GET", "POST"},
        upstream = "user_service",
        auth = true,
        rate_limit = 200,       -- 每分钟请求数
    },
    ["/v1/users/:id"] = {
        method = {"GET", "PUT", "DELETE"},
        upstream = "user_service",
        auth = true,
        rate_limit = 100,
    },

    -- 订单服务
    ["/v1/orders"] = {
        method = {"GET", "POST"},
        upstream = "order_service",
        auth = true,
        rate_limit = 100,
    },

    -- 公开服务（无需认证）
    ["/v1/products"] = {
        method = {"GET"},
        upstream = "product_service",
        auth = false,
        rate_limit = 500,
    },

    -- 健康检查（跳过限流）
    ["/v1/health"] = {
        method = {"GET"},
        upstream = "user_service",
        auth = false,
        rate_limit = 0,         -- 不限流
    },
}

-- ============================================================
-- API Key 存储（生产环境应使用 Redis/数据库）
-- ============================================================

local api_keys = {
    ["ak_test_001"] = { name = "test_app",   roles = {"read"} },
    ["ak_prod_002"] = { name = "prod_app",   roles = {"read", "write"} },
    ["ak_admin_003"] = { name = "admin_app", roles = {"read", "write", "admin"} },
}

-- ============================================================
-- 响应辅助函数
-- ============================================================

local function send_json(status_code, body)
    ngx.status = status_code
    ngx.header["Content-Type"] = "application/json; charset=utf-8"
    ngx.say(cjson.encode(body))
    return ngx.exit(status_code)
end

-- ============================================================
-- 路由匹配
-- ============================================================

--- 匹配请求路径到路由规则
-- 支持精确匹配和带参数的路径（如 /v1/users/:id）
local function match_route(uri, method)
    -- 1. 精确匹配
    local route = routes[uri]
    if route then
        for _, m in ipairs(route.method) do
            if m == method then
                return route, uri
            end
        end
        return nil
    end

    -- 2. 参数化路径匹配（/v1/users/:id -> /v1/users/123）
    for pattern, rule in pairs(routes) do
        local regex = pattern:gsub(":[^/]+", "[^/]+")
        regex = "^" .. regex .. "$"
        if ngx.re.match(uri, regex, "jo") then
            for _, m in ipairs(rule.method) do
                if m == method then
                    return rule, uri
                end
            end
        end
    end

    return nil
end

-- ============================================================
-- 认证中间件
-- ============================================================

local function authenticate(route)
    if not route.auth then
        return true
    end

    local api_key = ngx.var.http_x_api_key
    if not api_key or api_key == "" then
        send_json(401, {
            error = "unauthorized",
            message = "缺少 API Key，请在 X-Api-Key 请求头中提供",
        })
        return false
    end

    local key_info = api_keys[api_key]
    if not key_info then
        send_json(403, {
            error = "forbidden",
            message = "无效的 API Key",
        })
        return false
    end

    -- 将认证信息注入请求头，传递给上游
    ngx.req.set_header("X-Auth-App", key_info.name)
    return true
end

-- ============================================================
-- 限流中间件（基于共享字典的滑动窗口）
-- ============================================================

local function rate_limit(route)
    local limit = route.rate_limit
    if not limit or limit == 0 then
        return true
    end

    local dict = ngx.shared.rate_limit
    local client_ip = ngx.var.remote_addr
    local route_path = ngx.var.uri
    local key = client_ip .. ":" .. route_path

    local window = 60  -- 60 秒窗口
    local now = ngx.now()

    -- 清理过期条目
    local current = dict:get(key) or 0
    local window_start = dict:get(key .. ":window_start") or now

    if now - window_start >= window then
        -- 新窗口
        dict:set(key, 1, window + 1)
        dict:set(key .. ":window_start", now, window + 1)
        return true
    end

    -- 当前窗口计数
    if current >= limit then
        send_json(429, {
            error = "too_many_requests",
            message = "请求频率超限，请稍后重试",
            retry_after = math.ceil(window_start + window - now),
        })
        return false
    end

    dict:incr(key, 1, nil, window + 1)
    return true
end

-- ============================================================
-- 主处理入口
-- ============================================================

--- 在 access_by_lua_block 中调用
function _M.handle()
    local method = ngx.req.get_method()
    local uri = ngx.var.uri

    -- 1. 路由匹配
    local route, matched_path = match_route(uri, method)
    if not route then
        send_json(404, {
            error = "not_found",
            message = "路由不存在: " .. method .. " " .. uri,
        })
        return
    end

    -- 记录匹配的路由，后续阶段使用
    ngx.ctx.matched_route = route
    ngx.ctx.matched_path = matched_path
    ngx.ctx.request_start = ngx.now()

    -- 2. 限流检查
    if not rate_limit(route) then
        return
    end

    -- 3. 认证检查
    if not authenticate(route) then
        return
    end

    -- 4. 选择上游服务器
    local peer = upstream.select(route.upstream)
    if not peer then
        send_json(503, {
            error = "service_unavailable",
            message = "上游服务不可用: " .. route.upstream,
        })
        return
    end

    -- 设置代理目标（需要配合 balancer_by_lua_block 使用）
    ngx.ctx.upstream_peer = peer
    ngx.var.upstream_addr = peer.host .. ":" .. peer.port
end

-- ============================================================
-- 响应过滤
-- ============================================================

--- 在 body_filter_by_lua_block 中调用
function _M.filter_response()
    -- 添加统一响应头
    if ngx.header["X-Request-Id"] == nil then
        ngx.header["X-Request-Id"] = ngx.var.request_id or ""
    end

    -- 如果是 JSON 响应，包装统一格式
    local content_type = ngx.header["Content-Type"] or ""
    if string.find(content_type, "application/json") and ngx.status >= 400 then
        -- 仅对错误响应做包装（避免重复包装）
        if ngx.ctx.error_wrapped then
            return
        end
        ngx.ctx.error_wrapped = true
    end
end

-- ============================================================
-- 请求日志
-- ============================================================

--- 在 log_by_lua_block 中调用
function _M.log_request()
    local route = ngx.ctx.matched_route
    if not route then
        return
    end

    local elapsed = ngx.now() - (ngx.ctx.request_start or ngx.now())

    -- 记录慢请求
    if elapsed > 1.0 then
        ngx.log(ngx.WARN, string.format(
            "slow request: %s %s -> %s (%.3fs)",
            ngx.req.get_method(),
            ngx.var.uri,
            ngx.status,
            elapsed
        ))
    end

    -- 上报上游健康状态
    if route.upstream and ngx.status >= 500 then
        upstream.report_failure(route.upstream)
    elseif route.upstream and ngx.status < 400 then
        upstream.report_success(route.upstream)
    end
end

return _M
