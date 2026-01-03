# Procd - 网络隔离设计规范

## 一、设计目标

在 **Procd层面** 实现动态网络隔离，无需与 K8s 交互，支持：
- **IP/CIDR 过滤**：精确控制出站流量目标 IP
- **域名过滤**：支持域名、通配符域名（*.example.com）
- **DNS 欺骗防护**：防火墙独立解析域名，不信任沙箱内 DNS
- **私有 IP 黑名单**：默认阻止访问内网 IP
- **动态配置**：运行时通过 API 修改网络策略
- **协议分离**：TCP 通过代理过滤，UDP/ICMP 直接过滤
- **暂停/恢复持久化**：策略在 pause/resume 后保持有效

---

## 二、架构设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Procd Network Architecture                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Host Namespace (Procd作为PID=1运行在Host)                                  │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                         NetworkManager                                 │  │
│   │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│   │  │  Firewall   │ │  TCPProxy   │ │  DNSResolver│ │ IPSet Mgmt  │     │  │
│   │  │ (nftables)  │ │  (SOCKS5)   │ │  (独立解析)  │ │  (动态更新)  │     │  │
│   │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                      iptables/nftables 规则                             │  │
│   │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│   │  │  PREROUTING (priority -150)                                      │  │  │
│   │  │   1. ESTABLISHED/RELATED → mark(0x1) + accept                    │  │  │
│   │  │   2. predefinedAllowSet → mark(0x1) + accept                      │  │  │
│   │  │   3. predefinedDenySet → DROP (私有IP黑名单)                      │  │  │
│   │  │   4. userAllowSet (非TCP) → mark(0x1) + accept                    │  │  │
│   │  │   5. userDenySet (非TCP) → DROP                                  │  │  │
│   │  │   6. TCP未标记 → REDIRECT to TCPProxy :<port>                    │  │  │
│   │  └─────────────────────────────────────────────────────────────────┘  │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                   Sandbox Network Namespace                          │  │
│   │  ┌─────────────────────────────────────────────────────────────────┐   │  │
│   │  │  veth_sb (sandbox内) ←──→ veth_host (host)                      │   │  │
│   │  │                                                                 │   │  │
│   │  │  ┌─────────────────────────────────────────────────────────┐   │   │  │
│   │  │  │              Main Container (用户进程)                   │   │   │  │
│   │  │  │   - 所有网络流量都经过veth对                             │   │   │  │
│   │  │  │   - TCP流量被重定向到TCPProxy进行域名过滤                 │   │   │  │
│   │  │  │   - UDP/ICMP等协议由nftables直接过滤                      │   │   │  │
│   │  │  └─────────────────────────────────────────────────────────┘   │   │  │
│   │  └─────────────────────────────────────────────────────────────────┘   │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、数据结构定义

### 3.1 网络策略结构

```go
// NetworkPolicy 网络策略 (运行时可修改)
type NetworkPolicy struct {
    // 策略模式
    Mode NetworkPolicyMode `json:"mode"`

    // 出站规则
    Egress *NetworkEgressPolicy `json:"egress,omitempty"`

    // 入站规则 (预留，目前一般不开放入站)
    Ingress *NetworkIngressPolicy `json:"ingress,omitempty"`

    // 最后更新时间
    UpdatedAt time.Time `json:"updated_at"`
}

type NetworkPolicyMode string

const (
    // NetworkModeAllowAll 允许所有出站流量 (不启用防火墙)
    NetworkModeAllowAll NetworkPolicyMode = "allow-all"

    // NetworkModeDenyAll 拒绝所有出站流量 (默认拒绝)
    NetworkModeDenyAll NetworkPolicyMode = "deny-all"

    // NetworkModeWhitelist 白名单模式 (仅允许明确指定的流量)
    NetworkModeWhitelist NetworkPolicyMode = "whitelist"
)

// NetworkEgressPolicy 出站策略
type NetworkEgressPolicy struct {
    // 允许的CIDR列表 (IP地址或CIDR块)
    AllowCIDRs []string `json:"allow_cidrs,omitempty"`

    // 允许的域名列表
    // 支持: "example.com", "*.example.com", "*"
    AllowDomains []string `json:"allow_domains,omitempty"`

    // 拒绝的CIDR列表 (优先级高于Allow)
    DenyCIDRs []string `json:"deny_cidrs,omitempty"`

    // TCP代理端口 (0表示不使用代理)
    // 非0时TCP流量会被重定向到此端口的SOCKS5代理
    TCPProxyPort int32 `json:"tcp_proxy_port,omitempty"`
}

// NetworkIngressPolicy 入站策略 (预留)
type NetworkIngressPolicy struct {
    // 允许的端口列表
    AllowPorts []int32 `json:"allow_ports,omitempty"`

    // 允许的源CIDR列表
    AllowSources []string `json:"allow_sources,omitempty"`
}
```

