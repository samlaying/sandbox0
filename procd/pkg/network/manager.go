package network

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Manager manages network isolation for the sandbox.
type Manager struct {
	mu sync.RWMutex

	firewall      *Firewall
	tcpProxy      *TCPProxy
	dnsResolver   *DNSResolver
	currentPolicy *NetworkPolicy
	config        *Config
	logger        *zap.Logger
}

// NewManager creates a new network manager.
func NewManager(config *Config, logger *zap.Logger) (*Manager, error) {
	nm := &Manager{
		config: config,
		logger: logger,
	}

	// Initialize firewall
	firewall, err := NewFirewall(config.DefaultDenyCIDRs, logger)
	if err != nil {
		return nil, fmt.Errorf("create firewall: %w", err)
	}
	nm.firewall = firewall

	// Initialize DNS resolver
	nm.dnsResolver = NewDNSResolver(config.DNSServers)

	// Initialize TCP proxy if enabled
	if config.EnableTCPProxy {
		nm.tcpProxy = NewTCPProxy(config.TCPProxyPort, nm.dnsResolver, logger)
	}

	// Set default policy
	nm.currentPolicy = DefaultPolicy()

	return nm, nil
}

// Setup configures the network (called once at startup).
func (nm *Manager) Setup() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Initialize nftables rules
	if err := nm.firewall.Initialize(); err != nil {
		return fmt.Errorf("initialize firewall: %w", err)
	}

	nm.logger.Info("Network setup completed",
		zap.String("sandbox_id", nm.config.SandboxID),
	)

	return nil
}

// UpdatePolicy updates the network policy.
func (nm *Manager) UpdatePolicy(policy *NetworkPolicy) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Update firewall rules
	if err := nm.firewall.UpdatePolicy(policy); err != nil {
		return fmt.Errorf("update firewall: %w", err)
	}

	// Update TCP proxy whitelist if in whitelist mode
	if nm.tcpProxy != nil && policy.Mode == PolicyModeWhitelist && policy.Egress != nil {
		// Update domain whitelist
		nm.tcpProxy.SetAllowDomains(policy.Egress.AllowDomains)

		// Update IP whitelist
		allowIPs := NewIPNetSet()
		for _, cidr := range policy.Egress.AllowCIDRs {
			if err := allowIPs.Add(cidr); err != nil {
				nm.logger.Warn("Failed to add CIDR to allow list",
					zap.String("cidr", cidr),
					zap.Error(err),
				)
			}
		}
		nm.tcpProxy.SetAllowIPs(allowIPs)
	}

	// Save current policy
	nm.currentPolicy = policy
	nm.currentPolicy.UpdatedAt = time.Now()

	nm.logger.Info("Network policy updated",
		zap.String("mode", string(policy.Mode)),
	)

	return nil
}

// GetPolicy returns the current network policy.
func (nm *Manager) GetPolicy() *NetworkPolicy {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.currentPolicy
}

// ResetPolicy resets to default allow-all policy.
func (nm *Manager) ResetPolicy() error {
	return nm.UpdatePolicy(DefaultPolicy())
}

// AddAllowCIDR adds a CIDR to the allow list.
func (nm *Manager) AddAllowCIDR(cidr string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.currentPolicy.Egress == nil {
		nm.currentPolicy.Egress = &EgressPolicy{}
	}

	nm.currentPolicy.Egress.AllowCIDRs = append(nm.currentPolicy.Egress.AllowCIDRs, cidr)
	nm.currentPolicy.UpdatedAt = time.Now()

	return nm.firewall.UpdatePolicy(nm.currentPolicy)
}

// AddAllowDomain adds a domain to the allow list.
func (nm *Manager) AddAllowDomain(domain string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.currentPolicy.Egress == nil {
		nm.currentPolicy.Egress = &EgressPolicy{}
	}

	nm.currentPolicy.Egress.AllowDomains = append(nm.currentPolicy.Egress.AllowDomains, domain)
	nm.currentPolicy.UpdatedAt = time.Now()

	if nm.tcpProxy != nil {
		nm.tcpProxy.SetAllowDomains(nm.currentPolicy.Egress.AllowDomains)
	}

	return nil
}

// AddDenyCIDR adds a CIDR to the deny list.
func (nm *Manager) AddDenyCIDR(cidr string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.currentPolicy.Egress == nil {
		nm.currentPolicy.Egress = &EgressPolicy{}
	}

	nm.currentPolicy.Egress.DenyCIDRs = append(nm.currentPolicy.Egress.DenyCIDRs, cidr)
	nm.currentPolicy.UpdatedAt = time.Now()

	return nm.firewall.UpdatePolicy(nm.currentPolicy)
}

// StartTCPProxy starts the TCP proxy server.
func (nm *Manager) StartTCPProxy() error {
	if nm.tcpProxy == nil {
		return nil
	}

	return nm.tcpProxy.Start()
}

// StopTCPProxy stops the TCP proxy server.
func (nm *Manager) StopTCPProxy() error {
	if nm.tcpProxy == nil {
		return nil
	}

	return nm.tcpProxy.Stop()
}

// Shutdown shuts down the network manager.
func (nm *Manager) Shutdown() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.tcpProxy != nil {
		nm.tcpProxy.Stop()
	}

	if nm.firewall != nil {
		nm.firewall.Cleanup()
	}

	nm.logger.Info("Network manager shutdown completed")
	return nil
}
