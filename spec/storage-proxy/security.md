# Storage Proxy - Security Design

## 一、安全模型

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Security Architecture                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Untrusted Zone                    Trusted Zone                              │
│  ┌─────────────────────┐         ┌─────────────────────────────────────────┐│
│  │ Procd Pod           │         │ Storage Proxy                           ││
│  │ ┌─────────────────┐ │         │ ┌─────────────────────────────────────┐ ││
│  │ │ User Shell/REPL │ │         │ │ All Credentials (S3, PostgreSQL)     │ ││
│  │ │    ├─ /proc     │ │         │ │ ┌─────────────────────────────────┐ │ ││
│  │ │    └─ Can't     │ │         │ │ │ JWT Validation                  │ │ ││
│  │ │      access     │ │         │ │ │ ├─ Volume access control       │ │ ││
│  │ │      Proxy      │ │         │ │ │ ├─ Token expiration (24h)       │ │ ││
│  │ └─────────────────┘ │         │ │ │ └─ Per-sandbox tokens          │ │ ││
│  │                     │         │ └─────────────────────────────────┘ │ ││
│  │ RemoteFS            │         │ ┌─────────────────────────────────┐ │ ││
│  │ ┌─────────────────┐ │         │ │ Audit Logging (all operations)   │ │ ││
│  │ │ gRPC Client     │ │         │ │ ├─ Who accessed what             │ │ ││
│  │ │ ├─ Bearer Token │ ├─────────┼─┤ ├─ When (timestamp)              │ │ ││
│  │ │ └─ No Creds     │ │         │ │ └─ How much (data size)          │ │ ││
│  │ └─────────────────┘ │         │ └─────────────────────────────────┘ │ ││
│  └─────────────────────┘         │ ┌─────────────────────────────────┐ │ ││
│           │                     │ │ Rate Limiting                    │ │ ││
│           │ gRPC (mark=0x2)     │ │ ├─ Per-volume QPS                │ │ ││
│           │                     │ │ ├─ Per-sandbox bandwidth         │ │ ││
│           ▼                     │ │ └─ DDoS protection              │ │ ││
│   nftables                    │ └─────────────────────────────────┘ │ ││
│   mark==0x2 → ACCEPT           └─────────────────────────────────────────┘│
│   other → whitelist                                                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 二、Token-Based Authentication

### 2.1 Token Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Token Authentication Flow                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Manager claims idle pod for sandbox                                     │
│     Manager → Procd: "Claim this pod for sandbox sb-123"                    │
│                                                                              │
│  2. Manager generates storage token                                          │
│     ├─ Fetch volume config from PostgreSQL                                   │
│     ├─ Verify sandbox has access to volume                                   │
│     └─ Generate JWT token:                                                  │
│         {                                                                     │
│           "volume_id": "vol-456",                                           │
│           "sandbox_id": "sb-123",                                           │
│           "team_id": "team-789",                                            │
│           "exp": 1706745600  // 24 hour expiration                         │
│         }                                                                   │
│         Signed with HMAC-SHA256 (JWT_SECRET)                                │
│                                                                              │
│  3. Manager calls Procd API with token                                       │
│     POST /api/v1/volumes/vol-456/mount                                      │
│     {                                                                       │
│       "sandbox_id": "sb-123",                                              │
│       "mount_point": "/workspace",                                          │
│       "storage_token": "eyJhbGc..."  // JWT token                          │
│     }                                                                       │
│                                                                              │
│  4. Procd stores token (in memory only, never written to disk)              │
│     Procd memory:                                                           │
│     ┌─────────────────────────────────────┐                                 │
│     │ volumeMounts: {                     │                                 │
│     │   "vol-456": {                     │                                 │
│     │     mountPoint: "/workspace",       │                                 │
│     │     token: "eyJhbGc...",            │                                 │
│     │     proxyAddr: "storage-proxy:8080"│                                 │
│     │   }                                │                                 │
│     │ }                                  │                                 │
│     └─────────────────────────────────────┘                                 │
│                                                                              │
│  5. Procd RemoteFS makes gRPC calls with token                               │
│     Every gRPC call includes:                                                │
│     metadata: {                                                              │
│       "authorization": ["Bearer eyJhbGc..."]                                │
│     }                                                                       │
│                                                                              │
│  6. Storage Proxy validates token                                            │
│     ├─ Verify HMAC signature                                                 │
│     ├─ Check expiration                                                      │
│     ├─ Verify volume access (from PostgreSQL)                                │
│     └─ Extract volume_id to route to correct VFS instance                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Token Structure

