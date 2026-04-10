-- access.lua
-- 基于 ngx.shared.DICT 的速率限制实现
--
-- 限流策略：
--   1. 优先使用 X-API-Key 请求头进行限流
--   2. 无 API Key 时回退到客户端 IP
--
-- 响应头：
--   X-RateLimit-Limit     : 当前时间窗口允许的最大请求数
--   X-RateLimit-Remaining : 剩余可用请求数
--   X-RateLimit-Reset     : 计数器重置时间戳（秒）

local limit_dict = ngx.shared.rate_limit

-- 限流配置
local config = {
    window      = 60,    -- 时间窗口（秒）
    max_requests = 20,    -- 默认每窗口最大请求数（IP）
    api_key_max = 100,    -- API Key 每窗口最大请求数
}

-- 获取客户端标识
local function get_client_id()
    local api_key = ngx.req.get_headers()["X-API-Key"]
    if api_key and api_key ~= "" then
        return "apikey:" .. api_key, config.api_key_max
    end
    return "ip:" .. ngx.var.binary_remote_addr, config.max_requests
end

-- 检查并更新计数器
-- 返回: allowed (boolean), remaining (number), limit (number), reset (number)
local function check_rate_limit(client_id, max_req)
    local key = client_id
    local now = ngx.now()

    local count, err = limit_dict:get(key)
    if err then
        ngx.log(ngx.ERR, "failed to get rate limit key: ", err)
        return true, max_req, max_req, math.ceil(now + config.window)
    end

    if count == nil then
        -- 首次请求，初始化计数器
        local ok, set_err = limit_dict:set(key, 1, config.window)
        if not ok then
            ngx.log(ngx.ERR, "failed to set rate limit key: ", set_err)
            return true, max_req, max_req, math.ceil(now + config.window)
        end
        return true, max_req - 1, max_req, math.ceil(now + config.window)
    end

    -- 获取键的剩余存活时间
    local ttl, ttl_err = limit_dict:ttl(key)
    if ttl_err then
        ngx.log(ngx.WARN, "failed to get TTL: ", ttl_err)
        ttl = config.window
    end

    local reset_time = math.ceil(now + (ttl or config.window))

    if count >= max_req then
        -- 超过限制
        return false, 0, max_req, reset_time
    end

    -- 原子递增计数器
    local new_count, incr_err = limit_dict:incr(key, 1)
    if incr_err then
        ngx.log(ngx.ERR, "failed to increment rate limit counter: ", incr_err)
        return true, max_req - count, max_req, reset_time
    end

    return true, max_req - new_count, max_req, reset_time
end

-- 主逻辑
local client_id, max_req = get_client_id()
local allowed, remaining, limit, reset_time = check_rate_limit(client_id, max_req)

-- 设置响应头
ngx.header["X-RateLimit-Limit"] = limit
ngx.header["X-RateLimit-Remaining"] = remaining
ngx.header["X-RateLimit-Reset"] = reset_time

if not allowed then
    ngx.status = 429
    ngx.header["Retry-After"] = reset_time - math.ceil(ngx.now())
    ngx.header["Content-Type"] = "application/json"
    ngx.say('{"error":"rate_limit_exceeded","message":"Too Many Requests","retry_after":' ..
        (reset_time - math.ceil(ngx.now())) .. '}')
    return ngx.exit(429)
end
