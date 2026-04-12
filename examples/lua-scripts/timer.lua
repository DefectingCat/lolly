-- timer.lua - 定时器示例
-- 此脚本演示 ngx.timer.at 的使用

-- 创建定时器回调函数
local function timer_callback()
    -- 注意：定时器回调在独立上下文中执行
    -- 不能直接访问请求相关 API
    ngx.log(ngx.INFO, "Timer executed!")
end

-- 创建 5 秒后执行的定时器
local handle, err = ngx.timer.at(5, timer_callback)
if handle then
    ngx.say("Timer created successfully")

    -- 查看活跃定时器数
    local count = ngx.timer.running_count()
    ngx.say("Active timers: ", count)
else
    ngx.say("Failed to create timer: ", err)
end

-- 创建带参数的定时器（简化版暂不支持参数传递）
local function param_callback()
    ngx.log(ngx.INFO, "Timer with params executed")
end

handle, err = ngx.timer.at(2, param_callback)
if handle then
    ngx.say("Timer with params created")
else
    ngx.say("Failed: ", err)
end

ngx.say("Timer demo completed!")