```go
// Token claims structure
type TokenClaims struct {
    // Subject
    VolumeID  string `json:"volume_id"`  // Target volume
    SandboxID string `json:"sandbox_id"` // Requesting sandbox
    TeamID    string `json:"team_id"`    // Owner team

    // Standard JWT claims
    jwt.RegisteredClaims
}

// Example token payload:
{
    "volume_id": "vol-abc123",
    "sandbox_id": "sb-def456",
    "team_id": "team-789",
    "iss": "sandbox0-manager",     // Issuer
    "iat": 1706659200,              // Issued at
    "exp": 1706745600,              // Expires (24 hours)
    "nbf": 1706659200               // Not before
}
```

### 2.3 Token Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Token Lifecycle                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Token Generation                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Manager generates token when:                                      │   │
│  │  - Sandbox is claimed (initial mount)                              │   │
│  │  - Token is about to expire (refresh)                              │   │
│  │  - Sandbox is resumed (after pause)                                │   │
│  │                                                                     │   │
│  │ Token validity: 24 hours (configurable)                            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Token Usage (in Procd)                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ gRPC calls from RemoteFS:                                           │   │
│  │  Read, Write, Create, Mkdir, Unlink, Lookup, ReadDir, etc.         │   │
│  │                                                                     │   │
│  │ Token sent in metadata:                                             │   │
│  │  Authorization: Bearer eyJhbGc...                                   │   │
│  │                                                                     │   │
│  │ Token storage:                                                       │   │
│  │  - In-memory only (never disk)                                      │   │
│  │  - Cleared on unmount/sandbox deletion                              │   │
│  │  - Not accessible to user processes                                 │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Token Validation (in Storage Proxy)                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Every gRPC request:                                                 │   │
│  │  1. Extract Bearer token from metadata                              │   │
│  │  2. Verify HMAC signature using JWT_SECRET                         │   │
│  │  3. Check expiration (exp)                                          │   │
│  │  4. Check not-before (nbf)                                          │   │
│  │  5. Validate volume access:                                         │   │
│  │     a. Extract volume_id from token                                 │   │
│  │     b. Query PostgreSQL: SELECT * FROM volume_access                │   │
│  │        WHERE volume_id = ? AND sandbox_id = ?                      │   │
│  │  6. Extract sandbox_id for audit logging                           │   │
│  │                                                                     │   │
│  │ On validation failure:                                               │   │
│  │  - Return Unauthenticated (401) or PermissionDenied (403)          │   │
│  │  - Log security event                                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Token Expiration                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ When token expires (after 24h):                                     │   │
│  │  1. Storage Proxy returns: "token expired"                         │   │
│  │  2. RemoteFS returns error to user                                  │   │
│  │  3. Procd calls Manager to refresh token                           │   │
│  │  4. Manager validates sandbox still exists                          │   │
│  │  5. Manager generates new token                                     │   │
│  │  6. Procd updates in-memory token                                   │   │
│  │                                                                     │   │
│  │ Note: Token refresh is transparent to user                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Token Revocation                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Immediate revocation scenarios:                                     │   │
│  │  - Sandbox is deleted                                              │   │
│  │  - Volume is unmounted from sandbox                                │   │
│  │  - Team subscription expires                                        │   │
│  │  - Security incident detected                                       │   │
│  │                                                                     │   │
│  │ Revocation mechanism:                                               │   │
│  │  1. Manager marks tokens as invalid in PostgreSQL                  │   │
│  │  2. Storage Proxy checks revocation list on every request          │   │
│  │  3. Or: Manager tells Procd to unmount volume                       │   │
│  │                                                                     │   │
│  │ Future: Add token blacklist in Redis for fast revocation           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、Credential Isolation

