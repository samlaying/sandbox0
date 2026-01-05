# Sandbox0 Internal Gateway 设计规范

## 一、设计目标

Internal Gateway 是 sandbox0 的统一入口，负责：
1. **鉴权认证**：验证客户端身份，支持多种认证方式
2. **请求路由**：将请求转发到对应的内部服务（manager/procd）
3. **协议转换**：统一外部API协议，屏蔽内部服务差异
4. **流量控制**：限流、配额管理、熔断降级

---

## 二、架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     Internal Gateway Architecture                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        HTTP Server (Port: 8443)                       │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│  │  │   Sandbox   │ │   Process   │ │SandboxVolume│ │   Template  │     │  │
│  │  │   APIs      │ │   APIs      │ │   APIs      │ │   APIs      │     │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        Middleware Layer                               │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│  │  │    Auth     │ │   Rate      │ │   Request   │ │   Response  │     │  │
│  │  │  Middleware │ │   Limit     │ │   Logging   │ │  Tracing    │     │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│              ┌─────────────────────┼─────────────────────┬───────────────┐   │
│              ▼                     ▼                     ▼               │   │
│  ┌───────────────────┐ ┌───────────────────┐ ┌─────────────────────┐     │   │
│  │      Manager      │ │      Procd        │ │   Storage Proxy     │     │   │
│  │   (Port: 8080)    │ │   (Dynamic)       │ │   (Port: 8081)     │     │   │
│  │                   │ │                   │ │                     │     │   │
│  │  - Sandbox        │ │  - Process        │ │  - SandboxVolume   │     │   │
│  │    Management     │ │    Management     │ │    Management      │     │   │
│  │  - Template       │ │  - File System    │ │  - JuiceFS Storage │     │   │
│  │    Management     │ │  - Context        │ │  - Snapshot/Restore│     │   │
│  └───────────────────┘ └───────────────────┘ └─────────────────────┘     │   │
│                                                              │             │   │
│                                                              ▼             │   │
│                                                    ┌───────────────────┐   │   │
│                                                    │    PostgreSQL      │   │   │
│                                                    │  - API Keys       │   │   │
│                                                    │  - Teams/Users    │   │   │
│                                                    │  - Quotas         │   │   │
│                                                    │  - Audit Logs     │   │   │
│                                                    │  - SandboxVolume  │   │   │
│                                                    │    Metadata       │   │   │
│                                                    └───────────────────┘   │   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、API 路由表

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           API Routing Table                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Sandbox Management (→ Manager)                                     │   │
│  │  ├─ POST   /api/v1/sandboxes                          创建沙箱      │   │
│  │  ├─ GET    /api/v1/sandboxes                          列出沙箱      │   │
│  │  ├─ GET    /api/v1/sandboxes/{id}                     获取沙箱详情  │   │
│  │  ├─ GET    /api/v1/sandboxes/{id}/status              获取状态      │   │
│  │  ├─ PATCH  /api/v1/sandboxes/{id}                     更新配置      │   │
│  │  ├─ DELETE /api/v1/sandboxes/{id}                     删除沙箱      │   │
│  │  ├─ POST   /api/v1/sandboxes/{id}/pause               暂停沙箱      │   │
│  │  ├─ POST   /api/v1/sandboxes/{id}/resume              恢复沙箱      │   │
│  │  └─ POST   /api/v1/sandboxes/{id}/refresh             刷新TTL       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Process/Context Management (→ Procd)                               │   │
│  │  ├─ POST   /api/v1/sandboxes/{id}/contexts              创建上下文  │   │
│  │  ├─ GET    /api/v1/sandboxes/{id}/contexts              列出上下文  │   │
│  │  ├─ GET    /api/v1/sandboxes/{id}/contexts/{ctx_id}    获取上下文  │   │
│  │  ├─ DELETE /api/v1/sandboxes/{id}/contexts/{ctx_id}    删除上下文  │   │
│  │  ├─ POST   /api/v1/sandboxes/{id}/contexts/{ctx_id}/restart  重启 │   │
│  │  ├─ POST   /api/v1/sandboxes/{id}/contexts/{ctx_id}/execute   执行  │   │
│  │  └─ WS     /api/v1/sandboxes/{id}/contexts/{ctx_id}/ws        WebSocket│   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  File System (→ Procd)                                               │   │
│  │  ├─ GET    /api/v1/sandboxes/{id}/files/*                 读文件     │   │
│  │  ├─ POST   /api/v1/sandboxes/{id}/files/*                 写文件     │   │
│  │  ├─ GET    /api/v1/sandboxes/{id}/files/*?stat=true       文件信息   │   │
│  │  ├─ POST   /api/v1/sandboxes/{id}/files/*?mkdir=true       创建目录   │   │
│  │  ├─ POST   /api/v1/sandboxes/{id}/files/move              移动文件   │   │
│  │  ├─ DELETE /api/v1/sandboxes/{id}/files/*                 删除文件    │   │
│  │  ├─ GET    /api/v1/sandboxes/{id}/files?list=true         列出目录    │   │
│  │  └─ WS     /api/v1/sandboxes/{id}/files/watch             监听变化    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Template Management (→ Manager)                                     │   │
│  │  ├─ GET    /api/v1/templates                           列出模板     │   │
│  │  ├─ GET    /api/v1/templates/{id}                      获取模板     │   │
│  │  ├─ POST   /api/v1/templates                           创建模板     │   │
│  │  ├─ PUT    /api/v1/templates/{id}                      更新模板     │   │
│  │  ├─ DELETE /api/v1/templates/{id}                      删除模板     │   │
│  │  └─ POST   /api/v1/templates/{id}/pool/warm            预热水池     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  SandboxVolume Management (→ Storage Proxy)                         │   │
│  │  ├─ POST   /api/v1/sandboxvolumes                    创建持久卷      │   │
│  │  ├─ GET    /api/v1/sandboxvolumes                    列出持久卷      │   │
│  │  ├─ GET    /api/v1/sandboxvolumes/{id}               获取持久卷      │   │
│  │  ├─ DELETE /api/v1/sandboxvolumes/{id}               删除持久卷      │   │
│  │  ├─ POST   /api/v1/sandboxvolumes/{id}/attach        挂载到沙箱      │   │
│  │  ├─ POST   /api/v1/sandboxvolumes/{id}/detach        卸载            │   │
│  │  ├─ POST   /api/v1/sandboxvolumes/{id}/snapshot      创建快照       │   │
│  │  └─ POST   /api/v1/sandboxvolumes/{id}/restore       恢复快照       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 四、鉴权机制

### 4.1 认证方式

```go
// AuthContext 认证上下文
type AuthContext struct {
    // 认证方式
    AuthMethod AuthMethod

    // 团队ID
    TeamID string

    // 用户ID（可选，JWT认证时存在）
    UserID string

    // API Key ID（可选，API Key认证时存在）
    APIKeyID string

    // 角色
    Roles []string

    // 权限
    Permissions []string
}

type AuthMethod string

const (
    AuthMethodAPIKey   AuthMethod = "api_key"
    AuthMethodJWT      AuthMethod = "jwt"
    AuthMethodInternal AuthMethod = "internal"
)
```

### 4.2 API Key 认证

```
Header: Authorization: Bearer <api_key>
Format: sb0_<team_id>_<random_secret>

Examples:
- sb0_team123_abc123def456789
- sb0_team456_xyz789ghi012345
```

### 4.3 权限控制

```go
// 预定义权限
const (
    // 沙箱权限
    PermSandboxCreate   = "sandbox:create"
    PermSandboxRead     = "sandbox:read"
    PermSandboxWrite    = "sandbox:write"
    PermSandboxDelete   = "sandbox:delete"

    // 模板权限
    PermTemplateCreate  = "template:create"
    PermTemplateRead    = "template:read"
    PermTemplateWrite   = "template:write"
    PermTemplateDelete  = "template:delete"

    // 持久卷权限
    PermSandboxVolumeCreate    = "sandboxvolume:create"
    PermSandboxVolumeRead      = "sandboxvolume:read"
    PermSandboxVolumeWrite     = "sandboxvolume:write"
    PermSandboxVolumeDelete    = "sandboxvolume:delete"
)

// 预定义角色
var RolePermissions = map[string][]string{
    "admin": {
        "*:*", // 全部权限
    },
    "developer": {
        PermSandboxCreate,
        PermSandboxRead,
        PermSandboxWrite,
        PermSandboxDelete,
        PermTemplateRead,
        PermSandboxVolumeCreate,
        PermSandboxVolumeRead,
        PermSandboxVolumeWrite,
        PermSandboxVolumeDelete,
    },
    "viewer": {
        PermSandboxRead,
        PermTemplateRead,
        PermSandboxVolumeRead,
    },
}
```

---

## 4.4 SandboxVolume Attach/Detach 协调流程

Internal Gateway 作为协调者，负责协调 Storage Proxy 和 Procd 完成 SandboxVolume 的挂载和卸载。

### Attach Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 Internal Gateway Attach Coordination                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Client Request                                                           │
│     POST /api/v1/sandboxvolumes/{id}/attach                                 │
│     { "sandbox_id": "sb-123", "mount_point": "/workspace" }                  │
│                                                                              │
│  2. Internal Gateway → Storage Proxy (prepare mount)                        │
│     POST http://storage-proxy:8081/api/v1/sandboxvolumes/{id}/attach        │
│     Response: { "token": "eyJ...", "storage_proxy_address": "..." }          │
│                                                                              │
│  3. Internal Gateway → Procd (mount with token)                             │
│     POST http://procd-{pod-id}:8080/api/v1/sandboxvolumes/mount              │
│     {                                                                       │
│       "sandboxvolume_id": "sbv-456",                                        │
│       "sandbox_id": "sb-123",                                               │
│       "mount_point": "/workspace",                                          │
│       "token": "eyJ...",                                                     │
│       "storage_proxy_address": "storage-proxy:8080"                         │
│     }                                                                       │
│     Response: { "mounted_at": "2024-01-01T00:00:00Z" }                       │
│                                                                              │
│  4. Return to Client                                                         │
│     Response: 200 OK                                                         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Detach Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 Internal Gateway Detach Coordination                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Client Request                                                           │
│     POST /api/v1/sandboxvolumes/{id}/detach                                 │
│     { "sandbox_id": "sb-123" }                                               │
│                                                                              │
│  2. Internal Gateway → Procd (unmount first)                                │
│     POST http://procd-{pod-id}:8080/api/v1/sandboxvolumes/unmount            │
│     Response: { "unmounted_at": "2024-01-01T00:00:00Z" }                     │
│                                                                              │
│  3. Internal Gateway → Storage Proxy (detach record)                        │
│     POST http://storage-proxy:8081/api/v1/sandboxvolumes/{id}/detach        │
│     Response: { "detached": true }                                           │
│                                                                              │
│  4. Return to Client                                                         │
│     Response: 200 OK                                                         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**优势：**
- **无循环依赖**：Storage Proxy 和 Procd 互不依赖
- **清晰职责**：Internal Gateway 负责编排，Storage Proxy 管理元数据，Procd 管理挂载
- **易于扩展**：未来可添加更多协调逻辑（如重试、回滚）

---

## 五、路由与服务发现

### 5.1 路由规则

```go
// RouteConfig 路由配置
type RouteConfig struct {
    // 路径前缀
    PathPrefix string

    // 目标服务
    TargetService string // "manager" or "procd"

    // 目标地址（可选，用于静态路由）
    TargetURL *url.URL

    // 超时配置
    Timeout time.Duration

    // 重试配置
    RetryPolicy *RetryPolicy

    // 限流配置
    RateLimit *RateLimitConfig
}

// Router 路由器
type Router struct {
    routes     map[string]*RouteConfig
    managerURL *url.URL
    procdResolver *ProcdResolver
}

// ProcdResolver Procd地址解析器
type ProcdResolver struct {
    // 从数据库获取Procd地址
    pgClient *pgxpool.Pool
}

func (r *ProcdResolver) Resolve(sandboxID string) (*url.URL, error) {
    // 直接查询数据库（PG索引查询足够快，无需额外缓存）
    var procdAddr string
    err := r.pgClient.QueryRow(
        context.Background(),
        "SELECT procd_address FROM sandboxes WHERE id = $1",
        sandboxID,
    ).Scan(&procdAddr)

    if err != nil {
        return nil, err
    }

    targetURL, err := url.Parse(procdAddr)
    if err != nil {
        return nil, err
    }

    return targetURL, nil
}
```

---

## 六、限流与配额管理

### 6.1 限流策略

```go
// RateLimitConfig 限流配置
type RateLimitConfig struct {
    // 每秒请求数
    RequestsPerSecond int

    // 突发大小
    Burst int

    // 每小时请求数
    RequestsPerHour *int

    // 并发连接数
    MaxConcurrent int
}

// TokenBucketLimiter 令牌桶限流器（基于PGSQL）
type TokenBucketLimiter struct {
    pgClient *pgxpool.Pool
    logger   *zap.Logger
}
```

### 6.2 配额管理

```go
// QuotaManager 配额管理器
type QuotaManager struct {
    pgClient *pgxpool.Pool
}

// QuotaType 配额类型
type QuotaType string

const (
    QuotaSandboxCount        QuotaType = "sandbox_count"         // 沙箱数量
    QuotaSandboxCPU          QuotaType = "sandbox_cpu"           // CPU配额
    QuotaSandboxMemory       QuotaType = "sandbox_memory"        // 内存配额
    QuotaSandboxVolumeStorage QuotaType = "sandboxvolume_storage" // 持久卷存储配额
    QuotaAPICalls            QuotaType = "api_calls"             // API调用次数
)
```

---

## 七、数据库 Schema

```sql
-- API Keys表
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    key_value TEXT NOT NULL UNIQUE,

    -- 关联
    team_id TEXT NOT NULL,
    created_by TEXT NOT NULL,

    -- 配置
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- 'user', 'service', 'internal'
    roles JSONB NOT NULL DEFAULT '[]',

    -- 状态
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ NOT NULL,

    -- 使用统计
    last_used_at TIMESTAMPTZ,
    usage_count BIGINT DEFAULT 0,

    -- 时间戳
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Teams表
CREATE TABLE teams (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,

    -- 配额
    quota JSONB NOT NULL,

    -- 状态
    is_active BOOLEAN NOT NULL DEFAULT true,

    -- 时间戳
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Users表
CREATE TABLE users (
    id TEXT PRIMARY KEY,

    -- 外部身份（SSO）
    external_id TEXT,
    provider TEXT, -- 'google', 'github', etc.

    -- 基本信息
    email TEXT NOT NULL,
    name TEXT,

    -- 团队关联
    primary_team_id TEXT,

    -- 权限
    roles JSONB NOT NULL DEFAULT '[]',
    permissions JSONB NOT NULL DEFAULT '[]',

    -- 状态
    is_active BOOLEAN NOT NULL DEFAULT true,

    -- 时间戳
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    FOREIGN KEY (primary_team_id) REFERENCES teams(id) ON DELETE SET NULL
);

-- Audit Log表
CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,

    -- 关联
    team_id TEXT NOT NULL,
    user_id TEXT,
    api_key_id TEXT,

    -- 请求信息
    request_id TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,

    -- 响应信息
    status_code INTEGER NOT NULL,
    latency_ms INTEGER,

    -- 元数据
    user_agent TEXT,
    client_ip TEXT,
    metadata JSONB,

    -- 时间戳
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL
);
```

---

## 八、与 E2B 功能对比

| 功能 | E2B | Sandbox0 | 说明 |
|------|-----|----------|------|
| API认证 | 5种认证方式 | API Key + JWT (可选) | 简化但足够 |
| 团队隔离 | Team | Team + RBAC | 更细粒度 |
| 沙箱API | `/sandboxes` | `/api/v1/sandboxes` | RESTful |
| 进程/执行 | `/sandboxes/{id}/execute` | `/sandboxes/{id}/contexts` | 支持多Context |
| 文件操作 | `/files` | `/sandboxes/{id}/files` | 路径更清晰 |
| WebSocket | 支持 | 支持 | 实时通信 |
| 限流 | 内置 | PGSQL行级锁 | 无额外依赖 |
| 配额 | 基于模板 | 独立配额系统 | 更灵活 |

---

## 九、总结

### 设计优势

1. **统一入口**：所有外部请求统一经过gateway，便于管理
2. **简化认证**：主要使用API Key，JWT为可选SSO支持
3. **清晰路由**：Manager管理沙箱/模板，Storage Proxy管理持久卷，Procd管理进程/文件
4. **灵活扩展**：中间件模式，易于添加新功能
5. **完整可观测**：Metrics、Tracing、Logging全覆盖
6. **低依赖**：仅依赖PGSQL，无额外中间件
7. **E2B兼容**：所有E2B功能都有对应实现
