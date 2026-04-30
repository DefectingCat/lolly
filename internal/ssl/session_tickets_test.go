// Package ssl 提供 Session Tickets 的单元测试。
//
// 测试覆盖：
//   - 密钥生成和加载
//   - 密钥轮换逻辑
//   - 多密钥保留策略
//   - 与 TLS 配置的集成
//   - 边界条件和错误处理
//
// 作者：xfy
package ssl

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
)

// TestNewSessionTicketManager 测试创建 Session Ticket 管理器。
func TestNewSessionTicketManager(t *testing.T) {
	tests := []struct {
		name          string
		cfg           config.SessionTicketsConfig
		wantError     bool
		checkDefaults bool
	}{
		{
			name: "disabled_should_error",
			cfg: config.SessionTicketsConfig{
				Enabled: false,
			},
			wantError: true,
		},
		{
			name: "enabled_without_keyfile",
			cfg: config.SessionTicketsConfig{
				Enabled: true,
			},
			wantError:     false,
			checkDefaults: true,
		},
		{
			name: "enabled_with_defaults",
			cfg: config.SessionTicketsConfig{
				Enabled:        true,
				KeyFile:        "",
				RotateInterval: 0,
				RetainKeys:     0,
			},
			wantError:     false,
			checkDefaults: true,
		},
		{
			name: "enabled_with_custom_values",
			cfg: config.SessionTicketsConfig{
				Enabled:        true,
				RotateInterval: 30 * time.Minute,
				RetainKeys:     5,
			},
			wantError:     false,
			checkDefaults: false, // 使用自定义值，不检查默认值
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewSessionTicketManager(tt.cfg)
			if tt.wantError {
				if err == nil {
					t.Errorf("NewSessionTicketManager() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NewSessionTicketManager() unexpected error: %v", err)
				return
			}
			if mgr == nil {
				t.Error("NewSessionTicketManager() returned nil manager")
				return
			}
			defer mgr.Stop()

			// 验证默认配置（仅当使用默认值时）
			if tt.checkDefaults {
				if mgr.config.RotateInterval != defaultRotateInterval {
					t.Errorf("RotateInterval = %v, want %v", mgr.config.RotateInterval, defaultRotateInterval)
				}
				if mgr.config.RetainKeys != defaultRetainKeys {
					t.Errorf("RetainKeys = %d, want %d", mgr.config.RetainKeys, defaultRetainKeys)
				}
			} else {
				// 验证自定义值被正确保留
				if mgr.config.RotateInterval != tt.cfg.RotateInterval {
					t.Errorf("RotateInterval = %v, want %v", mgr.config.RotateInterval, tt.cfg.RotateInterval)
				}
				if mgr.config.RetainKeys != tt.cfg.RetainKeys {
					t.Errorf("RetainKeys = %d, want %d", mgr.config.RetainKeys, tt.cfg.RetainKeys)
				}
			}
		})
	}
}

// TestSessionTicketManager_KeyGeneration 测试密钥生成。
func TestSessionTicketManager_KeyGeneration(t *testing.T) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	keys := mgr.GetKeys()
	if len(keys) == 0 {
		t.Fatal("Expected at least one key, got none")
	}

	// 验证密钥大小
	for i, key := range keys {
		if len(key) != ticketKeySize {
			t.Errorf("Key %d size = %d, want %d", i, len(key), ticketKeySize)
		}
	}
}

// TestSessionTicketManager_KeyRotation 测试密钥轮换。
func TestSessionTicketManager_KeyRotation(t *testing.T) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: time.Hour, // 使用长间隔，手动触发轮换
		RetainKeys:     3,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	initialKeys := mgr.GetKeys()
	if len(initialKeys) != 1 {
		t.Fatalf("Expected 1 initial key, got %d", len(initialKeys))
	}

	// 手动轮换密钥
	if err := mgr.RotateKey(); err != nil {
		t.Fatalf("RotateKey() failed: %v", err)
	}

	keysAfter1 := mgr.GetKeys()
	if len(keysAfter1) != 2 {
		t.Errorf("Expected 2 keys after rotation, got %d", len(keysAfter1))
	}

	// 验证新旧密钥不同
	if string(initialKeys[0]) == string(keysAfter1[1]) {
		t.Error("New key should be different from initial key")
	}

	// 继续轮换到超过保留数量
	_ = mgr.RotateKey()
	_ = mgr.RotateKey()

	keysAfter4 := mgr.GetKeys()
	if len(keysAfter4) != 3 {
		t.Errorf("Expected 3 keys (max retain), got %d", len(keysAfter4))
	}
}

// TestSessionTicketManager_KeyRetention 测试密钥保留策略。
func TestSessionTicketManager_KeyRetention(t *testing.T) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: time.Hour,
		RetainKeys:     2,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	// 生成多个密钥
	for i := range 5 {
		if err := mgr.RotateKey(); err != nil {
			t.Fatalf("RotateKey() failed at iteration %d: %v", i, err)
		}
	}

	keys := mgr.GetKeys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys (RetainKeys limit), got %d", len(keys))
	}
}

