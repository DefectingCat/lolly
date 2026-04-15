// Package config 提供配置加载和管理的测试。
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad 测试从文件加载配置。
func TestLoad(t *testing.T) {
	t.Run("有效配置文件", func(t *testing.T) {
		// 创建临时配置文件
		content := `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www"
        index:
          - "index.html"
logging:
  access:
    path: "/var/log/access.log"
    format: "combined"
  error:
    path: "/var/log/error.log"
    level: "info"
performance:
  goroutine_pool:
    enabled: true
    max_workers: 100
  file_cache:
    max_entries: 1000
monitoring:
  status:
    path: "/status"
`
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "config.yaml")
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("创建临时配置文件失败: %v", err)
		}

		cfg, err := Load(tmpFile)
		if err != nil {
			t.Fatalf("Load() 失败: %v", err)
		}

		if cfg.Servers[0].Listen != ":8080" {
			t.Errorf("Servers[0].Listen = %q, want %q", cfg.Servers[0].Listen, ":8080")
		}
		if cfg.Servers[0].Static[0].Root != "/var/www" {
			t.Errorf("Servers[0].Static.Root = %q, want %q", cfg.Servers[0].Static[0].Root, "/var/www")
		}
		if len(cfg.Servers[0].Static[0].Index) != 1 || cfg.Servers[0].Static[0].Index[0] != "index.html" {
			t.Errorf("Servers[0].Static.Index = %v, want [index.html]", cfg.Servers[0].Static[0].Index)
		}
	})

	t.Run("文件不存在", func(t *testing.T) {
		_, err := Load("/nonexistent/path/config.yaml")
		if err == nil {
			t.Error("Load() 期望返回错误，但返回 nil")
		}
	})

	t.Run("无效YAML", func(t *testing.T) {
		content := `
server:
  listen: ":8080"
  static:
    root: [invalid yaml structure
`
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "invalid.yaml")
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("创建临时配置文件失败: %v", err)
		}

		_, err := Load(tmpFile)
		if err == nil {
			t.Error("Load() 期望返回错误，但返回 nil")
		}
	})

	t.Run("缺少必填字段（无服务器配置）", func(t *testing.T) {
		content := `
logging:
  access:
    path: "/var/log/access.log"
`
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "no_server.yaml")
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("创建临时配置文件失败: %v", err)
		}

		_, err := Load(tmpFile)
		if err == nil {
			t.Error("Load() 期望返回错误，但返回 nil")
		}
	})

	t.Run("多虚拟主机模式", func(t *testing.T) {
		content := `
servers:
  - listen: ":8080"
    name: "server1"
  - listen: ":8081"
    name: "server2"
`
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "multi.yaml")
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("创建临时配置文件失败: %v", err)
		}

		cfg, err := Load(tmpFile)
		if err != nil {
			t.Fatalf("Load() 失败: %v", err)
		}

		if len(cfg.Servers) != 2 {
			t.Fatalf("len(Servers) = %d, want 2", len(cfg.Servers))
		}
		if cfg.Servers[0].Name != "server1" {
			t.Errorf("Servers[0].Name = %q, want %q", cfg.Servers[0].Name, "server1")
		}
		if cfg.Servers[1].Name != "server2" {
			t.Errorf("Servers[1].Name = %q, want %q", cfg.Servers[1].Name, "server2")
		}
	})
}