### 3.1 What Credentials Exist

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Credentials Inventory                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  PostgreSQL Credentials (JuiceFS metadata)                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Where stored:                                                         │   │
│  │  - Storage Proxy environment variables                               │   │
│  │  - Or: AWS Secrets Manager / Vault (recommended)                     │   │
│  │                                                                       │   │
│  │ Format:                                                                │   │
│  │  postgres://username:password@postgres:5432/sandbox0                 │   │
│  │                                                                       │   │
│  │ Permissions needed:                                                   │   │
│  │  - CREATE, SELECT, INSERT, UPDATE, DELETE on juicefs tables          │   │
│  │  - (juicefs creates tables automatically on first mount)             │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  S3 Credentials (JuiceFS data chunks)                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Where stored:                                                         │   │
│  │  - Storage Proxy environment variables                              │   │
│  │  - Or: IRSA (IAM Roles for Service Accounts) - recommended          │   │
│  │                                                                       │   │
│  │ Format:                                                                │   │
│  │  Option A: Access Key + Secret Key                                   │   │
│  │    AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE                            │   │
│  │    AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY    │   │
│  │                                                                       │   │
│  │  Option B: IRSA (recommended)                                        │   │
│  │    AWS_WEB_IDENTITY_TOKEN_FILE=/var/run/secrets/eks/...             │   │
│  │    AWS_ROLE_ARN=arn:aws:iam::123456:role/sandbox0-s3-access         │   │
│  │                                                                       │   │
│  │ IAM Policy needed:                                                     │   │
│  │  - s3:GetObject, s3:PutObject, s3:DeleteObject                       │   │
│  │  - s3:ListBucket on sandbox0-volumes bucket                         │   │
│  │  - Path restricted: sandbox0-volumes/teams/{team_id}/*              │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  JWT Secret (Token signing)                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Where stored:                                                         │   │
│  │  - Storage Proxy environment variable (JWT_SECRET)                   │   │
│  │  - Manager environment variable (same value)                         │   │
│  │                                                                       │   │
│  │ Format: 32+ byte random string                                        │   │
│  │                                                                       │   │
│  │ Rotation:                                                             │   │
│  │  - Manual rotation via redeployment                                   │   │
│  │  - Future: support multiple active secrets for smooth rotation       │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Credential Access Control

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Credential Access Matrix                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────┬──────────────┬──────────────┬─────────────────────┐│
│  │     Component       │  PostgreSQL  │     S3       │     JWT Secret      ││
│  ├─────────────────────┼──────────────┼──────────────┼─────────────────────┤│
│  │ Procd               │      ❌      │      ❌      │        ❌          ││
│  │ (User processes)    │              │              │                     ││
│  ├─────────────────────┼──────────────┼──────────────┼─────────────────────┤│
│  │ Procd RemoteFS      │      ❌      │      ❌      │        ❌          ││
│  │ (gRPC client only)  │              │              │  (Has signed token) ││
│  ├─────────────────────┼──────────────┼──────────────┼─────────────────────┤│
│  │ Storage Proxy       │      ✅       │      ✅       │        ✅          ││
│  │ (Trusted service)    │              │              │  (Signs tokens)     ││
│  ├─────────────────────┼──────────────┼──────────────┼─────────────────────┤│
│  │ Manager             │      ✅       │      ❌      │        ✅          ││
│  │ (Issues tokens)     │              │              │  (Signs tokens)     ││
│  ├─────────────────────┼──────────────┼──────────────┼─────────────────────┤│
│  │ User Shell/REPL     │      ❌      │      ❌      │        ❌          ││
│  │ (Untrusted code)    │              │              │                     ││
│  └─────────────────────┴──────────────┴──────────────┴─────────────────────┘│
│                                                                              │
│  Legend:                                                                     │
│  ✅ = Has direct access to credential                                         │
│  ❌ = No access (credential isolation)                                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 Least Privilege IAM Policy (S3)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::sandbox0-volumes",
        "arn:aws:s3:::sandbox0-volumes/teams/${aws:username}/*"
      ],
      "Condition": {
        "StringLike": {
          "s3:prefix": ["teams/${aws:username}/*"]
        }
      }
    }
  ]
}
```

---

## 四、Network Security

### 4.1 Packet Marking (Procd → Proxy)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Packet Marking Architecture                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  nftables rule in Procd (OUTPUT chain):                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ chain SANDBOX0_OUTPUT {                                              │   │
│  │   # Proxy bypass rule (highest priority)                             │   │
│  │   meta mark & 0x2 == 0x2 accept                                      │   │
│  │                                                                      │   │
│  │   # Private IP blacklist (for user traffic)                          │   │
│  │   ip daddr @predef_deny drop                                         │   │
│  │                                                                      │   │
│  │   # User-defined deny list                                           │   │
│  │   ip daddr @user_deny drop                                           │   │
│  │                                                                      │   │
│  │   # Whitelist mode: redirect TCP to proxy                            │   │
│  │   meta l4proto tcp tcp dport != 8080 redirect to 127.0.0.1:1080     │   │
│  │ }                                                                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  gRPC client in Procd sets SO_MARK=0x2:                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ dialer := &net.Dialer{                                               │   │
│  │     Control: func(network, address string, c syscall.RawConn) error { │   │
│  │       return c.Control(func(fd uintptr) {                            │   │
│  │         syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET,           │   │
│  │           0x24, 0x2)  // SO_MARK=0x2                                │   │
│  │       })                                                             │   │
│  │     },                                                               │   │
│  │   }                                                                  │   │
│  │                                                                      │   │
│  │ conn, err := dialer.Dial("tcp", "storage-proxy:8080")               │   │
│  │  → All packets from this connection have mark=0x2                   │   │
│  │  → nftables rule accepts these packets                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Result:                                                                     │
│  ✅ Procd → Storage Proxy traffic: ALLOWED (mark=0x2)                       │
│  ❌ User processes → Direct S3/PG: BLOCKED (no mark, hits whitelist rules)   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Service Mesh Integration (Optional)

```yaml
# Istio service mesh for additional security
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: storage-proxy
spec:
  selector:
    matchLabels:
      app: storage-proxy
  mtls:
    mode: STRICT  # Require mTLS for all connections
---
apiVersion: security.istio.io/v1
kind: AuthorizationPolicy
metadata:
  name: storage-proxy-authz
spec:
  selector:
    matchLabels:
      app: storage-proxy
  rules:
  - from:
    - source:
        principals:
        - cluster.local/ns/sandbox0-system/sa/procd  # Only Procd can connect
    to:
    - operation:
        ports: ["8080"]  # gRPC port only
```

---

## 五、Audit Logging

### 5.1 What Is Logged

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Audit Log Fields                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Every gRPC operation logged:                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ {                                                                    │   │
│  │   "timestamp": "2024-01-01T12:00:00Z",                               │   │
│  │   "volume_id": "vol-abc123",                                         │   │
│  │   "sandbox_id": "sb-def456",                                         │   │
│  │   "team_id": "team-789",                                            │   │
│  │   "operation": "write",                                             │   │
│  │   "path": "/workspace/project/data.csv",                             │   │
│  │   "size_bytes": 12345,                                              │   │
│  │   "latency_ms": 15,                                                 │   │
│  │   "status": "success",                                              │   │
│  │   "client_ip": "10.244.1.5",                                        │   │
│  │   "user_agent": "procd/1.0.0"                                       │   │
│  │ }                                                                    │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Security events logged:                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ - Authentication failures (invalid/expired tokens)                   │   │
│  │ - Authorization failures (access denied to volume)                   │   │
│  │ - Suspicious activity (unusual patterns)                             │   │
│  │ - Token revocation                                                   │   │
│  │ - Volume mount/unmount                                               │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Log destinations:                                                           │
│  - stdout (for k8s logging)                                                 │
│  - PostgreSQL audit table                                                   │
│  - External SIEM (optional: Splunk, Elasticsearch)                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Anomaly Detection

```go
// Detect suspicious patterns
type AnomalyDetector struct {
    // Track per-sandbox operation rates
    opRates map[string]*RateTracker
}

type SuspiciousEvent struct {
    SandboxID  string
    VolumeID   string
    Reason     string  // e.g., "excessive_write_ops", "unusual_file_size"
    Operations []AuditLog
}

// Triggers:
// - More than 1000 write operations per second (potential ransomware)
// - Files > 10GB written in < 1 minute (potential data exfiltration)
// - Access to unusual paths (../../etc/passwd attempts)
// - Rapid file creation/deletion (potential attack)
```

---

## 六、Defense in Depth

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Security Layers                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Layer 1: Network Isolation                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ - nftables in Procd blocks user traffic to S3/PG                    │   │
│  │ - Packet marking allows Procd → Proxy traffic                        │   │
│  │ - Service mesh mTLS (optional)                                       │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Layer 2: Authentication                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ - JWT tokens signed with HMAC-SHA256                                │   │
│  │ - 24-hour expiration                                               │   │
│  │ - Token includes volume_id, sandbox_id                             │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Layer 3: Authorization                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ - Volume access validation from PostgreSQL                          │   │
│  │ - Sandbox can only access assigned volumes                          │   │
│  │ - Token revocation on sandbox/volume deletion                      │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Layer 4: Credential Isolation                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ - S3/PG credentials only in Storage Proxy                           │   │
│  │ - IRSA for S3 (no static keys)                                     │   │
│  │ - Secrets management (Vault/AWS SM)                                 │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Layer 5: Audit & Monitoring                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ - All operations logged with context                               │   │
│  │ - Anomaly detection for suspicious patterns                        │   │
│  │ - Prometheus metrics for observability                              │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Layer 6: Runtime Security (Optional)                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ - Kata Containers for VM-level isolation                           │   │
│  │ - AppArmor profiles for Storage Proxy                              │   │
│  │ - seccomp filters                                                   │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 七、Security Best Practices

### 7.1 Deployment Checklist

- [ ] Use IRSA for S3 access (no static keys)
- [ ] Rotate JWT secret quarterly
- [ ] Enable mTLS between services (Istio/Linkerd)
- [ ] Restrict Storage Proxy to only accept traffic from Procd pods
- [ ] Enable Pod Security Policies (no privileged containers)
- [ ] Scan container images for vulnerabilities
- [ ] Enable audit logging to external SIEM
- [ ] Set up alerting for authentication failures
- [ ] Use separate database credentials for Storage Proxy
- [ ] Encrypt PostgreSQL connections (sslmode=require)

### 7.2 Incident Response

If credential leakage is suspected:

1. **Immediate Actions**
   - Revoke all tokens (tell Procd pods to unmount)
   - Rotate JWT secret (deploy new Storage Proxy instances)
   - Rotate S3 credentials (create new IAM role)
   - Rotate PostgreSQL password

2. **Investigation**
   - Review audit logs for affected time period
   - Identify which sandboxes/volumes were accessed
   - Check S3 access logs for unusual activity

3. **Recovery**
   - Re-deploy Storage Proxy with new credentials
   - Remount volumes in all active sandboxes
   - Generate new tokens for all sandboxes
   - Monitor for suspicious activity post-rotation