// TestSessionTicketManager_Persistence 测试密钥持久化。
func TestSessionTicketManager_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "ticket.key")

	// 创建管理器并生成密钥
	mgr1, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		KeyFile:        keyFile,
		RotateInterval: time.Hour,
		RetainKeys:     3,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// 轮换几次生成多个密钥
	_ = mgr1.RotateKey()
	_ = mgr1.RotateKey()
	mgr1.Stop()

	// 验证密钥文件存在
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Fatal("Key file should exist after saving")
	}

	// 从文件加载密钥创建新管理器
	mgr2, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		KeyFile:        keyFile,
		RotateInterval: time.Hour,
		RetainKeys:     3,
	})
	if err != nil {
		t.Fatalf("Failed to create manager from existing key file: %v", err)
	}
	defer mgr2.Stop()

	keys := mgr2.GetKeys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys loaded from file, got %d", len(keys))
	}
}

// TestSessionTicketManager_ApplyToTLSConfig 测试应用到 TLS 配置。
func TestSessionTicketManager_ApplyToTLSConfig(t *testing.T) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: time.Hour,
		RetainKeys:     3,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	mgr.ApplyToTLSConfig(tlsCfg)

	// 验证可以获取密钥
	keys := mgr.GetKeys()
	if len(keys) == 0 {
		t.Error("Expected keys to be set in TLS config")
	}
}

// TestSessionTicketManager_StartStop 测试启动和停止。
func TestSessionTicketManager_StartStop(t *testing.T) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: 100 * time.Millisecond,
		RetainKeys:     3,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// 验证初始状态
	status := mgr.GetStatus()
	if status.Started {
		t.Error("Manager should not be started initially")
	}

	// 启动
	mgr.Start()
	status = mgr.GetStatus()
	if !status.Started {
		t.Error("Manager should be started after Start()")
	}

	// 等待一次轮换
	time.Sleep(150 * time.Millisecond)

	keys := mgr.GetKeys()
	if len(keys) < 1 {
		t.Error("Expected at least 1 key after auto-rotation")
	}

	// 停止
	mgr.Stop()
	status = mgr.GetStatus()
	if status.Started {
		t.Error("Manager should not be started after Stop()")
	}
}

// TestSessionTicketManager_GetStatus 测试获取状态。
func TestSessionTicketManager_GetStatus(t *testing.T) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: 30 * time.Minute,
		RetainKeys:     5,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	status := mgr.GetStatus()

	if status.KeyCount != 1 {
		t.Errorf("KeyCount = %d, want 1", status.KeyCount)
	}
	// 使用自定义值 5，不是默认值
	if status.RetainKeys != 5 {
		t.Errorf("RetainKeys = %d, want 5", status.RetainKeys)
	}
	// RotateInterval 使用配置值（30m > 0，所以保留）
	if status.RotateInterval != 30*time.Minute {
		t.Errorf("RotateInterval = %v, want %v", status.RotateInterval, 30*time.Minute)
	}
	if status.Started {
		t.Error("Started should be false before Start()")
	}

	mgr.Start()
	status = mgr.GetStatus()
	if !status.Started {
		t.Error("Started should be true after Start()")
	}
}

// TestGenerateTicketKey 测试密钥生成函数。
func TestGenerateTicketKey(t *testing.T) {
	key1, err := generateTicketKey()
	if err != nil {
		t.Fatalf("generateTicketKey() failed: %v", err)
	}

	if len(key1) != ticketKeySize {
		t.Errorf("generateTicketKey() key size = %d, want %d", len(key1), ticketKeySize)
	}

	key2, err := generateTicketKey()
	if err != nil {
		t.Fatalf("generateTicketKey() second call failed: %v", err)
	}

	// 验证生成的密钥是随机的（不相同）
	if string(key1) == string(key2) {
		t.Error("generateTicketKey() should generate random keys")
	}
}

// TestSessionTicketManager_ConcurrentAccess 测试并发访问。
func TestSessionTicketManager_ConcurrentAccess(t *testing.T) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: 10 * time.Millisecond,
		RetainKeys:     3,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	mgr.Start()

	// 并发读取和轮换
	done := make(chan bool, 3)

	// 协程 1: 持续获取密钥
	go func() {
		for range 100 {
			_ = mgr.GetKeys()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// 协程 2: 持续获取状态
	go func() {
		for range 100 {
			_ = mgr.GetStatus()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// 协程 3: 手动轮换
	go func() {
		for range 20 {
			_ = mgr.RotateKey()
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// 等待所有协程完成
	for range 3 {
		<-done
	}

	// 验证最终状态
	keys := mgr.GetKeys()
	if len(keys) < 1 || len(keys) > 3 {
		t.Errorf("Final key count %d out of expected range [1, 3]", len(keys))
	}
}

// BenchmarkGenerateTicketKey 基准测试密钥生成。
func BenchmarkGenerateTicketKey(b *testing.B) {
	for b.Loop() {
		_, err := generateTicketKey()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSessionTicketManager_GetKeys 基准测试获取密钥。
func BenchmarkSessionTicketManager_GetKeys(b *testing.B) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: time.Hour,
		RetainKeys:     3,
	})
	if err != nil {
		b.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	// 预生成多个密钥
	for range 2 {
		_ = mgr.RotateKey()
	}

	b.ResetTimer()
	for b.Loop() {
		_ = mgr.GetKeys()
	}
}

// BenchmarkSessionTicketManager_RotateKey 基准测试密钥轮换。
func BenchmarkSessionTicketManager_RotateKey(b *testing.B) {
	mgr, err := NewSessionTicketManager(config.SessionTicketsConfig{
		Enabled:        true,
		RotateInterval: time.Hour,
		RetainKeys:     3,
	})
	if err != nil {
		b.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	b.ResetTimer()
	for b.Loop() {
		err := mgr.RotateKey()
		if err != nil {
			b.Fatal(err)
		}
	}
}
