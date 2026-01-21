// +kubebuilder:object:generate=true
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// StorageProxyConfig holds the server configuration.
type StorageProxyConfig struct {
	// gRPC Server
	// +optional
	// +kubebuilder:default="0.0.0.0"
	GRPCAddr string `yaml:"grpc_addr" json:"grpcAddr"`
	// +optional
	// +kubebuilder:default=8080
	GRPCPort int    `yaml:"grpc_port" json:"grpcPort"`

	// HTTP Management API
	// +optional
	// +kubebuilder:default="0.0.0.0"
	HTTPAddr string `yaml:"http_addr" json:"httpAddr"`
	// +optional
	// +kubebuilder:default=8081
	HTTPPort int    `yaml:"http_port" json:"httpPort"`

	// Database
	// +optional
	DatabaseURL string `yaml:"database_url" json:"databaseUrl"`

	// JuiceFS defaults
	// +optional
	MetaURL        string `yaml:"meta_url" json:"metaUrl"`
	// +optional
	S3Bucket       string `yaml:"s3_bucket" json:"s3Bucket"`
	// +optional
	S3Region       string `yaml:"s3_region" json:"s3Region"`
	// +optional
	S3Endpoint     string `yaml:"s3_endpoint" json:"s3Endpoint"`
	// +optional
	S3AccessKey    string `yaml:"s3_access_key" json:"s3AccessKey"`
	// +optional
	S3SecretKey    string `yaml:"s3_secret_key" json:"s3SecretKey"`
	// +optional
	S3SessionToken string `yaml:"s3_session_token" json:"s3SessionToken"`

	// +optional
	// +kubebuilder:default="1G"
	DefaultCacheSize string `yaml:"default_cache_size" json:"defaultCacheSize"`
	// +optional
	// +kubebuilder:default="/var/lib/storage-proxy/cache"
	CacheDir         string `yaml:"cache_dir" json:"cacheDir"`
	// +optional
	DefaultClusterId string `yaml:"default_cluster_id" json:"defaultClusterId"`

	// Monitoring
	// +optional
	// +kubebuilder:default=true
	MetricsEnabled bool `yaml:"metrics_enabled" json:"metricsEnabled"`
	// +optional
	// +kubebuilder:default=9090
	MetricsPort    int  `yaml:"metrics_port" json:"metricsPort"`

	// Logging
	// +optional
	// +kubebuilder:default="info"
	LogLevel  string `yaml:"log_level" json:"logLevel"`
	// +optional
	// +kubebuilder:default=true
	AuditLog  bool   `yaml:"audit_log" json:"auditLog"`
	// +optional
	// +kubebuilder:default="/var/log/storage-proxy/audit.log"
	AuditFile string `yaml:"audit_file" json:"auditFile"`

	// Rate limiting
	// +optional
	// +kubebuilder:default=10000
	MaxOpsPerSecond   int   `yaml:"max_ops_per_second" json:"maxOpsPerSecond"`
	// +optional
	// +kubebuilder:default=1073741824
	MaxBytesPerSecond int64 `yaml:"max_bytes_per_second" json:"maxBytesPerSecond"`

	// Kubernetes
	// +optional
	KubeconfigPath string `yaml:"kubeconfig_path" json:"kubeconfigPath"` // Path to kubeconfig file (empty for in-cluster config)
}

// LoadStorageProxyConfig returns the storage-proxy configuration.
func LoadStorageProxyConfig() *StorageProxyConfig {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "/config/config.yaml"
	}

	cfg, err := loadStorageProxyConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config from %s: %v, using empty config\n", path, err)
		cfg = &StorageProxyConfig{}
	}
	return cfg
}

func loadStorageProxyConfig(path string) (*StorageProxyConfig, error) {
	cfg := &StorageProxyConfig{}
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	data = []byte(os.ExpandEnv(string(data)))

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration.
func (c *StorageProxyConfig) Validate() error {
	return nil
}

// ConfigError represents a configuration error.
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}