### 3.2 网络配置

```go
// NetworkConfig 网络配置
type NetworkConfig struct {
    // Sandbox ID
    SandboxID string

    // 网络段配置
    HostCIDR        string  // "192.168.127.0/24"
    HostIP          string  // "192.168.127.1"
    SandboxIP       string  // "192.168.127.2"

    // TCP代理配置
    TCPProxyPort    int32   // 默认 0 (不启用)
    EnableTCPProxy  bool

    // DNS配置
    DNServers       []string // ["8.8.8.8", "8.8.4.4"]

    // 预定义黑名单 (默认阻止的IP范围)
    DefaultDenyCIDRs []string
}
```

---

## 四、NetworkManager 实现

### 4.1 核心接口

```go
// NetworkManager 网络管理器 (Procd核心组件)
type NetworkManager struct {
    mu               sync.RWMutex

    // 网络命名空间
    sandboxNS       netns.NsHandle
    hostNS          netns.NsHandle

    // 网络设备
    vethHost         string  // veth_sb_host (host侧)
    vethSandbox      string  // veth_sb (sandbox内)

    // 防火墙
    firewall         *Firewall
    nftConn          *nftables.Conn

    // TCP代理
    tcpProxy         *TCPProxy
    tcpProxyPort     int32

    // DNS解析器 (独立解析，防止DNS欺骗)
    dnsResolver      *DNSResolver

    // 当前策略
    currentPolicy    *NetworkPolicy

    // 配置
    config           *NetworkConfig
}

// NewNetworkManager 创建网络管理器
func NewNetworkManager(config *NetworkConfig) (*NetworkManager, error) {
    nm := &NetworkManager{
        config:     config,
        tcpProxyPort: config.TCPProxyPort,
    }

    // 1. 创建网络命名空间
    if err := nm.createNetworkNamespace(); err != nil {
        return nil, fmt.Errorf("create network namespace: %w", err)
    }

    // 2. 创建veth对
    if err := nm.createVethPair(); err != nil {
        return nil, fmt.Errorf("create veth pair: %w", err)
    }

    // 3. 初始化防火墙
    firewall, err := NewFirewall(nm.vethHost, nm.config.DefaultDenyCIDRs)
    if err != nil {
        return nil, fmt.Errorf("create firewall: %w", err)
    }
    nm.firewall = firewall

    // 4. 初始化DNS解析器
    nm.dnsResolver = NewDNSResolver(config.DNServers)

    // 5. 设置默认策略 (allow-all)
    nm.currentPolicy = &NetworkPolicy{
        Mode:    NetworkModeAllowAll,
        Egress:  &NetworkEgressPolicy{},
        UpdatedAt: time.Now(),
    }

    return nm, nil
}

// SetupNetwork 配置网络 (Procd启动时调用一次)
func (nm *NetworkManager) SetupNetwork() error {
    nm.mu.Lock()
    defer nm.mu.Unlock()

    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    // 保存当前namespace
    hostNS, err := netns.Get()
    if err != nil {
        return err
    }
    nm.hostNS = hostNS
    defer hostNS.Close()

    // 1. 创建sandbox网络命名空间
    nm.sandboxNS, err = netns.NewNamed(fmt.Sprintf("sb-%s", nm.config.SandboxID))
    if err != nil {
        return err
    }

    // 2. 创建veth对
    if err := nm.createVethPair(); err != nil {
        return err
    }

    // 3. 配置路由和NAT
    if err := nm.configureRouting(); err != nil {
        return err
    }

    // 4. 初始化nftables规则
    if err := nm.firewall.Initialize(); err != nil {
        return err
    }

    return nil
}
```

### 4.2 Firewall 实现

