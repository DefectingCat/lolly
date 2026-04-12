-- subrequest.lua - 子请求示例
-- 此脚本演示 ngx.location.capture 的使用

-- 简单子请求
local res = ngx.location.capture("/api/status")
ngx.say("Subrequest status: ", res.status)
ngx.say("Subrequest body: ", res.body)

-- 带 method 的子请求
res = ngx.location.capture("/api/users", {
    method = "POST",
    body = '{"name": "test"}'
})
ngx.say("POST status: ", res.status)

-- 带 headers 的子请求
res = ngx.location.capture("/api/check", {
    method = "GET",
    headers = {
        ["Authorization"] = "Bearer token123",
        ["X-Custom"] = "value"
    }
})
ngx.say("GET with headers status: ", res.status)

ngx.say("Subrequest demo completed!")