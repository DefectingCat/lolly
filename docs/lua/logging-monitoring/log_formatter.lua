-- log_formatter.lua
-- 自定义 JSON 结构化日志格式化

local cjson = require "cjson.safe"

local M = {}

--- 将毫秒转换为秒
local function ms_to_s(ms)
    if not ms or ms == "-1" then
        return nil
    end
    return tonumber(ms) / 1000
end

--- 格式化日志并输出到 stderr（Nginx error log）
-- @param ngx_obj nginx 上下文对象
function M.log(ngx_obj)
    local entry = {
        time = ngx_obj.var.time_iso8601,
        remote_addr = ngx_obj.var.remote_addr,
        method = ngx_obj.var.request_method,
        uri = ngx_obj.var.uri,
        args = ngx_obj.var.args,
        status = ngx_obj.status,
        request_time = tonumber(ngx_obj.var.request_time) or 0,
        body_bytes_sent = tonumber(ngx_obj.var.body_bytes_sent) or 0,
        http_referer = ngx_obj.var.http_referer,
        http_user_agent = ngx_obj.var.http_user_agent,
        http_x_forwarded_for = ngx_obj.var.http_x_forwarded_for,
    }

    -- 上游响应时间
    local upstream_time = ngx_obj.var.upstream_response_time
    if upstream_time then
        entry.upstream_response_time = ms_to_s(upstream_time)
    end

    -- 请求 ID（如果有）
    local req_id = ngx_obj.var.request_id
    if req_id then
        entry.request_id = req_id
    end

    -- 移除 nil 字段
    local clean = {}
    for k, v in pairs(entry) do
        if v ~= nil then
            clean[k] = v
        end
    end

    local json, err = cjson.encode(clean)
    if json then
        ngx_obj.log(ngx_obj.ERR, "[access] ", json)
    else
        ngx_obj.log(ngx_obj.ERR, "[access] json encode failed: ", err)
    end
end

return M