```go
// Firewall 防火墙 (基于nftables)
type Firewall struct {
    conn              *nftables.Conn
    table             *nftables.Table
    filterChain       *nftables.Chain

    // IP集合 (使用nftables sets)
    predefinedAllowSet set.Set  // 预定义允许集合 (通常为空)
    predefinedDenySet  set.Set  // 预定义拒绝集合 (私有IP)

    userAllowSet       set.Set  // 用户定义允许集合
    userDenySet        set.Set  // 用户定义拒绝集合

    // TCP标记 (用于识别是否已被处理)
    allowedMark        uint32  // 0x1
    tcpMarkRule        *nftables.Rule  // TCP标记规则 (可动态添加/删除)

    vethInterface      string
}

// NewFirewall 创建防火墙
func NewFirewall(vethInterface string, defaultDenyCIDRs []string) (*Firewall, error) {
    conn, err := nftables.New(nftables.AsLasting)
    if err != nil {
        return nil, err
    }

    table := &nftables.Table{
        Name:   "sb0-firewall",
        Family: nftables.TableFamilyINet,
    }
    conn.AddTable(table)

    // 创建PREROUTING filter链
    filterChain := &nftables.Chain{
        Name:     "PREROUTE_FILTER",
        Table:    table,
        Type:     nftables.ChainTypeFilter,
        Hooknum:  nftables.ChainHookPrerouting,
        Priority: nftables.ChainPriorityRef(-150),
        Policy:   nftables.ChainPolicyAccept,
    }
    conn.AddChain(filterChain)

    // 创建IP集合
    predefinedAllowSet, _ := set.New(conn, table, "predef_allow", nftables.TypeIPAddr)
    predefinedDenySet, _ := set.New(conn, table, "predef_deny", nftables.TypeIPAddr)
    userAllowSet, _ := set.New(conn, table, "user_allow", nftables.TypeIPAddr)
    userDenySet, _ := set.New(conn, table, "user_deny", nftables.TypeIPAddr)

    fw := &Firewall{
        conn:               conn,
        table:              table,
        filterChain:        filterChain,
        predefinedAllowSet: predefinedAllowSet,
        predefinedDenySet:  predefinedDenySet,
        userAllowSet:       userAllowSet,
        userDenySet:        userDenySet,
        allowedMark:        0x1,
        vethInterface:      vethInterface,
    }

    // 初始化预定义黑名单
    if err := fw.predefinedDenySet.ClearAndAddElements(conn, defaultDenyCIDRs); err != nil {
        return nil, err
    }

    // 安装基础规则
    if err := fw.installBaseRules(); err != nil {
        return nil, err
    }

    return fw, nil
}

// UpdatePolicy 更新网络策略 (动态调用)
func (fw *Firewall) UpdatePolicy(policy *NetworkPolicy) error {
    // 1. 清空用户集合
    if err := fw.userAllowSet.Clear(fw.conn); err != nil {
        return err
    }
    if err := fw.userDenySet.Clear(fw.conn); err != nil {
        return err
    }

    // 2. 根据模式配置规则
    switch policy.Mode {
    case NetworkModeAllowAll:
        // 移除TCP标记规则 (所有TCP不经过代理)
        if err := fw.removeTCPMarkRule(); err != nil {
            return err
        }

    case NetworkModeDenyAll:
        // 添加TCP标记规则 (所有TCP需要代理，代理会拒绝所有)
        if err := fw.addTCPMarkRuleIfNeeded(); err != nil {
            return err
        }

    case NetworkModeWhitelist:
        // 配置允许集合
        if policy.Egress != nil {
            // 添加允许的CIDRs
            for _, cidr := range policy.Egress.AllowCIDRs {
                if err := fw.userAllowSet.AddElement(fw.conn, cidr); err != nil {
                    return err
                }
            }
        }
        // TCP需要代理 (用于域名过滤)
        if err := fw.addTCPMarkRuleIfNeeded(); err != nil {
            return err
        }
    }

    // 3. 配置拒绝集合 (优先级高于允许)
    if policy.Egress != nil {
        for _, cidr := range policy.Egress.DenyCIDRs {
            if err := fw.userDenySet.AddElement(fw.conn, cidr); err != nil {
                return err
            }
        }
    }

    return fw.conn.Flush()
}
```

### 4.3 TCPProxy 实现（域名过滤）

