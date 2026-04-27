// Package config 提供配置加载器测试。
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewConfigLoader 测试 ConfigLoader 构造函数。
func TestNewConfigLoader(t *testing.T) {
	t.Run("相对路径转换", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		loader := NewConfigLoader(configPath)
		if loader == nil {
			t.Fatal("NewConfigLoader() returned nil")
		}

		// 验证 baseDir 是配置文件所在目录
		expectedDir, _ := filepath.Abs(tmpDir)
		if loader.baseDir != expectedDir {
			t.Errorf("baseDir = %q, want %q", loader.baseDir, expectedDir)
		}
	})

	t.Run("绝对路径保持不变", func(t *testing.T) {
		absPath := "/etc/lolly/config.yaml"
		loader := NewConfigLoader(absPath)
		if loader == nil {
			t.Fatal("NewConfigLoader() returned nil")
		}

		if loader.baseDir != "/etc/lolly" {
			t.Errorf("baseDir = %q, want /etc/lolly", loader.baseDir)
		}
	})

	t.Run("初始化状态", func(t *testing.T) {
		loader := NewConfigLoader("config.yaml")

		if loader.loadedFiles == nil {
			t.Error("loadedFiles not initialized")
		}
		if loader.stack == nil {
			t.Error("stack not initialized")
		}
		if loader.depth != 0 {
			t.Errorf("depth = %d, want 0", loader.depth)
		}
	})
}