// TestLoadFromString 测试从字符串加载配置。
func TestLoadFromString(t *testing.T) {
	t.Run("有效字符串", func(t *testing.T) {
		yamlStr := `
servers:
  - listen: ":9090"
    static:
      - path: "/"
        root: "/app/public"
`
		cfg, err := LoadFromString(yamlStr)
		if err != nil {
			t.Fatalf("LoadFromString() 失败: %v", err)
		}

		if cfg.Servers[0].Listen != ":9090" {
			t.Errorf("Servers[0].Listen = %q, want %q", cfg.Servers[0].Listen, ":9090")
		}
		if cfg.Servers[0].Static[0].Root != "/app/public" {
			t.Errorf("Servers[0].Static.Root = %q, want %q", cfg.Servers[0].Static[0].Root, "/app/public")
		}
	})

	t.Run("无效YAML", func(t *testing.T) {
		yamlStr := `
servers:
  - listen: ":8080"
    broken: [unclosed
`
		_, err := LoadFromString(yamlStr)
		if err == nil {
			t.Error("LoadFromString() 期望返回错误，但返回 nil")
		}
	})

	t.Run("缺少必填字段", func(t *testing.T) {
		yamlStr := `
logging:
  access:
    path: "/var/log/access.log"
`
		_, err := LoadFromString(yamlStr)
		if err == nil {
			t.Error("LoadFromString() 期望返回错误，但返回 nil")
		}
	})

	t.Run("空字符串", func(t *testing.T) {
		_, err := LoadFromString("")
		if err == nil {
			t.Error("LoadFromString() 期望返回错误，但返回 nil")
		}
	})
}

// TestSave 测试保存配置到文件。
func TestSave(t *testing.T) {
	t.Run("正常保存", func(t *testing.T) {
		cfg := &Config{
			Servers: []ServerConfig{{
				Listen: ":8080",
				Static: []StaticConfig{{
					Path:  "/",
					Root:  "/var/www",
					Index: []string{"index.html"},
				}},
			}},
		}

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "saved_config.yaml")

		if err := Save(cfg, tmpFile); err != nil {
			t.Fatalf("Save() 失败: %v", err)
		}

		// 验证文件已创建并可重新加载
		loaded, err := Load(tmpFile)
		if err != nil {
			t.Fatalf("重新加载配置失败: %v", err)
		}

		if loaded.Servers[0].Listen != cfg.Servers[0].Listen {
			t.Errorf("loaded.Servers[0].Listen = %q, want %q", loaded.Servers[0].Listen, cfg.Servers[0].Listen)
		}
		if loaded.Servers[0].Static[0].Root != cfg.Servers[0].Static[0].Root {
			t.Errorf("loaded.Servers[0].Static[0].Root = %q, want %q", loaded.Servers[0].Static[0].Root, cfg.Servers[0].Static[0].Root)
		}
	})

	t.Run("无效路径", func(t *testing.T) {
		cfg := &Config{
			Servers: []ServerConfig{{
				Listen: ":8080",
			}},
		}

		err := Save(cfg, "/nonexistent/directory/config.yaml")
		if err == nil {
			t.Error("Save() 期望返回错误，但返回 nil")
		}
	})

	t.Run("保存多虚拟主机配置", func(t *testing.T) {
		cfg := &Config{
			Servers: []ServerConfig{
				{Listen: ":8080", Name: "server1"},
				{Listen: ":8081", Name: "server2"},
			},
		}

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "multi.yaml")

		if err := Save(cfg, tmpFile); err != nil {
			t.Fatalf("Save() 失败: %v", err)
		}

		loaded, err := Load(tmpFile)
		if err != nil {
			t.Fatalf("重新加载配置失败: %v", err)
		}

		if len(loaded.Servers) != 2 {
			t.Errorf("len(loaded.Servers) = %d, want 2", len(loaded.Servers))
		}
	})

	t.Run("保存并加载完整配置", func(t *testing.T) {
		cfg := &Config{
			Servers: []ServerConfig{{
				Listen: ":8443",
				Name:   "default",
				Static: []StaticConfig{{
					Path:  "/",
					Root:  "/var/www/html",
					Index: []string{"index.html", "index.htm"},
				}},
				Proxy: []ProxyConfig{
					{
						Path: "/api",
						Targets: []ProxyTarget{
							{URL: "http://backend1:8080", Weight: 1},
							{URL: "http://backend2:8080", Weight: 2},
						},
						LoadBalance: "round_robin",
					},
				},
				SSL: SSLConfig{
					Cert:      "/etc/ssl/cert.pem",
					Key:       "/etc/ssl/key.pem",
					Protocols: []string{"TLSv1.2", "TLSv1.3"},
				},
				Security: SecurityConfig{
					RateLimit: RateLimitConfig{
						RequestRate: 100,
						Burst:       200,
					},
				},
			}},
			Logging: LoggingConfig{
				Access: AccessLogConfig{
					Path:   "/var/log/access.log",
					Format: "combined",
				},
				Error: ErrorLogConfig{
					Path:  "/var/log/error.log",
					Level: "warn",
				},
			},
			Performance: PerformanceConfig{
				GoroutinePool: GoroutinePoolConfig{
					Enabled:    true,
					MaxWorkers: 1000,
					MinWorkers: 10,
				},
			},
		}

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "full.yaml")

		if err := Save(cfg, tmpFile); err != nil {
			t.Fatalf("Save() 失败: %v", err)
		}

		loaded, err := Load(tmpFile)
		if err != nil {
			t.Fatalf("重新加载配置失败: %v", err)
		}

		// 验证关键字段
		if loaded.Servers[0].Listen != cfg.Servers[0].Listen {
			t.Errorf("loaded.Servers[0].Listen = %q, want %q", loaded.Servers[0].Listen, cfg.Servers[0].Listen)
		}
		if len(loaded.Servers[0].Proxy) != 1 {
			t.Errorf("len(loaded.Servers[0].Proxy) = %d, want 1", len(loaded.Servers[0].Proxy))
		}
		if loaded.Servers[0].Proxy[0].LoadBalance != "round_robin" {
			t.Errorf("loaded.Servers[0].Proxy[0].LoadBalance = %q, want %q", loaded.Servers[0].Proxy[0].LoadBalance, "round_robin")
		}
	})
}

