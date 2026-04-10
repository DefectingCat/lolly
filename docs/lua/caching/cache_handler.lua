-- cache_handler.lua
--
-- 缓存处理逻辑：读取、写入、失效
-- 配合 nginx.conf 中的 set 指令使用:
--   set $cache_key "prefix:$request_uri"
--   set $cache_ttl 60
--
-- 在 location 块中使用:
--   content_by_lua_file /path/to/cache_handler.lua;

local cjson = require "cjson.safe"

local _M = {}

-- 默认 TTL（秒）
local DEFAULT_TTL = 60

--- 获取缓存数据
-- @param dict 共享字典对象
-- @param key 缓存键
-- @return data 解码后的数据, ttl 剩余秒数
function _M.get(dict, key)
    local value = dict:get(key)
    if not value then
        return nil, 0
    end
    local ttl = dict:ttl(key) or 0
    return cjson.decode(value), ttl
end

--- 设置缓存数据
-- @param dict 共享字典对象
-- @param key 缓存键
-- @param data 要缓存的数据（table）
-- @param ttl 过期时间（秒），默认 60
-- @return ok, err, forcible
function _M.set(dict, key, data, ttl)
    ttl = ttl or DEFAULT_TTL
    local json, err = cjson.encode(data)
    if not json then
        return nil, "encode failed: " .. tostring(err)
    end
    return dict:set(key, json, ttl)
end

--- 手动失效单个 key
-- @param dict 共享字典对象
-- @param key 缓存键
-- @return true
function _M.invalidate(dict, key)
    dict:delete(key)
    return true
end

--- 批量失效（按前缀匹配）
-- @param dict 共享字典对象
-- @param prefix key 前缀
-- @return count 清除数量
function _M.invalidate_prefix(dict, prefix)
    local keys = dict:get_keys(0)
    local count = 0
    for _, key in ipairs(keys) do
        if string.sub(key, 1, #prefix) == prefix then
            dict:delete(key)
            count = count + 1
        end
    end
    return count
end

--- 失效全部缓存
-- @param dict 共享字典对象
function _M.invalidate_all(dict)
    dict:flush_all()
    dict:flush_expired(0)
end

--- 获取缓存统计
-- @param dict 共享字典对象
-- @return stats table
function _M.stats(dict)
    local keys = dict:get_keys(0)
    return {
        capacity = dict:capacity(),
        free_space = dict:free_space(),
        key_count = #keys,
    }
end

--- 主入口：作为 content_by_lua_file 使用时调用
-- 从 nginx 变量读取 key/ttl，执行 cache-then-origin 流程
-- 外部数据通过 _M.fetch_data 回调提供
function _M.run(fetch_data)
    local cache = ngx.shared.response_cache

    local key = ngx.var.cache_key
    if not key then
        ngx.log(ngx.ERR, "cache_key variable not set")
        ngx.status = 500
        ngx.say(cjson.encode({error = "cache key not configured"}))
        return
    end

    local ttl = tonumber(ngx.var.cache_ttl) or DEFAULT_TTL

    -- 尝试缓存命中
    local cached, remaining = _M.get(cache, key)
    if cached then
        ngx.header["X-Cache"] = "HIT"
        ngx.header["X-Cache-TTL"] = tostring(remaining)
        ngx.header["Content-Type"] = "application/json"
        ngx.say(cjson.encode(cached))
        return
    end

    -- 缓存未命中，调用外部数据源
    local data, err = fetch_data()
    if not data then
        ngx.status = 502
        ngx.say(cjson.encode({error = err or "upstream error"}))
        return
    end

    -- 写入缓存
    local ok, set_err, forcible = _M.set(cache, key, data, ttl)
    if not ok then
        ngx.log(ngx.ERR, "cache set failed: ", tostring(set_err))
    end
    if forcible then
        ngx.log(ngx.WARN, "cache LRU eviction occurred")
    end

    ngx.header["X-Cache"] = "MISS"
    ngx.header["Content-Type"] = "application/json"
    ngx.say(cjson.encode(data))
end

return _M