```go
// TCPProxy TCP代理 (SOCKS5协议)
type TCPProxy struct {
    listenAddr   string
    dnsResolver  *DNSResolver
    allowDomains *DomainMatcher  // 允许的域名列表

    // 连接跟踪
    connections  sync.Map
}

// DomainMatcher 域名匹配器
type DomainMatcher struct {
    mu       sync.RWMutex
    exact    map[string]bool      // 精确匹配: "example.com"
    wildcard []*glob.Glob         // 通配符: "*.example.com"
    allowAll bool                 // "*"
}

// NewDomainMatcher 创建域名匹配器
func NewDomainMatcher(domains []string) *DomainMatcher {
    dm := &DomainMatcher{
        exact: make(map[string]bool),
    }

    for _, d := range domains {
        d = strings.ToLower(strings.TrimSpace(d))

        if d == "*" {
            dm.allowAll = true
            continue
        }

        if strings.HasPrefix(d, "*.") {
            // 通配符域名
            g, _ := glob.Compile(d)
            dm.wildcard = append(dm.wildcard, g)
        } else {
            // 精确域名
            dm.exact[d] = true
        }
    }

    return dm
}

// TCP代理处理流程
func (p *TCPProxy) handleConnection(clientConn net.Conn) {
    // 1. 解析SOCKS5握手，获取目标地址
    targetAddr, err := p.parseSocks5Handshake(clientConn)
    if err != nil {
        clientConn.Close()
        return
    }

    // 2. 分离host和port
    host, port, _ := net.SplitHostPort(targetAddr)

    // 3. DNS欺骗防护：代理自己解析域名
    var targetIPs []string
    if ip := net.ParseIP(host); ip != nil {
        // 直接IP连接
        targetIPs = []string{host}
    } else {
        // 域名连接：代理自己DNS解析
        resolvedIPs, err := p.dnsResolver.Resolve(host)
        if err != nil {
            clientConn.Close()
            return
        }

        // 4. 域名白名单检查
        if !p.allowDomains.Match(host) {
            // 域名不在白名单，拒绝连接
            clientConn.Close()
            return
        }

        targetIPs = resolvedIPs
    }

    // 5. IP白名单检查 (在防火墙也已检查，这里再检查一遍)
    for _, ip := range targetIPs {
        if p.isIPAllowed(ip) {
            // 6. 建立到目标服务器的连接
            targetConn, err := net.Dial("tcp", net.JoinHostPort(ip, port))
            if err != nil {
                continue
            }

            // 7. 双向转发数据
            go p.relay(clientConn, targetConn)
            return
        }
    }

    // 所有IP都被拒绝
    clientConn.Close()
}

// isIPAllowed 检查IP是否被允许
func (p *TCPProxy) isIPAllowed(ipStr string) bool {
    ip := net.ParseIP(ipStr)

    // 检查是否是私有IP (默认拒绝)
    if isPrivateIP(ip) {
        return false
    }

    return true
}

// isPrivateIP 检查是否是私有IP
func isPrivateIP(ip net.IP) bool {
    privateRanges := []string{
        "10.0.0.0/8",
        "127.0.0.0/8",
        "169.254.0.0/16",
        "172.16.0.0/12",
        "192.168.0.0/16",
    }

    for _, cidr := range privateRanges {
        _, network, _ := net.ParseCIDR(cidr)
        if network.Contains(ip) {
            return true
        }
    }

    return false
}
```

### 4.4 DNSResolver 实现

```go
// DNSResolver 独立DNS解析器 (防止沙箱内DNS欺骗)
type DNSResolver struct {
    servers    []string
    cache      sync.Map  // 域名 -> []IP (带过期)
    cacheTTL   time.Duration
}

// NewDNSResolver 创建DNS解析器
func NewDNSResolver(servers []string) *DNSResolver {
    return &DNSResolver{
        servers:  servers,
        cacheTTL: 5 * time.Minute,
    }
}

// Resolve 解析域名 (独立于沙箱内DNS配置)
func (r *DNSResolver) Resolve(domain string) ([]string, error) {
    domain = strings.ToLower(domain)

    // 检查缓存
    if cached, ok := r.cache.Load(domain); ok {
        entry := cached.(*cacheEntry)
        if time.Since(entry.expiredAt) < r.cacheTTL {
            return entry.ips, nil
        }
        r.cache.Delete(domain)
    }

    // 执行DNS查询 (使用外部DNS服务器)
    var ips []string
    var lastErr error

    for _, server := range r.servers {
        resolver := &net.Resolver{
            PreferGo: true,
            Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
                d := net.Dialer{
                    Timeout: time.Second * 5,
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

    // 更新缓存
    r.cache.Store(domain, &cacheEntry{
        ips:       ips,
        expiredAt: time.Now().Add(r.cacheTTL),
    })

    return ips, nil
}

type cacheEntry struct {
    ips       []string
    expiredAt time.Time
}
```