// TestConfigLoader_Load 测试配置加载。
func TestConfigLoader_Load(t *testing.T) {
	t.Run("单文件配置加载", func(t *testing.T) {
		content := `
servers:
  - listen: ":8080"
    name: "main"
    static:
      - path: "/"
        root: "/var/www"
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			t.Fatalf("写入配置文件失败: %v", err)
		}

		loader := NewConfigLoader(configPath)
		cfg, err := loader.Load(configPath)
		if err != nil {
			t.Fatalf("Load() 失败: %v", err)
		}

		if len(cfg.Servers) != 1 {
			t.Errorf("len(Servers) = %d, want 1", len(cfg.Servers))
		}
		if cfg.Servers[0].Listen != ":8080" {
			t.Errorf("Listen = %q, want :8080", cfg.Servers[0].Listen)
		}
	})

	t.Run("文件不存在", func(t *testing.T) {
		loader := NewConfigLoader("/nonexistent/config.yaml")
		_, err := loader.Load("/nonexistent/config.yaml")
		if err == nil {
			t.Error("Load() 期望返回错误，但返回 nil")
		}
	})

	t.Run("无效YAML", func(t *testing.T) {
		content := `servers: [invalid yaml`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid.yaml")
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			t.Fatalf("写入配置文件失败: %v", err)
		}

		loader := NewConfigLoader(configPath)
		_, err := loader.Load(configPath)
		if err == nil {
			t.Error("Load() 期望返回错误，但返回 nil")
		}
	})
}

// TestConfigLoader_Include 测试 include 指令。
func TestConfigLoader_Include(t *testing.T) {
	t.Run("多文件include合并", func(t *testing.T) {
		tmpDir := t.TempDir()

		// 主配置文件
		mainConfig := `
servers:
  - listen: ":8080"
    name: "main"
include:
  - path: "servers/*.yaml"
`
		mainPath := filepath.Join(tmpDir, "main.yaml")
		if err := os.WriteFile(mainPath, []byte(mainConfig), 0o644); err != nil {
			t.Fatalf("写入主配置文件失败: %v", err)
		}

		// 创建 servers 目录
		serversDir := filepath.Join(tmpDir, "servers")
		if err := os.Mkdir(serversDir, 0o755); err != nil {
			t.Fatalf("创建 servers 目录失败: %v", err)
		}

		// 子配置文件1
		server1 := `
servers:
  - listen: ":8081"
    name: "server1"
`
		if err := os.WriteFile(filepath.Join(serversDir, "server1.yaml"), []byte(server1), 0o644); err != nil {
			t.Fatalf("写入 server1.yaml 失败: %v", err)
		}

		// 子配置文件2
		server2 := `
servers:
  - listen: ":8082"
    name: "server2"
`
		if err := os.WriteFile(filepath.Join(serversDir, "server2.yaml"), []byte(server2), 0o644); err != nil {
			t.Fatalf("写入 server2.yaml 失败: %v", err)
		}

		loader := NewConfigLoader(mainPath)
		cfg, err := loader.Load(mainPath)
		if err != nil {
			t.Fatalf("Load() 失败: %v", err)
		}

		// 应该有3个server: main + server1 + server2
		if len(cfg.Servers) != 3 {
			t.Errorf("len(Servers) = %d, want 3", len(cfg.Servers))
		}
	})

	t.Run("循环引用检测", func(t *testing.T) {
		tmpDir := t.TempDir()

		// a.yaml includes b.yaml
		configA := `
servers:
  - listen: ":8080"
    name: "a"
include:
  - path: "b.yaml"
`
		// b.yaml includes a.yaml (循环)
		configB := `
servers:
  - listen: ":8081"
    name: "b"
include:
  - path: "a.yaml"
`
		if err := os.WriteFile(filepath.Join(tmpDir, "a.yaml"), []byte(configA), 0o644); err != nil {
			t.Fatalf("写入 a.yaml 失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "b.yaml"), []byte(configB), 0o644); err != nil {
			t.Fatalf("写入 b.yaml 失败: %v", err)
		}

		loader := NewConfigLoader(filepath.Join(tmpDir, "a.yaml"))
		_, err := loader.Load(filepath.Join(tmpDir, "a.yaml"))
		if err == nil {
			t.Error("Load() 期望返回循环引用错误，但返回 nil")
		}
	})

	t.Run("深度超限", func(t *testing.T) {
		tmpDir := t.TempDir()

		// 创建深度嵌套的配置文件链
		for i := range 12 {
			config := `
servers:
  - listen: ":8080"
`
			if i < 11 {
				config += `include:
  - path: "next.yaml"
`
			}
			filename := "config.yaml"
			if i > 0 {
				filename = "next.yaml"
			}
			// 每个层级创建子目录
			subDir := filepath.Join(tmpDir, "level"+string(rune('0'+i)))
			_ = os.Mkdir(subDir, 0o755)
			if i == 0 {
				if err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(config), 0o644); err != nil {
					t.Fatalf("写入配置文件失败: %v", err)
				}
			}
		}

		// 简化测试：直接测试深度限制
		loader := NewConfigLoader(filepath.Join(tmpDir, "config.yaml"))
		loader.depth = 11 // 超过 maxIncludeDepth (10)

		_, err := loader.Load(filepath.Join(tmpDir, "config.yaml"))
		if err == nil {
			t.Error("Load() 期望返回深度超限错误，但返回 nil")
		}
	})

	t.Run("DAG共享子配置", func(t *testing.T) {
		tmpDir := t.TempDir()

		// shared.yaml - 被多处引用
		shared := `
servers:
  - listen: ":9090"
    name: "shared"
`
		if err := os.WriteFile(filepath.Join(tmpDir, "shared.yaml"), []byte(shared), 0o644); err != nil {
			t.Fatalf("写入 shared.yaml 失败: %v", err)
		}

		// main.yaml - 引用 shared.yaml 两次（应该只处理一次）
		main := `
servers:
  - listen: ":8080"
    name: "main"
include:
  - path: "shared.yaml"
  - path: "shared.yaml"
`
		if err := os.WriteFile(filepath.Join(tmpDir, "main.yaml"), []byte(main), 0o644); err != nil {
			t.Fatalf("写入 main.yaml 失败: %v", err)
		}

		loader := NewConfigLoader(filepath.Join(tmpDir, "main.yaml"))
		cfg, err := loader.Load(filepath.Join(tmpDir, "main.yaml"))
		if err != nil {
			t.Fatalf("Load() 失败: %v", err)
		}

		// shared 应该只被处理一次
		sharedCount := 0
		for _, s := range cfg.Servers {
			if s.Name == "shared" {
				sharedCount++
			}
		}
		if sharedCount != 1 {
			t.Errorf("shared server count = %d, want 1", sharedCount)
		}
	})
}

// TestConfigLoader_Merge 测试配置合并。
func TestConfigLoader_Merge(t *testing.T) {
	t.Run("server name冲突", func(t *testing.T) {
		tmpDir := t.TempDir()

		main := `
servers:
  - listen: ":8080"
    name: "duplicate"
include:
  - path: "sub.yaml"
`
		sub := `
servers:
  - listen: ":8081"
    name: "duplicate"
`
		if err := os.WriteFile(filepath.Join(tmpDir, "main.yaml"), []byte(main), 0o644); err != nil {
			t.Fatalf("写入 main.yaml 失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "sub.yaml"), []byte(sub), 0o644); err != nil {
			t.Fatalf("写入 sub.yaml 失败: %v", err)
		}

		loader := NewConfigLoader(filepath.Join(tmpDir, "main.yaml"))
		_, err := loader.Load(filepath.Join(tmpDir, "main.yaml"))
		if err == nil {
			t.Error("Load() 期望返回 server name 冲突错误，但返回 nil")
		}
	})

	t.Run("stream listen冲突", func(t *testing.T) {
		tmpDir := t.TempDir()

		main := `
stream:
  - listen: "12345"
    proxy_pass: "backend:54321"
include:
  - path: "sub.yaml"
`
		sub := `
stream:
  - listen: "12345"
    proxy_pass: "backend2:54321"
`
		if err := os.WriteFile(filepath.Join(tmpDir, "main.yaml"), []byte(main), 0o644); err != nil {
			t.Fatalf("写入 main.yaml 失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "sub.yaml"), []byte(sub), 0o644); err != nil {
			t.Fatalf("写入 sub.yaml 失败: %v", err)
		}

		loader := NewConfigLoader(filepath.Join(tmpDir, "main.yaml"))
		_, err := loader.Load(filepath.Join(tmpDir, "main.yaml"))
		if err == nil {
			t.Error("Load() 期望返回 stream listen 冲突错误，但返回 nil")
		}
	})

	t.Run("正常合并", func(t *testing.T) {
		tmpDir := t.TempDir()

		main := `
servers:
  - listen: ":8080"
    name: "main"
include:
  - path: "sub.yaml"
`
		sub := `
servers:
  - listen: ":8081"
    name: "sub"
stream:
  - listen: "12345"
    proxy_pass: "backend:54321"
`
		if err := os.WriteFile(filepath.Join(tmpDir, "main.yaml"), []byte(main), 0o644); err != nil {
			t.Fatalf("写入 main.yaml 失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "sub.yaml"), []byte(sub), 0o644); err != nil {
			t.Fatalf("写入 sub.yaml 失败: %v", err)
		}

		loader := NewConfigLoader(filepath.Join(tmpDir, "main.yaml"))
		cfg, err := loader.Load(filepath.Join(tmpDir, "main.yaml"))
		if err != nil {
			t.Fatalf("Load() 失败: %v", err)
		}

		if len(cfg.Servers) != 2 {
			t.Errorf("len(Servers) = %d, want 2", len(cfg.Servers))
		}
		if len(cfg.Stream) != 1 {
			t.Errorf("len(Stream) = %d, want 1", len(cfg.Stream))
		}
	})
}

// TestConfigLoader_Glob 测试 glob 模式展开。
func TestConfigLoader_Glob(t *testing.T) {
	t.Run("glob模式匹配", func(t *testing.T) {
		tmpDir := t.TempDir()

		// 创建多个配置文件
		for i := range 3 {
			content := `
servers:
  - listen: ":8080"
`
			filename := filepath.Join(tmpDir, "server"+string(rune('0'+i+1))+".yaml")
			if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
				t.Fatalf("写入配置文件失败: %v", err)
			}
		}

		loader := NewConfigLoader(tmpDir)
		files, err := loader.expandGlob(filepath.Join(tmpDir, "server*.yaml"))
		if err != nil {
			t.Fatalf("expandGlob() 失败: %v", err)
		}

		if len(files) != 3 {
			t.Errorf("len(files) = %d, want 3", len(files))
		}
	})

	t.Run("无匹配文件", func(t *testing.T) {
		loader := NewConfigLoader("/tmp")
		files, err := loader.expandGlob("/nonexistent/*.yaml")
		if err != nil {
			t.Fatalf("expandGlob() 失败: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("len(files) = %d, want 0", len(files))
		}
	})
}

// TestConfigLoader_ResolvePath 测试路径解析。
func TestConfigLoader_ResolvePath(t *testing.T) {
	t.Run("相对路径解析", func(t *testing.T) {
		loader := NewConfigLoader("/etc/lolly/config.yaml")

		result := loader.resolvePath("servers/config.yaml")
		expected := "/etc/lolly/servers/config.yaml"
		if result != expected {
			t.Errorf("resolvePath() = %q, want %q", result, expected)
		}
	})

	t.Run("绝对路径保持不变", func(t *testing.T) {
		loader := NewConfigLoader("/etc/lolly/config.yaml")

		result := loader.resolvePath("/absolute/path.yaml")
		if result != "/absolute/path.yaml" {
			t.Errorf("resolvePath() = %q, want /absolute/path.yaml", result)
		}
	})
}
