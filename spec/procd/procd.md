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
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 子模块索引

| 子模块 | 说明 | 文档 |
|--------|------|------|
| **Context/Process** | 进程管理（REPL、Shell） | [process.md](./process.md) |
| **SandboxVolume** | 持久化存储管理 | [client-sandboxvolume.md](./client-sandboxvolume.md) |
| **Network** | 网络隔离与策略控制 | [network.md](./network.md) |
| **File** | 文件系统操作与监听 | [file.md](./file.md) |

---

## 三、启动配置

### 3.1 环境变量

| 变量名 | 必需 | 说明 | 默认值 |
|--------|------|------|--------|
| `SANDBOX_ID` | ✅ | 沙箱ID | - |
| `TEMPLATE_ID` | ✅ | 模板ID | - |
| `PROCD_LOG_LEVEL` | ❌ | 日志级别 (debug/info/warn/error) | info |
| `PROCD_HTTP_PORT` | ❌ | HTTP 端口 | 8080 |
| `STORAGE_PROXY_BASE_URL` | ✅ | Storage Proxy 地址 | - |
| `NODE_NAME` | ❌ | 节点名称（用于 volume cache） | - |
| `PROCD_MAX_CONTEXTS` | ❌ | 最大 Context 数 | 100 |

### 3.2 Pod 配置

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
  - name: procd
    image: procd:latest
    securityContext:
      capabilities:
        add:
          - NET_ADMIN             # 网络隔离需要
    env:
      - name: SANDBOX_ID
        value: "sb-abc"
      - name: TEMPLATE_ID
        value: "python-dev"
      - name: STORAGE_PROXY_BASE_URL
        value: "storage-proxy.sandbox0-system.svc.cluster.local:8080"
    ports:
      - containerPort: 8080        # HTTP API
```

---

## 四、与其他组件的关系

### 4.1 与 Internal Gateway 的交互

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 Internal Gateway → Procd                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Gateway                                    Procd                           │
│     │                                        │                              │
│     │ 创建 Context / 执行代码 / 网络策略 / 文件操作                          │
│     ├───────────────────────────────────────►                              │
│     │                                        │ 启动进程 / 配置网络 / 文件操作 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 与 Storage Proxy 的交互

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                   Storage Proxy ↔ Procd                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Storage Proxy                              Procd                           │
│     │                                           │                          │
│     │ attach/detach/snapshot/restore            │                          │
│     ├──────────────────────────────────────────►│                          │
│     │                                           │ FUSE mount               │
│     │                                           │ gRPC file operations     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 五、技术选型

| 组件 | 技术选择 | 说明 |
|------|----------|------|
| HTTP Server | Go stdlib | 无需额外依赖 |
| PTY | github.com/creack/pty | 伪终端支持 |
| nftables | google.com/googleapis/nftables | 网络防火墙 |
| fsnotify | github.com/fsnotify/fsnotify | 文件监听 |
| 日志 | go.uber.org/zap | 结构化日志 |

---

## 六、限制与约束

1. **单进程模式**：Procd 作为 PID=1 运行
2. **内存限制**：所有 Context 共享容器内存配额
3. **网络隔离**：每个沙箱独立的网络命名空间
4. **存储路径**：SandboxVolume 挂载点由用户指定
