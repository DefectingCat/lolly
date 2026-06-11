package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestHealthzHandler(t *testing.T) {
	t.Parallel()
	var ctx fasthttp.RequestCtx
	HealthzHandler(&ctx)
	assert.Equal(t, 200, ctx.Response.StatusCode())
	assert.Equal(t, "application/json", string(ctx.Response.Header.ContentType()))
	assert.Equal(t, `{"status":"ok"}`, string(ctx.Response.Body()))
}

func TestHealthzHandler_ValidJSON(t *testing.T) {
	t.Parallel()
	var ctx fasthttp.RequestCtx
	HealthzHandler(&ctx)
	var result map[string]string
	err := json.Unmarshal(ctx.Response.Body(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "ok", result["status"])
}

func TestReadyzHandler_Ready(t *testing.T) {
	t.Parallel()
	handler := NewReadyzHandler(func() (bool, []string) {
		return true, nil
	})
	var ctx fasthttp.RequestCtx
	handler(&ctx)
	assert.Equal(t, 200, ctx.Response.StatusCode())
	assert.Equal(t, `{"status":"ready"}`, string(ctx.Response.Body()))
}

func TestReadyzHandler_NotReady(t *testing.T) {
	t.Parallel()
	handler := NewReadyzHandler(func() (bool, []string) {
		return false, []string{"test reason"}
	})
	var ctx fasthttp.RequestCtx
	handler(&ctx)
	assert.Equal(t, 503, ctx.Response.StatusCode())
	assert.Equal(t, `{"status":"not ready","reasons":["test reason"]}`, string(ctx.Response.Body()))
}

func TestReadyzHandler_NotReady_NoReasons(t *testing.T) {
	t.Parallel()
	handler := NewReadyzHandler(func() (bool, []string) {
		return false, nil
	})
	var ctx fasthttp.RequestCtx
	handler(&ctx)
	assert.Equal(t, 503, ctx.Response.StatusCode())
	assert.Equal(t, `{"status":"not ready"}`, string(ctx.Response.Body()))
}

func TestBuildReasonsJSON_MultipleReasons(t *testing.T) {
	t.Parallel()
	result := buildReasonsJSON([]string{"reason A", "reason B", "reason C"})
	var parsed map[string]any
	err := json.Unmarshal([]byte(result), &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "not ready", parsed["status"])
	reasons, ok := parsed["reasons"].([]any)
	assert.True(t, ok)
	assert.Equal(t, 3, len(reasons))
	assert.Equal(t, "reason A", reasons[0])
	assert.Equal(t, "reason B", reasons[1])
	assert.Equal(t, "reason C", reasons[2])
}
