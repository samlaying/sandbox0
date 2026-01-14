package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for edge-gateway
type Config struct {
	// Server configuration
	HTTPPort int    `yaml:"http_port"`
	LogLevel string `yaml:"log_level"`

	// Database configuration (for API key validation)
	DatabaseURL string `yaml:"database_url"`

	// Upstream service
	InternalGatewayURL string `yaml:"internal_gateway_url"`

	// Authentication
	JWTSecret string `yaml:"jwt_secret"`

	// Internal authentication (for generating tokens to internal-gateway)
	InternalJWTPrivateKeyPath string `yaml:"internal_jwt_private_key_path"`

	// Rate limiting
	RateLimitRPS   int `yaml:"rate_limit_rps"`
	RateLimitBurst int `yaml:"rate_limit_burst"`

	// Timeouts
	ProxyTimeout    time.Duration `yaml:"proxy_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		HTTPPort:                  8080,
		LogLevel:                  "info",
		DatabaseURL:               "postgres://localhost:5432/sandbox0?sslmode=disable",
		InternalGatewayURL:        "http://internal-gateway.sandbox0-system:8443",
		JWTSecret:                 "",
		InternalJWTPrivateKeyPath: "/secrets/internal_jwt_private.key",
		RateLimitRPS:              100,
		RateLimitBurst:            200,
		ProxyTimeout:              30 * time.Second,
		ShutdownTimeout:           30 * time.Second,
	}
}

var Cfg *Config

func init() {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "/config/config.yaml"
	}

	var err error
	Cfg, err = load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config from %s: %v, using defaults\n", path, err)
		Cfg = DefaultConfig()
	}
}

// LoadConfig returns the global configuration
func LoadConfig() *Config {
	return Cfg
}

// load loads configuration from a YAML file
func load(path string) (*Config, error) {
	// Default configuration
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}
