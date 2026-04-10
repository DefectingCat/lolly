-- metrics.lua
-- 请求耗时统计与指标收集

local M = {}

local shared

--- 初始化 metrics 模块
-- @param shared_dict nginx shared dict（通过 lua_shared_dict 创建）
function M.init(shared_dict)
    shared = shared_dict
end

--- 原子递增计数器
-- @param key 指标键
-- @param delta 增量（默认 1）
local function incr(key, delta)
    local newval, err = shared:incr(key, delta or 1)
    if not newval then
        -- key 不存在，初始化为 0 再递增
        shared:add(key, 0)
        shared:incr(key, delta or 1)
    end
end

--- 更新请求耗时统计
-- 在 log_by_lua 阶段调用
-- @param ngx_obj nginx 上下文对象
function M.observe(ngx_obj)
    if not shared then
        return
    end

    local status = ngx_obj.status or 0
    local uri = ngx_obj.var.uri or "unknown"
    local method = ngx_obj.var.request_method or "UNKNOWN"
    local request_time = tonumber(ngx_obj.var.request_time) or 0

    local prefix = method .. ":" .. uri

    -- 总请求数
    incr("req:total", 1)

    -- 按状态码分组
    local status_group = math.floor(status / 100) .. "xx"
    incr("req:" .. status_group, 1)

    -- 按路径+状态码分组
    incr("req:" .. prefix .. ":" .. status, 1)

    -- 请求耗时累计（用于计算平均值）
    shared:incr("time:" .. prefix .. ":sum", request_time)
    incr("time:" .. prefix .. ":count", 1)

    -- 最大耗时
    local max_key = "time:" .. prefix .. ":max"
    local current_max = shared:get(max_key)
    if not current_max or request_time > current_max then
        shared:set(max_key, request_time)
    end
end

--- 生成指标报告
-- @param ngx_obj nginx 上下文对象
function M.report(ngx_obj)
    if not shared then
        ngx_obj.status = 500
        return ngx_obj.say("metrics not initialized")
    end

    local lines = {}

    -- 遍历共享字典收集所有指标
    for key, value in shared:get_keys(0) do
        lines[#lines + 1] = key .. " " .. tostring(value)
    end

    table.sort(lines)

    ngx_obj.header("Content-Type", "text/plain; charset=utf-8")
    ngx_obj.say("# lolly metrics")
    for _, line in ipairs(lines) do
        ngx_obj.say(line)
    end
end

--- 重置所有指标
-- @param ngx_obj nginx 上下文对象
function M.reset(ngx_obj)
    if not shared then
        return
    end
    shared:flush_all()
end

return M
