-- shared_dict.lua - 共享字典示例
-- 此脚本演示 ngx.shared.DICT 的使用

-- 获取共享字典（需要在配置中预先定义）
local dict = ngx.shared.DICT("my_cache")

-- 设置值
local ok, err = dict:set("user_count", "100")
if not ok then
    ngx.log(ngx.ERR, "failed to set user_count: ", err)
end

-- 设置带 TTL 的值
ok, err = dict:set("session_token", "abc123", 3600)  -- 1 小时过期
if not ok then
    ngx.log(ngx.ERR, "failed to set session_token: ", err)
end

-- 获取值
local value, flags = dict:get("user_count")
ngx.say("user_count: ", value)

-- 自增计数器
local new_val, err = dict:incr("request_count", 1)
ngx.say("request_count: ", new_val)

-- 添加值（仅不存在时）
ok, err = dict:add("unique_key", "value")
if ok then
    ngx.say("unique_key added successfully")
else
    ngx.say("unique_key already exists")
end

-- 查看字典大小
local size = dict:size()
ngx.say("dict size: ", size)

-- 获取剩余容量
local free = dict:free_space()
ngx.say("free space: ", free)

ngx.say("Shared dict demo completed!")