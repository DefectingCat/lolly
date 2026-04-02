package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// 验证 Listen 默认值
	if cfg.Server.Listen != ":8080" {
		t.Errorf("Server.Listen 期望 :8080, 实际 %s", cfg.Server.Listen)
	}

	// 验证 SSL 默认版本
	if len(cfg.Server.SSL.Protocols) != 2 {
		t.Errorf("SSL.Protocols 期望 2 个版本, 实际 %d", len(cfg.Server.SSL.Protocols))
	}
	expectedProtocols := []string{"TLSv1.2", "TLSv1.3"}
	for i, proto := range cfg.Server.SSL.Protocols {
		if proto != expectedProtocols[i] {
			t.Errorf("SSL.Protocols[%d] 期望 %s, 实际 %s", i, expectedProtocols[i], proto)
		}
	}

	// 验证 HSTS 默认值
	if cfg.Server.SSL.HSTS.MaxAge != 31536000 {
		t.Errorf("HSTS.MaxAge 期望 31536000, 实际 %d", cfg.Server.SSL.HSTS.MaxAge)
	}
	if !cfg.Server.SSL.HSTS.IncludeSubDomains {
		t.Errorf("HSTS.IncludeSubDomains 期望 true, 实际 %v", cfg.Server.SSL.HSTS.IncludeSubDomains)
	}
	if cfg.Server.SSL.HSTS.Preload {
		t.Errorf("HSTS.Preload 期望 false, 实际 %v", cfg.Server.SSL.HSTS.Preload)
	}

	// 验证压缩默认值
	if cfg.Server.Compression.Type != "gzip" {
		t.Errorf("Compression.Type 期望 gzip, 实际 %s", cfg.Server.Compression.Type)
	}
	if cfg.Server.Compression.Level != 6 {
		t.Errorf("Compression.Level 期望 6, 实际 %d", cfg.Server.Compression.Level)
	}
	if cfg.Server.Compression.MinSize != 1024 {
		t.Errorf("Compression.MinSize 期望 1024, 实际 %d", cfg.Server.Compression.MinSize)
	}
	expectedTypes := []string{"text/html", "text/css", "text/javascript", "application/json", "application/javascript"}
	for i, ct := range cfg.Server.Compression.Types {
		if ct != expectedTypes[i] {
			t.Errorf("Compression.Types[%d] 期望 %s, 实际 %s", i, expectedTypes[i], ct)
		}
	}
}

func TestGenerateConfigYAML(t *testing.T) {
	cfg := DefaultConfig()

	yamlData, err := GenerateConfigYAML(cfg)
	if err != nil {
		t.Fatalf("GenerateConfigYAML 返回错误: %v", err)
	}

	// 验证输出非空
	if len(yamlData) == 0 {
		t.Error("GenerateConfigYAML 输出为空")
	}

	yamlStr := string(yamlData)

	// 验证包含注释
	if !strings.Contains(yamlStr, "#") {
		t.Error("YAML 输出未包含注释")
	}
	if !strings.Contains(yamlStr, "# Lolly 配置文件") {
		t.Error("YAML 输出未包含文件头注释")
	}
	if !strings.Contains(yamlStr, "# 服务器配置") {
		t.Error("YAML 输出未包含服务器配置注释")
	}

	// 验证可重新解析 - 使用 LoadFromString 解析生成的 YAML
	// 注意：GenerateConfigYAML 生成的 YAML 包含注释的示例配置（如 proxy、rewrite 等）
	// 这些是注释掉的示例，不会被解析。需要提取实际生效的部分进行验证。

	// 构建一个可解析的简化 YAML 进行验证
	simpleYAML, err := GenerateSimpleYAML(cfg)
	if err != nil {
		t.Fatalf("GenerateSimpleYAML 返回错误: %v", err)
	}

 parsedCfg, err := LoadFromString(string(simpleYAML))
	if err != nil {
		t.Fatalf("解析生成的 YAML 失败: %v", err)
	}

	// 验证配置一致性
	if parsedCfg.Server.Listen != cfg.Server.Listen {
		t.Errorf("解析后 Server.Listen 不一致: 期望 %s, 实际 %s", cfg.Server.Listen, parsedCfg.Server.Listen)
	}
	if parsedCfg.Server.Name != cfg.Server.Name {
		t.Errorf("解析后 Server.Name 不一致: 期望 %s, 实际 %s", cfg.Server.Name, parsedCfg.Server.Name)
	}
	if parsedCfg.Server.Compression.Type != cfg.Server.Compression.Type {
		t.Errorf("解析后 Compression.Type 不一致: 期望 %s, 实际 %s", cfg.Server.Compression.Type, parsedCfg.Server.Compression.Type)
	}
	if parsedCfg.Server.Compression.Level != cfg.Server.Compression.Level {
		t.Errorf("解析后 Compression.Level 不一致: 期望 %d, 实际 %d", cfg.Server.Compression.Level, parsedCfg.Server.Compression.Level)
	}

	// 验证性能配置一致性
	if parsedCfg.Performance.GoroutinePool.MaxWorkers != cfg.Performance.GoroutinePool.MaxWorkers {
		t.Errorf("解析后 GoroutinePool.MaxWorkers 不一致: 期望 %d, 实际 %d",
			cfg.Performance.GoroutinePool.MaxWorkers, parsedCfg.Performance.GoroutinePool.MaxWorkers)
	}
	if parsedCfg.Performance.FileCache.MaxEntries != cfg.Performance.FileCache.MaxEntries {
		t.Errorf("解析后 FileCache.MaxEntries 不一致: 期望 %d, 实际 %d",
			cfg.Performance.FileCache.MaxEntries, parsedCfg.Performance.FileCache.MaxEntries)
	}

	// 验证时间.Duration 字段正确解析
	if parsedCfg.Performance.GoroutinePool.IdleTimeout != cfg.Performance.GoroutinePool.IdleTimeout {
		t.Errorf("解析后 GoroutinePool.IdleTimeout 不一致: 期望 %v, 实际 %v",
			cfg.Performance.GoroutinePool.IdleTimeout, parsedCfg.Performance.GoroutinePool.IdleTimeout)
	}
	if parsedCfg.Performance.FileCache.Inactive != cfg.Performance.FileCache.Inactive {
		t.Errorf("解析后 FileCache.Inactive 不一致: 期望 %v, 实际 %v",
			cfg.Performance.FileCache.Inactive, parsedCfg.Performance.FileCache.Inactive)
	}
}

