package loadbalance

import "time"

// StickyConfig 配置 Sticky 负载均衡的 Cookie 参数。
type StickyConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Name     string        `yaml:"name"`
	Expires  time.Duration `yaml:"expires"`
	Domain   string        `yaml:"domain"`
	Path     string        `yaml:"path"`
	Secure   bool          `yaml:"secure"`
	HttpOnly bool          `yaml:"http_only"`
	SameSite string        `yaml:"same_site"`
}

// DefaultStickyConfig 返回 Sticky 负载均衡的默认配置。
func DefaultStickyConfig() StickyConfig {
	return StickyConfig{
		Name:     "lolly_route",
		Expires:  time.Hour,
		Path:     "/",
		HttpOnly: true,
		SameSite: "Lax",
	}
}
