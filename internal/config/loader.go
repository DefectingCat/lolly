package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const maxIncludeDepth = 10

// ConfigLoader 配置加载器
type ConfigLoader struct {
	loadedFiles map[string]bool // 所有已加载文件（用于跳过重复处理）
	stack       map[string]bool // 当前调用栈（用于 DAG 循环检测）
	baseDir     string
	depth       int
}

// NewConfigLoader 构造函数
func NewConfigLoader(mainConfigPath string) *ConfigLoader {
	absPath, err := filepath.Abs(mainConfigPath)
	if err != nil {
		absPath = mainConfigPath
	}

	return &ConfigLoader{
		baseDir:     filepath.Dir(absPath),
		loadedFiles: make(map[string]bool),
		stack:       make(map[string]bool),
		depth:       0,
	}
}

// Load 加载配置（含 DAG-safe 循环检测）
func (l *ConfigLoader) Load(path string) (*Config, error) {
	// 深度限制
	if l.depth > maxIncludeDepth {
		return nil, fmt.Errorf("include depth exceeds maximum (%d)", maxIncludeDepth)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path failed: %w", err)
	}

	// DAG-safe 循环检测
	// 使用 stack 检测真正的循环（当前调用栈中的文件）
	// 使用 loadedFiles 跳过已处理的文件（允许 DAG 共享子配置）
	if l.stack[absPath] {
		return nil, fmt.Errorf("circular include detected: '%s' is in current include chain", absPath)
	}

	// 如果文件已处理过，跳过（不报错）
	if l.loadedFiles[absPath] {
		return &Config{}, nil // 返回空配置，跳过重复处理
	}

	l.stack[absPath] = true       // 加入调用栈
	l.loadedFiles[absPath] = true // 标记已处理
	l.depth++

	// 加载文件
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml failed: %w", err)
	}

	// 处理 include
	for _, inc := range cfg.Include {
		files, err := l.expandGlob(inc.Path)
		if err != nil {
			return nil, err
		}

		for _, f := range files {
			subCfg, err := l.Load(f) // 递归
			if err != nil {
				return nil, fmt.Errorf("include %s: %w", f, err)
			}

			if err := l.merge(&cfg, subCfg, f); err != nil {
				return nil, err
			}
		}
	}

	// 清理调用栈
	delete(l.stack, absPath)
	l.depth--
	return &cfg, nil
}

// merge 合并配置
func (l *ConfigLoader) merge(dst, src *Config, _ string) error {
	// Server name collision（listen collision 由 validate.go 处理）
	for _, newServer := range src.Servers {
		for _, existing := range dst.Servers {
			if newServer.Name == existing.Name {
				return fmt.Errorf("server name collision: '%s'", newServer.Name)
			}
		}
	}
	dst.Servers = append(dst.Servers, src.Servers...)

	// Stream collision
	for _, newStream := range src.Stream {
		for _, existing := range dst.Stream {
			if newStream.Listen == existing.Listen {
				return fmt.Errorf("stream listen collision: '%s'", newStream.Listen)
			}
		}
	}
	dst.Stream = append(dst.Stream, src.Stream...)

	return nil
}

// expandGlob 展开 glob 模式
func (l *ConfigLoader) expandGlob(pattern string) ([]string, error) {
	absPattern := l.resolvePath(pattern)
	return filepath.Glob(absPattern)
}

// resolvePath 解析路径
func (l *ConfigLoader) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(l.baseDir, path)
}