func TestGenerateSimpleYAML(t *testing.T) {
	cfg := DefaultConfig()

	yamlData, err := GenerateSimpleYAML(cfg)
	if err != nil {
		t.Fatalf("GenerateSimpleYAML 返回错误: %v", err)
	}

	if len(yamlData) == 0 {
		t.Error("GenerateSimpleYAML 输出为空")
	}

	// 验证不包含注释（简洁 YAML）
	yamlStr := string(yamlData)
	if strings.Contains(yamlStr, "# Lolly 配置文件") {
		t.Error("简洁 YAML 不应包含文件头注释")
	}
}

func TestDefaultConfigPerformance(t *testing.T) {
	cfg := DefaultConfig()

	// 验证 GoroutinePool 默认值
	if cfg.Performance.GoroutinePool.Enabled {
		t.Errorf("GoroutinePool.Enabled 期望 false, 实际 %v", cfg.Performance.GoroutinePool.Enabled)
	}
	if cfg.Performance.GoroutinePool.MaxWorkers != 1000 {
		t.Errorf("GoroutinePool.MaxWorkers 期望 1000, 实际 %d", cfg.Performance.GoroutinePool.MaxWorkers)
	}
	if cfg.Performance.GoroutinePool.MinWorkers != 10 {
		t.Errorf("GoroutinePool.MinWorkers 期望 10, 实际 %d", cfg.Performance.GoroutinePool.MinWorkers)
	}
	if cfg.Performance.GoroutinePool.IdleTimeout != 60*time.Second {
		t.Errorf("GoroutinePool.IdleTimeout 期望 60s, 实际 %v", cfg.Performance.GoroutinePool.IdleTimeout)
	}

	// 验证 FileCache 默认值
	if cfg.Performance.FileCache.MaxEntries != 10000 {
		t.Errorf("FileCache.MaxEntries 期望 10000, 实际 %d", cfg.Performance.FileCache.MaxEntries)
	}
	if cfg.Performance.FileCache.MaxSize != 256*1024*1024 {
		t.Errorf("FileCache.MaxSize 期望 256MB, 实际 %d", cfg.Performance.FileCache.MaxSize)
	}
	if cfg.Performance.FileCache.Inactive != 20*time.Second {
		t.Errorf("FileCache.Inactive 期望 20s, 实际 %v", cfg.Performance.FileCache.Inactive)
	}
	if !cfg.Performance.FileCache.LRUEviction {
		t.Errorf("FileCache.LRUEviction 期望 true, 实际 %v", cfg.Performance.FileCache.LRUEviction)
	}

	// 验证 Transport 默认值
	if cfg.Performance.Transport.MaxIdleConns != 100 {
		t.Errorf("Transport.MaxIdleConns 期望 100, 实际 %d", cfg.Performance.Transport.MaxIdleConns)
	}
	if cfg.Performance.Transport.MaxIdleConnsPerHost != 32 {
		t.Errorf("Transport.MaxIdleConnsPerHost 期望 32, 实际 %d", cfg.Performance.Transport.MaxIdleConnsPerHost)
	}
	if cfg.Performance.Transport.IdleConnTimeout != 90*time.Second {
		t.Errorf("Transport.IdleConnTimeout 期望 90s, 实际 %v", cfg.Performance.Transport.IdleConnTimeout)
	}
	if cfg.Performance.Transport.MaxConnsPerHost != 0 {
		t.Errorf("Transport.MaxConnsPerHost 期望 0 (不限制), 实际 %d", cfg.Performance.Transport.MaxConnsPerHost)
	}
}