package loadbalance

import "time"

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

func DefaultStickyConfig() StickyConfig {
	return StickyConfig{
		Name:     "lolly_route",
		Expires:  time.Hour,
		Path:     "/",
		HttpOnly: true,
		SameSite: "Lax",
	}
}
