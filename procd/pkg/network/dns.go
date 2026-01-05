package network

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

// DNSResolver performs independent DNS resolution to prevent DNS spoofing.
type DNSResolver struct {
	servers  []string
	cache    sync.Map
	cacheTTL time.Duration
}

type cacheEntry struct {
	ips       []string
	expiredAt time.Time
}

// NewDNSResolver creates a new DNS resolver.
func NewDNSResolver(servers []string) *DNSResolver {
	if len(servers) == 0 {
		servers = []string{"8.8.8.8", "8.8.4.4"}
	}

	return &DNSResolver{
		servers:  servers,
		cacheTTL: 5 * time.Minute,
	}
}

// Resolve resolves a domain name using configured DNS servers.
func (r *DNSResolver) Resolve(domain string) ([]string, error) {
	domain = strings.ToLower(domain)

	// Check cache
	if cached, ok := r.cache.Load(domain); ok {
		entry := cached.(*cacheEntry)
		if time.Now().Before(entry.expiredAt) {
			return entry.ips, nil
		}
		r.cache.Delete(domain)
	}

	// Perform DNS query using external DNS servers
	var ips []string
	var lastErr error

	for _, server := range r.servers {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: 5 * time.Second,
				}
				return d.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result, err := resolver.LookupIPAddr(ctx, domain)
		cancel()

		if err == nil {
			for _, ipAddr := range result {
				ips = append(ips, ipAddr.IP.String())
			}
			break
		}
		lastErr = err
	}

	if len(ips) == 0 {
		return nil, lastErr
	}

	// Update cache
	r.cache.Store(domain, &cacheEntry{
		ips:       ips,
		expiredAt: time.Now().Add(r.cacheTTL),
	})

	return ips, nil
}

// ClearCache clears the DNS cache.
func (r *DNSResolver) ClearCache() {
	r.cache = sync.Map{}
}

// SetCacheTTL sets the cache TTL.
func (r *DNSResolver) SetCacheTTL(ttl time.Duration) {
	r.cacheTTL = ttl
}