---

## 五、HTTP API

### 5.1 获取当前网络策略

```http
GET /api/v1/network/policy

Response: 200 OK
{
    "mode": "whitelist",
    "egress": {
        "allow_cidrs": ["8.8.8.8", "1.1.1.0/24"],
        "allow_domains": ["google.com", "*.github.com"],
        "deny_cidrs": ["10.0.0.0/8"],
        "tcp_proxy_port": 1080
    },
    "updated_at": "2024-01-01T00:00:00Z"
}
```

### 5.2 更新网络策略

```http
PUT /api/v1/network/policy
Content-Type: application/json

{
    "mode": "whitelist",
    "egress": {
        "allow_cidrs": ["8.8.8.8"],
        "allow_domains": ["google.com", "*.github.com"]
    }
}

Response: 200 OK
{
    "mode": "whitelist",
    "egress": { ... },
    "updated_at": "2024-01-01T00:01:00Z"
}
```

### 5.3 重置为默认策略

```http
POST /api/v1/network/policy/reset

Response: 200 OK
{
    "mode": "allow-all",
    "egress": null,
    "updated_at": "2024-01-01T00:02:00Z"
}
```

### 5.4 添加/删除规则

```http
# 添加允许的CIDR
POST /api/v1/network/policy/allow/cidr
{
    "cidr": "8.8.8.8"
}

# 添加允许的域名
POST /api/v1/network/policy/allow/domain
{
    "domain": "google.com"
}

# 添加拒绝的CIDR
POST /api/v1/network/policy/deny/cidr
{
    "cidr": "10.0.0.0/8"
}
```

---

## 六、与 Manager 的交互

### 6.1 沙箱认领时更新策略

```go
// Manager认领沙箱时调用Procd API
func (s *SandboxService) claimIdlePod(ctx context.Context, template *crd.SandboxTemplate, pod *corev1.Pod, req *ClaimSandboxRequest) (*ClaimSandboxResponse, error) {
    // ... 更新Pod labels ...

    // 获取Procd地址
    procdAddr := s.buildProcdAddress(pod)

    // 如果有网络策略配置，调用Procd API更新
    if req.Config != nil && req.Config.Network != nil {
        if err := s.updateNetworkPolicy(ctx, procdAddr, req.Config.Network); err != nil {
            // 回滚...
            return nil, err
        }
    }

    return &ClaimSandboxResponse{...}, nil
}

func (s *SandboxService) updateNetworkPolicy(ctx context.Context, procdAddr string, network *NetworkOverride) error {
    // 调用Procd API更新网络策略
    url := fmt.Sprintf("http://%s/api/v1/network/policy", procdAddr)

    policy := &NetworkPolicy{
        Mode: NetworkModeWhitelist,
        Egress: &NetworkEgressPolicy{
            AllowCIDRs:   network.AllowedCIDRs,
            AllowDomains: network.AllowedDomains,
        },
    }

    resp, err := http.Put(url, policy)
    return err
}
```

---

## 七、与 E2B 功能对比

| 功能 | E2B | Sandbox0 (Procd层面) |
|------|-----|---------------------|
| **IP/CIDR过滤** | ✅ nftables | ✅ nftables |
| **域名过滤** | ✅ TCP代理 | ✅ TCP代理 |
| **通配符域名** | ✅ *.domain.com | ✅ *.domain.com |
| **DNS欺骗防护** | ✅ 独立DNS解析 | ✅ 独立DNS解析 |
| **私有IP黑名单** | ✅ 预定义拒绝集合 | ✅ 预定义拒绝集合 |
| **动态配置** | ✅ 运行时修改 | ✅ HTTP API |
| **暂停/恢复持久化** | ✅ | ✅ (策略保存在Procd内存) |
| **协议分离** | ✅ TCP代理/其他直接 | ✅ TCP代理/其他直接 |
| **冷启动延迟** | ⚠️ 需要分配网络slot | ✅ Pod预启动，策略动态更新 |

---

## 八、优势总结

1. **零冷启动延迟**：网络策略在Procd层面配置，Pod认领时无需与K8s交互
2. **动态配置**：通过HTTP API随时修改网络策略
3. **完整功能**：实现E2B所有网络隔离功能
4. **简单部署**：无需复杂的网络slot池管理
5. **K8s原生**：与ReplicaSet + Idle Pool完美配合
