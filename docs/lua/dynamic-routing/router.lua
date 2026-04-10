--- router.lua - 动态路由模块
--
-- 基于路径和请求头的动态路由分发
--
-- 路由优先级:
--   1. X-Service 请求头 -> 直接指定 upstream
--   2. X-Canary 请求头  -> 灰度 upstream
--   3. X-Version 请求头 -> 版本化 upstream
--   4. 路径前缀匹配      -> 规则匹配 upstream
--   5. 默认 upstream     -> 兜底

local cjson = require "cjson.safe"

local _M = {}

--- 获取路由规则
-- @return table 路由规则
function _M.get_rules()
    local router_config = ngx.shared.router_config
    if not router_config then
        return { error = "router_config shared dict not found" }
    end

    local raw = router_config:get("rules")
    return cjson.decode(raw) or {}
end

--- 更新路由规则
-- @param body string JSON 格式的规则数据
-- @return boolean, string|nil 成功标志, 错误信息
function _M.update_rules(body)
    local router_config = ngx.shared.router_config
    if not router_config then
        return false, "router_config shared dict not found"
    end

    local data, err = cjson.decode(body)
    if not data then
        return false, "invalid JSON: " .. (err or "unknown error")
    end

    -- 验证规则格式
    if not data.path or type(data.path) ~= "table" then
        return false, "missing or invalid 'path' field"
    end

    if not data.default or type(data.default) ~= "string" then
        return false, "missing or invalid 'default' field"
    end

    local ok = router_config:set("rules", cjson.encode(data))
    if not ok then
        return false, "failed to store rules"
    end

    ngx.log(ngx.INFO, "Routing rules updated")
    return true
end

--- 根据 X-Service 头选择 upstream
-- @param ngx nginx 对象
-- @return string|nil upstream 名称
local function select_by_service_header(ngx)
    local service = ngx.req.get_headers()["X-Service"]
    if service and service ~= "" then
        ngx.log(ngx.DEBUG, "Route by X-Service header: ", service)
        return service
    end
    return nil
end

--- 根据灰度头选择 upstream
-- @param ngx nginx 对象
-- @param base_name string 基础 upstream 名称
-- @return string|nil 灰度 upstream 名称
local function select_canary(ngx, base_name)
    local canary = ngx.req.get_headers()["X-Canary"]
    if canary and (canary == "true" or canary == "1") then
        local canary_name = base_name .. "_canary"
        ngx.log(ngx.DEBUG, "Route to canary: ", canary_name)
        return canary_name
    end
    return nil
end

--- 根据版本号选择 upstream
-- @param ngx nginx 对象
-- @param base_name string 基础 upstream 名称
-- @return string|nil 版本化 upstream 名称
local function select_by_version(ngx, base_name)
    local version = ngx.req.get_headers()["X-Version"]
    if version and version ~= "" then
        local ver_name = base_name .. "_" .. version
        ngx.log(ngx.DEBUG, "Route by X-Version header: ", ver_name)
        return ver_name
    end
    return nil
end

--- 根据路径前缀匹配 upstream
-- @param uri string 请求 URI
-- @param rules table 路由规则
-- @return string|nil 匹配的 upstream 名称
local function match_path(uri, rules)
    if not rules or not rules.path then
        return nil
    end

    for _, rule in ipairs(rules.path) do
        if rule.prefix and string.sub(uri, 1, #rule.prefix) == rule.prefix then
            ngx.log(ngx.DEBUG, "Route by path prefix '", rule.prefix, "': ", rule.upstream)
            return rule.upstream
        end
    end

    return nil
end

--- 选择目标 upstream
--
-- 按优先级依次尝试:
--   1. X-Service 头
--   2. 路径匹配 + X-Canary 灰度
--   3. 路径匹配 + X-Version 版本化
--   4. 路径匹配
--   5. 默认 upstream
--
-- @param ngx nginx 对象
-- @return string|nil 选中的 upstream 名称
function _M.select(ngx)
    local rules = _M.get_rules()
    if rules.error then
        ngx.log(ngx.ERR, "Failed to load routing rules: ", rules.error)
        return nil
    end

    local uri = ngx.var.uri

    -- 优先级 1: X-Service 头直接指定
    local service = select_by_service_header(ngx)
    if service then
        return select_canary(ngx, service) or service
    end

    -- 优先级 2: 路径匹配
    local base_upstream = match_path(uri, rules)
    if base_upstream then
        -- 检查灰度
        local canary = select_canary(ngx, base_upstream)
        if canary then
            return canary
        end

        -- 检查版本号
        local versioned = select_by_version(ngx, base_upstream)
        if versioned then
            return versioned
        end

        return base_upstream
    end

    -- 优先级 3: 默认回退
    if rules.default then
        ngx.log(ngx.DEBUG, "Route to default upstream: ", rules.default)
        return rules.default
    end

    ngx.log(ngx.WARN, "No upstream matched for URI: ", uri)
    return nil
end

return _M