// TestConfigMethods 测试 Config 结构体的方法。
func TestConfigMethods(t *testing.T) {
	t.Run("HasServers_有服务器列表", func(t *testing.T) {
		cfg := &Config{
			Servers: []ServerConfig{
				{Listen: ":8080"},
			},
		}
		if !cfg.HasServers() {
			t.Error("HasServers() = false, want true")
		}
	})

	t.Run("HasServers_无服务器列表", func(t *testing.T) {
		cfg := &Config{}
		if cfg.HasServers() {
			t.Error("HasServers() = true, want false")
		}
	})

	t.Run("GetDefaultServerFromList_有默认服务器", func(t *testing.T) {
		cfg := &Config{
			Servers: []ServerConfig{
				{Listen: ":8080", Name: "api"},
				{Listen: ":8081", Name: "default", Default: true},
			},
		}
		server := cfg.GetDefaultServerFromList()
		if server == nil {
			t.Fatal("GetDefaultServerFromList() = nil, want non-nil")
		}
		if server.Listen != ":8081" {
			t.Errorf("server.Listen = %q, want %q", server.Listen, ":8081")
		}
	})

	t.Run("GetDefaultServerFromList_无默认服务器", func(t *testing.T) {
		cfg := &Config{
			Servers: []ServerConfig{
				{Listen: ":8080", Name: "api"},
			},
		}
		server := cfg.GetDefaultServerFromList()
		if server != nil {
			t.Errorf("GetDefaultServerFromList() = %v, want nil", server)
		}
	})

	t.Run("配置模式判断", func(t *testing.T) {
		tests := []struct {
			cfg            *Config
			name           string
			wantHasServers bool
		}{
			{
				name:           "仅多虚拟主机",
				cfg:            &Config{Servers: []ServerConfig{{Listen: ":8080"}}},
				wantHasServers: true,
			},
			{
				name: "混合模式",
				cfg: &Config{
					Servers: []ServerConfig{{Listen: ":8081"}},
				},
				wantHasServers: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := tt.cfg.HasServers(); got != tt.wantHasServers {
					t.Errorf("HasServers() = %v, want %v", got, tt.wantHasServers)
				}
			})
		}
	})
}
