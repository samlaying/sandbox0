package network

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// Firewall manages nftables rules for network isolation.
// NOTE: This is a simplified implementation. In production, use
// github.com/google/nftables for actual nftables manipulation.
type Firewall struct {
	mu sync.Mutex

	initialized bool
	tableName   string

	// IP sets
	predefinedDenyCIDRs []string
	userAllowCIDRs      []string
	userDenyCIDRs       []string

	// TCP redirect config
	tcpRedirectPort    int32
	tcpRedirectEnabled bool

	logger *zap.Logger
}

// NewFirewall creates a new firewall.
func NewFirewall(defaultDenyCIDRs []string, logger *zap.Logger) (*Firewall, error) {
	return &Firewall{
		tableName:           "sb0-firewall",
		predefinedDenyCIDRs: defaultDenyCIDRs,
		logger:              logger,
	}, nil
}

// Initialize sets up the base nftables rules.
func (fw *Firewall) Initialize() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.initialized {
		return nil
	}

	// NOTE: In production, this would use github.com/google/nftables
	// to create actual nftables rules. Here we document the intended
	// behavior for the implementation.

	/*
	   Expected nftables configuration:

	   table inet sb0-firewall {
	       set predef_deny {
	           type ipv4_addr
	           flags interval
	           elements = { 10.0.0.0/8, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.168.0.0/16 }
	       }

	       set user_allow {
	           type ipv4_addr
	           flags interval
	       }

	       set user_deny {
	           type ipv4_addr
	           flags interval
	       }

	       chain SANDBOX0_OUTPUT {
	           type filter hook output priority filter; policy accept;

	           # Proxy bypass (highest priority) - packets marked 0x2 bypass all rules
	           meta mark & 0x2 == 0x2 accept

	           # Predefined blacklist (private IPs)
	           ip daddr @predef_deny drop

	           # User deny list (higher priority than allow)
	           ip daddr @user_deny drop

	           # User allow list check happens in TCP proxy
	       }
	   }
	*/

	fw.logger.Info("Firewall initialized",
		zap.String("table", fw.tableName),
		zap.Int("predefined_deny_count", len(fw.predefinedDenyCIDRs)),
	)

	fw.initialized = true
	return nil
}

// UpdatePolicy updates the firewall rules based on the policy.
func (fw *Firewall) UpdatePolicy(policy *NetworkPolicy) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if !fw.initialized {
		return fmt.Errorf("firewall not initialized")
	}

	// Clear user sets
	fw.userAllowCIDRs = nil
	fw.userDenyCIDRs = nil
	fw.tcpRedirectEnabled = false

	switch policy.Mode {
	case PolicyModeAllowAll:
		// No additional rules needed, default policy is accept
		fw.logger.Info("Firewall set to allow-all mode")

	case PolicyModeDenyAll:
		// Add rule to drop all traffic
		// In production: add nftables rule "drop" at end of chain
		fw.logger.Info("Firewall set to deny-all mode")

	case PolicyModeWhitelist:
		if policy.Egress != nil {
			// Set up allow CIDRs
			fw.userAllowCIDRs = policy.Egress.AllowCIDRs

			// Set up TCP redirect for domain filtering
			if policy.Egress.TCPProxyPort > 0 {
				fw.tcpRedirectPort = policy.Egress.TCPProxyPort
				fw.tcpRedirectEnabled = true

				/*
				   In production, add nftables redirect rule:

				   meta l4proto tcp tcp dport != {proxy_port} redirect to :proxy_port
				*/
			}
		}
		fw.logger.Info("Firewall set to whitelist mode",
			zap.Int("allow_cidrs", len(fw.userAllowCIDRs)),
			zap.Bool("tcp_redirect", fw.tcpRedirectEnabled),
		)
	}

	// Set up deny CIDRs (always applied, higher priority than allow)
	if policy.Egress != nil {
		fw.userDenyCIDRs = policy.Egress.DenyCIDRs
	}

	return nil
}

// Cleanup removes all firewall rules.
func (fw *Firewall) Cleanup() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// In production: delete the entire nftables table
	fw.initialized = false
	fw.logger.Info("Firewall cleaned up")

	return nil
}

// IsInitialized returns whether the firewall is initialized.
func (fw *Firewall) IsInitialized() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.initialized
}

// GetAllowCIDRs returns the current allow CIDRs.
func (fw *Firewall) GetAllowCIDRs() []string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.userAllowCIDRs
}

// GetDenyCIDRs returns the current deny CIDRs.
func (fw *Firewall) GetDenyCIDRs() []string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.userDenyCIDRs
}
