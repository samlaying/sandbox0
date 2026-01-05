# Procd - SandboxVolume Client (RemoteFS Client)

## 一、设计目标

Procd 的 SandboxVolumeManager 负责将远程文件系统（SandboxVolume）挂载到 Pod 内的指定路径。

### 核心原则

1. **零存储凭证**：Procd 不持有任何 S3、PostgreSQL 凭证
2. **轻量级**：只负责 FUSE 挂载和 gRPC 客户端
3. **网络隔离兼容**：通过 packet marking 绕过用户网络规则
4. **快速挂载**：<50ms 挂载延迟

---

## 二、架构设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Procd SandboxVolume Architecture                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Storage Proxy                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ SandboxVolume Management                                             │   │
│  │  - Create/Delete SandboxVolume                                       │   │
│  │  - Attach/Detach to Sandbox                                          │   │
│  │  - Snapshot/Restore                                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                                    ▼                                         │
│  Procd (PID=1, in Pod)                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ SandboxVolumeManager                                                  │   │
│  │  - Mount/Unmount API (HTTP)                                          │   │
│  │  - RemoteFS (FUSE filesystem)                                        │   │
│  │  - gRPC Client with packet marking (SO_MARK=0x2)                     │   │
│  │                                                                        │   │
│  │  /workspace (FUSE mount point)                                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │ gRPC (mark=0x2)                        │
│                                    ▼                                        │
│                          Storage Proxy (JuiceFS Backend)                   │
│                          (Has all credentials)                            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、挂载流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│              Internal Gateway Coordinated Mount Flow                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. User requests to attach SandboxVolume to Sandbox                        │
│     POST /api/v1/sandboxvolumes/{id}/attach                                 │
│                                                                              │
│  2. Internal Gateway calls Storage Proxy to prepare mount                   │
│     → Returns token for gRPC authentication                                 │
│                                                                              │
│  3. Internal Gateway calls Procd API to mount sandboxvolume                 │
│     POST /api/v1/sandboxvolumes/mount                                        │
│                                                                              │
│  4. Procd SandboxVolumeManager mounts RemoteFS                              │
│     ├─ Create gRPC connection (with SO_MARK=0x2)                            │
│     ├─ Mount FUSE at /workspace                                             │
│     └─ Start FUSE server (forwards to gRPC)                                 │
│                                                                              │
│  5. User can now access files in /workspace                                 │
│     All file operations: User → FUSE → gRPC → Storage Proxy → S3/PG          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 四、网络配置

### 4.1 nftables 配置

Procd 启动时配置 nftables 规则，允许标记的数据包绕过用户网络规则：

```bash
table inet sb0-firewall {
    chain SANDBOX0_OUTPUT {
        # Proxy bypass (highest priority)
        meta mark & 0x2 == 0x2 accept

        # User rules...
    }
}
```

### 4.2 节点亲和性路由

Procd 优先连接同节点的 Storage Proxy 实例以减少延迟：

- Storage Proxy 作为 StatefulSet 部署
- Procd 通过 `NODE_NAME` 环境变量计算目标副本
- 使用 hash 函数：`hash(nodeName) % replicaCount`

---

## 五、性能优化

### 5.1 性能目标

| Operation | 目标延迟 | 说明 |
|-----------|----------|------|
| Mount | ~30-50ms | gRPC connect + FUSE mount |
| Read (cached) | ~2-3ms | gRPC roundtrip |
| Write | ~5-10ms | gRPC + async write |
| Create | ~3-5ms | gRPC roundtrip |

### 5.2 本地缓存策略

- **仅缓存读操作**，写操作直接透传
- 基于 inode 的 LRU 淘汰策略
- TTL 过期自动失效（默认 30 秒）
- 可配置缓存大小上限（默认 100MB）

---

## 六、环境变量

| 变量名 | 必需 | 说明 | 默认值 |
|--------|------|------|--------|
| `STORAGE_PROXY_BASE_URL` | ✅ | Storage Proxy 服务地址 | - |
| `STORAGE_PROXY_REPLICAS` | ✅ | Storage Proxy 副本数 | - |
| `NODE_NAME` | ❌ | 当前节点名（用于亲和性路由） | - |
| `CACHE_MAX_BYTES` | ❌ | 本地缓存大小限制 | 104857600 (100MB) |
| `CACHE_TTL_SECONDS` | ❌ | 缓存 TTL | 30 |

---

## 七、错误定义

| 错误 | 说明 |
|------|------|
| `sandboxvolume_already_mounted` | 卷已挂载 |
| `sandboxvolume_not_mounted` | 卷未挂载 |
| `invalid_mount_point` | 无效的挂载点 |
| `mount_timeout` | 挂载超时 |
| `unmount_failed` | 卸载失败 |
| `grpc_connection_failed` | gRPC 连接失败 |

---

## 八、优势总结

| 特性 | 说明 |
|------|------|
| **零凭证** | Procd 不持有任何 S3/PG 凭证 |
| **轻量级** | 只负责 FUSE + gRPC 客户端 |
| **网络隔离** | Packet marking 绕过用户规则 |
| **快速挂载** | <50ms 延迟 |
| **集中式存储** | 所有存储逻辑在 Proxy |
