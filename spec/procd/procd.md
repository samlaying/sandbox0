# Procd 设计规范

## 一、设计目标

Procd 是 Sandbox0 的核心容器组件（PID=1），负责沙箱内的资源管理和进程控制。

### 核心职责

1. **进程管理**：统一的进程抽象，支持 REPL 和 Shell 两种进程类型
2. **SandboxVolume 管理**：持久化存储的挂载、卸载（通过 storage-proxy）
3. **网络隔离**：动态网络策略，IP/CIDR 过滤、域名过滤、DNS 欺骗防护
4. **文件操作**：文件读写、目录监听、文件事件推送

---

## 二、架构概览

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                        Procd Architecture                                    │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Procd (PID=1)                                                              │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                        HTTP Server (Port: 8080)                       │  │
│   │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐      │  │
│   │  │  Process    │ │SandboxVolume│ │   Network   │ │   File      │      │  │
│   │  │   APIs      │ │    APIs     │ │    APIs     │ │   APIs      │      │  │
│   │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘      │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                        Core Managers                                  │  │
│   │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐      │  │
│   │  │  Context    │ │SandboxVolume│ │  Network    │ │   File      │      │  │
│   │  │  Manager    │ │  Manager    │ │  Manager    │ │  Manager    │      │  │
│   │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘      │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                      Subprocess Layer                                 │  │
│   │  ┌─────────────┐ ┌─────────────┐                                      │  │
│   │  │ REPL Proc   │ │ Shell Proc  │                                      │  │
│   │  │ (IPython)   │ │  (Bash)     │                                      │  │
│   │  └─────────────┘ └─────────────┘                                      │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 子模块索引

| 子模块 | 说明 | 文档 |
|--------|------|------|
| **Process** | 进程管理（REPL、Shell） | [process.md](./process.md) |
| **SandboxVolume** | 持久化存储管理（RemoteFS 客户端） | [sandboxvolume-client.md](./sandboxvolume-client.md) |
| **Network** | 网络隔离与策略控制 | [network.md](./network.md) |
| **File** | 文件系统操作与监听 | [file.md](./file.md) |

---

## 三、HTTP API 概览

Procd 提供统一的 HTTP API（端口 8080），详细定义见各子模块文档。

| API 类别 | 基础路径 | 详细文档 |
|----------|----------|----------|
| Context/进程 | `/api/v1/contexts` | [process.md](./process.md) |
| SandboxVolume | `/api/v1/sandboxvolumes` | [sandboxvolume-client.md](./sandboxvolume-client.md) |
| Network | `/api/v1/network` | [network.md](./network.md) |
| File | `/api/v1/files` | [file.md](./file.md) |
| Health | `/healthz` | - |

---

## 四、核心数据结构概览

### 4.1 Context 结构

