// +kubebuilder:object:generate=true
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SchedulerConfig holds all configuration for scheduler.
type SchedulerConfig struct {
	// Server configuration
	// +optional
	// +kubebuilder:default=8080
	HTTPPort int `yaml:"http_port" json:"httpPort"`
	// +optional
	// +kubebuilder:default="info"
	LogLevel string `yaml:"log_level" json:"logLevel"`

	// Database configuration
	// +optional
	DatabaseURL string `yaml:"database_url" json:"databaseUrl"`

	// Reconciler configuration
	// +optional
	// +kubebuilder:default="30s"
	ReconcileInterval metav1.Duration `yaml:"reconcile_interval" json:"reconcileInterval"`

	// Timeouts
	// +optional
	// +kubebuilder:default="30s"
	ShutdownTimeout metav1.Duration `yaml:"shutdown_timeout" json:"shutdownTimeout"`
}

// LoadSchedulerConfig returns the scheduler configuration.
func LoadSchedulerConfig() *SchedulerConfig {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "/config/config.yaml"
	}

	cfg, err := loadSchedulerConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config from %s: %v, using empty config\n", path, err)
		cfg = &SchedulerConfig{}
	}
	return cfg
}

func loadSchedulerConfig(path string) (*SchedulerConfig, error) {
	cfg := &SchedulerConfig{}
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
