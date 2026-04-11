-- log.lua - Log 阶段日志记录示例
-- 此脚本演示如何在 log 阶段记录请求信息

local log_data = {
    uri = ngx.var.uri,
    method = ngx.req.get_method(),
    status = ngx.resp.get_status(),
    user_id = ngx.ctx.user_id or "anonymous",
    auth_time = ngx.ctx.auth_time or 0,
    duration = ngx.now() - ngx.ctx.start_time
}

-- 输出日志信息（实际应用中可写入文件或发送到日志服务）
ngx.log(ngx.INFO, "Request completed: " ..
    log_data.method .. " " ..
    log_data.uri .. " " ..
    "status=" .. log_data.status .. " " ..
    "user=" .. log_data.user_id .. " " ..
    "duration=" .. log_data.duration .. "s")

-- 记录响应大小
local response_size = #ngx.resp.get_headers()
ngx.log(ngx.INFO, "Response headers count: " .. response_size)