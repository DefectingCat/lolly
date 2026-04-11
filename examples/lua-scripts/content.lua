-- content.lua - Content 阶段内容生成示例
-- 此脚本演示如何在 content 阶段生成响应内容

-- 检查是否有认证信息
local user_id = ngx.ctx.user_id
if not user_id then
    -- 未认证，返回错误
    ngx.say("Not authenticated")
    ngx.exit(401)
end

-- 生成响应内容
ngx.say("Hello, " .. user_id .. "!")
ngx.say("Request processed at: " .. ngx.now())

-- 设置响应头
ngx.resp.set_header("X-User-Id", user_id)
ngx.resp.set_header("X-Server", "lolly-lua")

-- 可以根据请求路径返回不同内容
local uri = ngx.var.uri
if uri == "/api/status" then
    ngx.say("Status: OK")
end

-- 正常完成处理
ngx.exit(200)