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
	Etcd                  EtcdConfig        `koanf:"etcd"`
	Heartbeat             HeartbeatConfig   `koanf:"heartbeat"`
	MyDatacenter          string            `koanf:"my_datacenter"`          // Name of the local datacenter this instance manages
	ClusterRetryInterval  time.Duration     `koanf:"cluster_retry_interval"` // How often to retry unavailable clusters
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

// EtcdConfig represents etcd cluster configuration for distributed state
type EtcdConfig struct {
	Endpoints   []string      `koanf:"endpoints"`
	DialTimeout time.Duration `koanf:"dial_timeout"`
	Username    string        `koanf:"username"`
	Password    string        `koanf:"password"`
	TLS         *TLSConfig    `koanf:"tls"`
}

// HeartbeatConfig represents heartbeat configuration for split-brain protection
type HeartbeatConfig struct {
	UpdateInterval time.Duration `koanf:"update_interval"` // How often to update heartbeat in etcd
	MaxFailures    int           `koanf:"max_failures"`    // Number of consecutive failures before draining nodes
	StaleThreshold time.Duration `koanf:"stale_threshold"` // Age after which heartbeat is considered stale
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

	// Validate my_datacenter
	if c.MyDatacenter == "" {
		return fmt.Errorf("my_datacenter is required")
	}

	// Validate etcd configuration
	if len(c.Etcd.Endpoints) == 0 {
		return fmt.Errorf("etcd.endpoints is required")
	}
	if c.Etcd.DialTimeout <= 0 {
		c.Etcd.DialTimeout = 5 * time.Second // Default
	}

	// Validate heartbeat configuration
	if c.Heartbeat.UpdateInterval <= 0 {
		c.Heartbeat.UpdateInterval = 30 * time.Second // Default
	}
	if c.Heartbeat.MaxFailures <= 0 {
		c.Heartbeat.MaxFailures = 3 // Default
	}
	if c.Heartbeat.StaleThreshold <= 0 {
		c.Heartbeat.StaleThreshold = 2 * time.Minute // Default
	}

	// Validate cluster retry interval
	if c.ClusterRetryInterval <= 0 {
		c.ClusterRetryInterval = 5 * time.Minute // Default: retry every 5 minutes
	}

	return nil
}
