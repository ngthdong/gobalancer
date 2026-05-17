package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr string        `yaml:"listen_addr"`
	Mode       string        `yaml:"mode"`
	Balancer   string        `yaml:"balancer"`
	Backends   []string      `yaml:"backends"`
	Timeouts   TimeoutConfig `yaml:"timeouts"`
	Health     HealthConfig  `yaml:"health"`
	Log        LogConfig     `yaml:"log"`
	Retries    RetryConfig   `yaml:"retries"`
}

type RetryConfig struct {
	MaxAttempts  int           `yaml:"max_attempts"`
	RetryOn5xx   bool          `yaml:"retry_on_5xx"`
	TotalTimeout time.Duration `yaml:"total_timeout"`
}

type TimeoutConfig struct {
	Dial  time.Duration `yaml:"dial"`
	Read  time.Duration `yaml:"read"`
	Write time.Duration `yaml:"write"`
	Idle  time.Duration `yaml:"idle"`
}

type HealthConfig struct {
	Interval         time.Duration `yaml:"interval"`
	Timeout          time.Duration `yaml:"timeout"`
	Path             string        `yaml:"path"`
	FailureThreshold int           `yaml:"failure_threshold"`
	SuccessThreshold int           `yaml:"success_threshold"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Backends) == 0 {
		return fmt.Errorf("at least one backend required")
	}
	if c.Mode != "tcp" && c.Mode != "http" {
		return fmt.Errorf("mode must be 'tcp' or 'http', got %q", c.Mode)
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr required")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.Timeouts.Dial == 0 {
		c.Timeouts.Dial = 5 * time.Second
	}
	if c.Timeouts.Read == 0 {
		c.Timeouts.Read = 30 * time.Second
	}
	if c.Timeouts.Idle == 0 {
		c.Timeouts.Idle = 90 * time.Second
	}
	if c.Health.Interval == 0 {
		c.Health.Interval = 10 * time.Second
	}
	if c.Health.Timeout == 0 {
		c.Health.Timeout = 2 * time.Second
	}
	if c.Retries.MaxAttempts == 0 {
		c.Retries.MaxAttempts = 3
	}
	if c.Retries.TotalTimeout == 0 {
		c.Retries.TotalTimeout = 15 * time.Second
	}
	if c.Balancer == "" {
		c.Balancer = "round_robin"
	}
	if c.Health.Path == "" {
		c.Health.Path = "/"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "text"
	}
}