> 详见 [process.md](./process.md#四、接口定义)

Context 是进程的逻辑容器，包含统一的工作目录、环境变量和主进程。

```go
type Context struct {
    ID          string
    Type        ProcessType  // "repl" | "shell"
    Language    string        // python/node/bash等
    CWD         string
    EnvVars     map[string]string
    MainProcess Process      // 主进程
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 4.2 Process 接口

> 详见 [process.md](./process.md#四、接口定义)

所有进程类型（REPL、Shell）都实现统一的 Process 接口。

---

## 五、启动配置

### 5.1 环境变量

```bash
# Procd 必需
SANDBOX_ID=sb-abc123              # 沙箱ID
TEMPLATE_ID=python-dev            # 模板ID

# Procd 可选
PROCD_LOG_LEVEL=info              # 日志级别
PROCD_MAX_CONTEXTS=100            # 最大Context数

# Storage Proxy (SandboxVolume 挂载需要)
STORAGE_PROXY_URL=storage-proxy.sandbox0-system.svc.cluster.local:8080
```

### 5.2 Pod 配置

```yaml
# SandboxTemplate CRD 中的 Pod 配置
apiVersion: v1
kind: Pod
metadata:
  name: sandbox-abc
  labels:
    sandbox0.ai/template-id: python-dev
    sandbox0.ai/pool-type: idle
spec:
  runtimeClassName: kata           # 推荐：使用 Kata Containers 隔离
  hostname: sandbox-abc

  containers:
  # Main: Procd (PID=1)
  - name: procd
    image: procd:latest
    securityContext:
      capabilities:
        add:
          - NET_ADMIN             # nftables config only
    env:
      - name: SANDBOX_ID
        value: "sb-abc"
      - name: TEMPLATE_ID
        value: "python-dev"
      - name: STORAGE_PROXY_URL
        value: "storage-proxy.sandbox0-system.svc.cluster.local:8080"
    ports:
      - containerPort: 8080        # HTTP API
```

### 5.3 启动流程

```go
func main() {
    // 1. Parse config
    config := loadConfig()

    // 2. Initialize managers
    networkManager := NewNetworkManager(config.Network)
    sandboxvolumeManager := NewSandboxVolumeManager(config.StorageProxyURL)
    contextManager := NewContextManager(config.Context)
    fileManager := NewFileManager(config.RootPath)

    // 3. Setup network
    networkManager.SetupNetwork()

    // 4. Start HTTP server
    server := NewProcdServer(networkManager, sandboxvolumeManager, contextManager, fileManager)
    server.Start(":8080")
}
```

---

## 六、与其他组件的关系

### 6.1 与 Storage Proxy 的交互

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                   Storage Proxy ↔ Procd 交互                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Storage Proxy                              Procd                         │
│     │                                           │                          │
│     │ 1. 挂载SandboxVolume                       │                          │
│     │    POST /api/v1/sandboxvolumes/mount     │                          │
│     ├──────────────────────────────────────────► │                          │
│     │                                           │ 2. 创建FUSE挂载           │
│     │                                           │                          │
│     ├──────────────────────────────────────────► │                          │
│     │                                           │ 4. 通过storage-proxy挂载    │
│     │                                           │    (gRPC to storage-proxy) │
│     │                                           │                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 与 Storage-Proxy 的交互 (gRPC)

> 详见 [sandboxvolume-client.md](./sandboxvolume-client.md#七、与-storage-proxy-的交互)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                Procd → Storage-Proxy 交互 (gRPC)                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Procd                                    Storage-Proxy                     │
│     │                                           │                          │
│     │ 1. 文件系统操作请求                         │                          │
│     │    gRPC: Read/Write/Create/Lookup...     │                          │
│     ├──────────────────────────────────────────► │                          │
│     │                                           │ 2. 返回文件系统数据        │
│     │                                           │    (JuiceFS操作结果)       │
│     │                                           │                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.3 与 Internal Gateway 的交互

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 Internal Gateway → Procd                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Gateway                                    Procd                           │
│     │                                        │                              │
│     │ 1. 创建Context                           │                              │
│     │    POST /api/v1/contexts                │                              │
│     ├───────────────────────────────────────► │                              │
│     │                                        │ 2. 启动REPL/Shell进程        │
│     │                                        │                              │
│     │ 3. 执行代码                              │                              │
│     │    POST /api/v1/contexts/{id}/execute  │                              │
│     ├───────────────────────────────────────► │                              │
│     │                                        │ 4. 写入PTY并读取输出         │
│     │                                        │                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 七、技术选型

| 组件 | 技术选择 | 说明 |
|------|----------|------|
| HTTP Server | Go stdlib | 无需额外依赖 |
| PTY | github.com/creack/pty | 伪终端支持 |
| FUSE | github.com/hanwen/go-fuse/v2 | RemoteFS 实现 |
| nftables | google.com/googleapis/... | 网络防火墙 |
| fsnotify | github.com/fsnotify/fsnotify | 文件监听 |

---

## 八、限制与约束

1. **单进程模式**：Procd 作为 PID=1 运行，不使用多进程
2. **内存限制**：所有 Context 共享容器内存配额
3. **网络隔离**：每个沙箱独立的网络命名空间
4. **存储路径**：SandboxVolume 挂载点固定在 `/workspace` 或用户指定路径

---

## 九、监控指标

```
# 进程指标
procd_contexts_total              # 当前Context总数
procd_contexts_by_type            # 按类型统计
procd_process_start_duration_ms   # 进程启动耗时

# SandboxVolume指标
procd_sandboxvolumes_mounted           # 已挂载数量
procd_sandboxvolume_mount_duration_ms  # 挂载耗时

# 网络指标
procd_network_rules_total         # 防火墙规则数
procd_network_policy_updates      # 策略更新次数

# 文件指标
procd_file_operations_total       # 文件操作总数
procd_file_watchers_active        # 活跃监听器数量
```
