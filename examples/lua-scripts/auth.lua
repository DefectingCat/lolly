-- auth.lua - Access 阶段认证检查示例
-- 此脚本演示如何在 access 阶段进行简单的 token 认证

local auth_header = ngx.req.get_headers()["Authorization"]

if not auth_header then
    ngx.say("Missing Authorization header")
    ngx.exit(401)
end

-- 验证 token (示例：简单的 token 检查)
local token = auth_header
if string.sub(token, 1, 7) ~= "Bearer " then
    ngx.say("Invalid token format")
    ngx.exit(401)
end

local actual_token = string.sub(token, 8)

-- 这里可以连接数据库或调用认证服务验证 token
-- 示例：简单的 token 比较
if actual_token ~= "valid-token-123" then
    ngx.say("Invalid token")
    ngx.exit(403)
end

-- 认证成功，设置用户信息到上下文
ngx.ctx.user_id = "user-123"
ngx.ctx.auth_time = ngx.now()

-- 继续处理请求