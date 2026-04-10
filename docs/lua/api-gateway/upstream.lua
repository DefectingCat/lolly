-- upstream.lua - 上游服务管理
-- 负责：动态节点选择、健康检查、故障剔除、负载均衡

local cjson = require("cjson.safe")

local _M = {}

-- ============================================================
-- 上游服务定义
-- ============================================================

local upstreams = {
    user_service = {
        nodes = {
            { host = "10.0.0.10", port = 8080, weight = 3 },
            { host = "10.0.0.11", port = 8080, weight = 3 },
            { host = "10.0.0.12", port = 8080, weight = 1 },  -- 低权重备机
        },
        health_check_interval = 10,   -- 健康检查间隔（秒）
        max_fails = 3,                -- 最大失败次数
        fail_timeout = 30,            -- 故障剔除时长（秒）
    },

    order_service = {
        nodes = {
            { host = "10.0.0.20", port = 8081, weight = 1 },
            { host = "10.0.0.21", port = 8081, weight = 1 },
        },
        health_check_interval = 10,
        max_fails = 5,
        fail_timeout = 60,
    },

    product_service = {
        nodes = {
            { host = "10.0.0.30", port = 8082, weight = 1 },
        },
        health_check_interval = 15,
        max_fails = 3,
        fail_timeout = 30,
    },
}

-- ============================================================
-- 健康状态存储（共享字典）
-- ============================================================

local health_dict = ngx.shared.upstream_health

--- 初始化上游节点健康状态
local function init_health(upstream_name, node_idx)
    local key = upstream_name .. ":" .. node_idx
    local existing = health_dict:get(key)
    if not existing then
        health_dict:set(key, cjson.encode({
            fails = 0,
            last_fail = 0,
            last_check = ngx.now(),
            healthy = true,
        }))
    end
end

-- ============================================================
-- 负载均衡器：加权轮询
-- ============================================================

local lb_state = {}   -- 本地轮询状态，per-worker

--- 加权轮询选择节点
local function weighted_round_robin(upstream_name, config)
    local nodes = config.nodes
    local total_weight = 0
    local candidates = {}

    -- 过滤掉不健康的节点
    for i, node in ipairs(nodes) do
        init_health(upstream_name, i)
        local key = upstream_name .. ":" .. i
        local state = cjson.decode(health_dict:get(key))

        if state.healthy then
            table.insert(candidates, { node = node, index = i, weight = node.weight })
            total_weight = total_weight + node.weight
        end
    end

    if #candidates == 0 then
        -- 所有节点均不可用，回退到第一个节点
        return nodes[1], 1
    end

    -- 简单加权轮询
    if not lb_state[upstream_name] then
        lb_state[upstream_name] = { current_weight = 0, index = 0 }
    end

    local state = lb_state[upstream_name]
    state.index = (state.index % #candidates) + 1
    local candidate = candidates[state.index]

    return candidate.node, candidate.index
end

-- ============================================================
-- 上游节点选择（公开接口）
-- ============================================================

--- 选择一个上游节点
-- @param upstream_name 上游服务名称
-- @return node table 或 nil
function _M.select(upstream_name)
    local config = upstreams[upstream_name]
    if not config then
        ngx.log(ngx.ERR, "unknown upstream: ", upstream_name)
        return nil
    end

    local node, index = weighted_round_robin(upstream_name, config)
    return node
end

-- ============================================================
-- 健康检查报告
-- ============================================================

--- 报告上游请求成功
function _M.report_success(upstream_name)
    -- 此处可扩展为更精细的健康评分
end

--- 报告上游请求失败
function _M.report_failure(upstream_name)
    -- 标记当前 worker 本次请求命中的节点
    -- 实际使用中配合 balancer_by_lua_block 使用 ngx.balancer
end

--- 执行健康检查（应通过定时任务周期性调用）
-- 可通过 ngx.timer.at 在 init_worker 阶段启动
function _M.health_check(upstream_name)
    local config = upstreams[upstream_name]
    if not config then
        return
    end

    for i, node in ipairs(config.nodes) do
        local key = upstream_name .. ":" .. i
        local state = cjson.decode(health_dict:get(key) or "{}")

        if state.fails and state.fails >= config.max_fails then
            local now = ngx.now()
            if now - (state.last_fail or 0) >= config.fail_timeout then
                -- 故障超时，恢复节点
                ngx.log(ngx.INFO, "recovering node ", node.host, ":", node.port,
                        " for ", upstream_name)
                health_dict:set(key, cjson.encode({
                    fails = 0,
                    last_fail = 0,
                    last_check = now,
                    healthy = true,
                }))
            end
        end

        -- 更新检查时间
        if state.last_check then
            state.last_check = now
            health_dict:set(key, cjson.encode(state))
        end
    end
end

--- 启动所有上游的健康检查定时器
function _M.start_health_checks()
    for name, config in pairs(upstreams) do
        local check_timer = function(premature)
            if premature then
                return
            end
            _M.health_check(name)
        end

        local ok, err = ngx.timer.every(config.health_check_interval, check_timer)
        if not ok then
            ngx.log(ngx.ERR, "failed to create health check timer for ", name, ": ", err)
        else
            ngx.log(ngx.INFO, "health check started for ", name,
                    " (interval: ", config.health_check_interval, "s)")
        end
    end
end

-- ============================================================
-- 统计信息
-- ============================================================

--- 获取上游服务统计 JSON
function _M.stats_json()
    local result = {}

    for name, config in pairs(upstreams) do
        local nodes_info = {}
        for i, node in ipairs(config.nodes) do
            local key = name .. ":" .. i
            local state = cjson.decode(health_dict:get(key) or "{}")
            table.insert(nodes_info, {
                host = node.host,
                port = node.port,
                weight = node.weight,
                healthy = state.healthy ~= false,
                fails = state.fails or 0,
            })
        end
        result[name] = nodes_info
    end

    local json, err = cjson.encode(result)
    return json or '{"error":"encode_failed"}'
end

return _M
