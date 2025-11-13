package config

import (
	"fmt"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config represents the application configuration
type Config struct {
	Server                ServerConfig      `koanf:"server"`
	Cache                 CacheConfig       `koanf:"cache"`
	HealthCheck           HealthCheckConfig `koanf:"health_check"`
	Clusters              []ClusterConfig   `koanf:"clusters"`
	SkipUnhealthyClusters bool              `koanf:"skip_unhealthy_clusters"`
}

// ServerConfig represents HTTP server configuration
type ServerConfig struct {
	Addr         string        `koanf:"addr"`
	ReadTimeout  time.Duration `koanf:"read_timeout"`
	WriteTimeout time.Duration `koanf:"write_timeout"`
	BasePath     string        `koanf:"base_path"` // Optional base path for reverse proxy (e.g., "/dc-switcher")
}

// CacheConfig represents cache configuration
type CacheConfig struct {
	TTL time.Duration `koanf:"ttl"`
}

// HealthCheckConfig represents health check configuration for active region monitoring
type HealthCheckConfig struct {
	Enabled         bool          `koanf:"enabled"`
	Interval        time.Duration `koanf:"interval"`
	FailedThreshold int           `koanf:"failed_threshold"`
}

// ClusterConfig represents a single Nomad cluster configuration
type ClusterConfig struct {
	Name    string     `koanf:"name"`
	Region  string     `koanf:"region"`
	Address string     `koanf:"address"`
	TLS     *TLSConfig `koanf:"tls"`
}

// TLSConfig represents TLS configuration for Nomad client
type TLSConfig struct {
	CA   string `koanf:"ca"`
	Cert string `koanf:"cert"`
	Key  string `koanf:"key"`
}

// Load loads configuration from the specified file
func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	// Load YAML config
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Addr == "" {
		return fmt.Errorf("server.addr is required")
	}

	if len(c.Clusters) == 0 {
		return fmt.Errorf("at least one cluster must be configured")
	}

	for i, cluster := range c.Clusters {
		if cluster.Address == "" {
			return fmt.Errorf("cluster[%d].address is required", i)
		}
		// Name and Region are optional - they will be auto-detected from Nomad API if not specified
	}

	// Validate health check configuration
	if c.HealthCheck.Enabled {
		if c.HealthCheck.Interval <= 0 {
			return fmt.Errorf("health_check.interval must be positive when health check is enabled")
		}
		if c.HealthCheck.FailedThreshold <= 0 {
			return fmt.Errorf("health_check.failed_threshold must be positive when health check is enabled")
		}
	}

	return nil
}
