// +kubebuilder:object:generate=true
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetdConfig holds netd configuration.
type NetdConfig struct {
	// LogLevel is the logging level (debug, info, warn, error)
	// +optional
	// +kubebuilder:default="info"
	LogLevel string `yaml:"log_level" json:"logLevel"`

	// MetricsPort is the port for Prometheus metrics
	// +optional
	// +kubebuilder:default=9090
	MetricsPort int `yaml:"metrics_port" json:"metricsPort"`

	// HealthPort is the port for health checks
	// +optional
	// +kubebuilder:default=8080
	HealthPort int `yaml:"health_port" json:"healthPort"`

	// NodeName is the name of the node this netd is running on
	// +optional
	// +kubebuilder:default="${NODE_NAME}"
	NodeName string `yaml:"node_name" json:"nodeName"`

	// Namespace is the namespace to watch for sandbox pods
	// +optional
	Namespace string `yaml:"namespace" json:"namespace"`

	// KubeConfig is the path to kubeconfig file (optional, uses in-cluster config if empty)
	// +optional
	KubeConfig string `yaml:"kube_config" json:"kubeConfig"`

	// ResyncPeriod is the period for informer resync
	// +optional
	// +kubebuilder:default="30s"
	ResyncPeriod metav1.Duration `yaml:"resync_period" json:"resyncPeriod"`

	// ProxyListenAddr is the address for the L7 proxy to listen on
	// +optional
	// +kubebuilder:default="0.0.0.0"
	ProxyListenAddr string `yaml:"proxy_listen_addr" json:"proxyListenAddr"`

	// ProxyHTTPPort is the port for HTTP proxy (redirect from port 80)
	// +optional
	// +kubebuilder:default=18080
	ProxyHTTPPort int `yaml:"proxy_http_port" json:"proxyHTTPPort"`

	// ProxyHTTPSPort is the port for HTTPS/TLS proxy (redirect from port 443)
	// +optional
	// +kubebuilder:default=18443
	ProxyHTTPSPort int `yaml:"proxy_https_port" json:"proxyHTTPSPort"`

	// DNSResolvers are the upstream DNS resolvers for the proxy
	// +optional
	// +kubebuilder:default={"8.8.8.8:53"}
	DNSResolvers []string `yaml:"dns_resolvers" json:"dnsResolvers"`

	// MetricsReportInterval is the interval for reporting metrics
	// +optional
	// +kubebuilder:default="10s"
	MetricsReportInterval metav1.Duration `yaml:"metrics_report_interval" json:"metricsReportInterval"`

	// FailClosed if true, blocks all traffic when netd is not ready
	// +optional
	// +kubebuilder:default=true
	FailClosed bool `yaml:"fail_closed" json:"failClosed"`

	// StorageProxyCIDR is the CIDR for storage-proxy (always allowed)
	// +optional
	StorageProxyCIDR string `yaml:"storage_proxy_cidr" json:"storageProxyCIDR"`

	// ClusterDNSCIDR is the CIDR for cluster DNS (always allowed for DNS)
	// +optional
	ClusterDNSCIDR string `yaml:"cluster_dns_cidr" json:"clusterDNSCIDR"`

	// InternalGatewayCIDR is the CIDR for internal-gateway (allowed for ingress to procd)
	// +optional
	InternalGatewayCIDR string `yaml:"internal_gateway_cidr" json:"internalGatewayCIDR"`

	// ProcdPort is the port procd listens on
	// +optional
	// +kubebuilder:default=49983
	ProcdPort int `yaml:"procd_port" json:"procdPort"`

	// UseEBPF enables eBPF-based bandwidth control (more efficient than tc htb)
	// +optional
	// +kubebuilder:default=true
	UseEBPF bool `yaml:"use_ebpf" json:"useEBPF"`

	// BPFFSPath is the path to bpf filesystem (usually /sys/fs/bpf)
	// +optional
	// +kubebuilder:default="/sys/fs/bpf"
	BPFFSPath string `yaml:"bpf_fs_path" json:"bpfFSPath"`

	// UseEDT enables Earliest Departure Time pacing for eBPF
	// +optional
	// +kubebuilder:default=true
	UseEDT bool `yaml:"use_edt" json:"useEDT"`
}

// LoadNetdConfig returns the netd configuration.
func LoadNetdConfig() *NetdConfig {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "/config/config.yaml"
	}

	cfg, err := loadNetdConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config from %s: %v, using empty config\n", path, err)
		cfg = &NetdConfig{}
	}
	return cfg
}

func loadNetdConfig(path string) (*NetdConfig, error) {
	cfg := &NetdConfig{}
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
