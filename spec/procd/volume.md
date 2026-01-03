# Procd - Volume 管理设计规范

## 一、设计目标

Sandbox0Volume 是独立于 Sandbox 的持久化存储资源，支持：
1. **高性能冷启动**：Copy-on-Write 分层 + OverlayFS，实现 <100ms 恢复
2. **多租户隔离**：数据加密 + 访问控制
3. **独立生命周期**：Volume 可独立创建、删除，不被 Sandbox 绑定
4. **灵活挂载**：一个 Volume 可被多个 Sandbox 挂载（只读），一个 Sandbox 可挂载多个 Volume
5. **快速快照**：Snapshot 只存储元数据引用，无需复制数据

---

## 二、核心概念

### 2.1 资源模型

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Sandbox0Volume 资源模型                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Volume (独立资源)                                                          │
│  ├── ID: vol-abc123                                                        │
│  ├── Name: my-workspace                                                    │
│  ├── TeamID: team-456                                                      │
│  ├── BaseLayerID: base-py311 (只读)                                         │
│  ├── WorkingLayerID: working-xyz789 (读写)                                  │
│  └── Snapshots: [snap-001, snap-002, ...]                                  │
│                                                                             │
│  Layer (分层存储)                                                           │
│  ├── Base Layer (只读，可复用)                                              │
│  │   ├── node_modules/                                                     │
│  │   ├── venv/                                                             │
│  │   └── 基础环境                                                           │
│  │                                                                          │
│  ├── Delta Layer (只读，可复用)                                             │
│  │   ├── + new_file.py                                                     │
│  │   └── - deleted_file.js                                                 │
│  │                                                                          │
│  └── Working Layer (读写，当前修改)                                          │
│      ├── + modified.json                                                   │
│      └── workspace/                                                        │
│                                                                             │
│  Snapshot (快照，元数据引用)                                                │
│  ├── ID: snap-001                                                          │
│  ├── LayerID: delta-def456  # 指向某个 Delta Layer                         │
│  └── 无数据复制                                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 关系模型

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Volume 与 Sandbox 的关系                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Volume (vol-123)                Volume (vol-456)                           │
│       │                                 │                                  │
│       ├──► Sandbox (sb-a) [读写]         ├──► Sandbox (sb-c) [读写]          │
│       ├──► Sandbox (sb-b) [只读]         └──► Sandbox (sb-d) [只读]          │
│       │                                  │                                  │
│       └──► 独立存在（未被挂载）            └──► 独立存在（未被挂载）           │
│                                                                             │
│  关系说明：                                                                 │
│  - 1 个 Volume 可被多个 Sandbox 挂载                                        │
│  - 第一个挂载者获得读写权限，后续挂载者只读                                  │
│  - 1 个 Sandbox 可挂载多个 Volume                                           │
│  - Sandbox 删除不影响 Volume                                                │
│  - Volume 可独立存在，不被任何 Sandbox 挂载                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 存储架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        存储架构 (S3 + 本地缓存)                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  S3 (持久化存储，事实来源)                                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ s3://sandbox0-volumes/                                                │  │
│  │ ├── volumes/                                                          │  │
│  │ │   ├── vol-123/                                                      │  │
│  │ │   │   ├── layers/                                                   │  │
│  │ │   │   │   ├── base-py311/          (Base Layer，500MB)             │  │
│  │ │   │   │   ├── delta-pip-pkgs/     (Delta Layer，50MB)              │  │
│  │ │   │   │   └── working-current/    (Working Layer，定期上传)         │  │
│  │ │   │   └── metadata.json             (Volume 元数据)                 │  │
│  │ │   └── vol-456/...                                                   │  │
│  │ └── snapshots/                                                         │  │
│  │     ├── snap-001-metadata.json  (只存元数据，指向 delta-layer)          │  │
│  │     └── snap-002-metadata.json                                         │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                   │                                          │
│                                   │ 按需下载                                  │
│                                   ▼                                          │
│  本地缓存 (Procd 容器内)                                                     │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ /var/lib/sandbox0/                                                    │  │
│  │ ├── preload/              (预热缓存，空闲池 Pod 启动时填充)             │  │
│  │ │   ├── base-py311/                                                    │  │
│  │ │   └── layer-node-modules/                                           │  │
│  │ ├── volumes/              (运行时缓存)                                   │  │
│  │ │   └── vol-123/                                                     │  │
│  │ │       ├── layers/                                                   │  │
│  │ │       │   ├── base-py311@       (硬链接到 preload)                   │  │
│  │ │       │   ├── delta-pip-pkgs@    (按需下载)                          │  │
│  │ │       │   └── working-xyz789/    (读写层)                           │  │
│  │ │       └── merged/            (OverlayFS 挂载点)                      │  │
│  │ └── cache/               (通用缓存)                                      │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                   │                                          │
│                    ┌───────────────┴───────────────┐                        │
│                    │   bind mount (host)          │                        │
│                    └───────────────┬───────────────┘                        │
│                                   ▼                                          │
│  用户容器看到                                                               │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ /workspace  ← OverlayFS 合并后的完整文件系统                             │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、数据结构定义

### 3.1 Volume

```go
// Volume Sandbox0 持久卷（独立资源，存储在 PGSQL）
type Volume struct {
    // 基本属性
    ID          string    `json:"id" db:"id"`
    Name        string    `json:"name" db:"name"`
    TeamID      string    `json:"team_id" db:"team_id"`
    Description string    `json:"description,omitempty" db:"description"`

    // 存储配置
    S3Bucket    string    `json:"s3_bucket" db:"s3_bucket"`
    S3Prefix    string    `json:"s3_prefix" db:"s3_prefix"`  // 如: volumes/team-123/my-workspace
    Capacity    string    `json:"capacity" db:"capacity"`    // 如: "10Gi"

    // 分层链
    BaseLayerID    string `json:"base_layer_id" db:"base_layer_id"`       // 根层
    WorkingLayerID string `json:"working_layer_id" db:"working_layer_id"` // 当前工作层

    // 加密配置
    EncryptionKeyID string `json:"encryption_key_id" db:"encryption_key_id"` // KMS 密钥 ID

    // 访问控制
    ReadOnly    bool     `json:"read_only" db:"read_only"`                // 只读模式
    AllowedSandboxIDs []string `json:"allowed_sandbox_ids" db:"allowed_sandbox_ids"` // 白名单

    // 标签和元数据
    Tags        []string          `json:"tags,omitempty" db:"tags"`
    Metadata    map[string]string `json:"metadata,omitempty" db:"metadata"`

    // 时间戳
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
    LastAccessedAt time.Time `json:"last_accessed_at" db:"last_accessed_at"`

    // 统计
    SizeBytes   int64 `json:"size_bytes" db:"size_bytes"`
    FileCount   int32 `json:"file_count" db:"file_count"`
    MountCount  int32 `json:"mount_count" db:"mount_count"`  // 当前挂载数
}

// VolumeStatus Volume 状态（运行时，不在 DB 中）
type VolumeStatus struct {
    VolumeID    string
    IsMounted   bool
    MountedBy   []string  // 当前挂载的 Sandbox ID 列表
    CacheStatus *VolumeCacheStatus
}

// VolumeCacheStatus 缓存状态
type VolumeCacheStatus struct {
    LocalPath   string
    CacheSize   int64
    LayersCached []string  // 已缓存的 Layer ID
    IsPreloaded bool       // 是否来自预热缓存
}
```

### 3.2 Layer

```go
// Layer 存储层（可以是 Base、Delta、Working）
type Layer struct {
    ID          string    `json:"id"`
    VolumeID    string    `json:"volume_id"`
    Type        LayerType `json:"type"`

    // 分层链
    BaseLayerID string    `json:"base_layer_id,omitempty"` // 父层 ID

    // 变更记录（增量同步用）
    Changes *LayerChanges `json:"changes,omitempty"`

    // 存储位置
    S3Path      string `json:"s3_path,omitempty"`      // S3 路径 (base/delta)
    LocalPath   string `json:"local_path,omitempty"`   // 本地路径 (working)

    // 统计
    SizeBytes   int64 `json:"size_bytes"`
    FileCount   int32 `json:"file_count"`

    // 校验
    Checksum    string `json:"checksum"`  // SHA256 整体校验

    // 时间戳
    CreatedAt   time.Time `json:"created_at"`
}

type LayerType string

const (
    LayerTypeBase    LayerType = "base"    // 基础层（只读，可复用）
    LayerTypeDelta   LayerType = "delta"   // 增量层（只读，可复用）
    LayerTypeWorking LayerType = "working" // 工作层（读写）
)

// LayerChanges 层的变更记录
type LayerChanges struct {
    AddedFiles     []string `json:"added_files,omitempty"`     // 新增文件
    ModifiedFiles  []string `json:"modified_files,omitempty"`  // 修改文件
    DeletedFiles   []string `json:"deleted_files,omitempty"`   // 删除文件

    // 每个文件的元数据（用于增量上传）
    FileMetadata map[string]*FileMetadata `json:"file_metadata,omitempty"`
}

// FileMetadata 文件元数据
type FileMetadata struct {
    Path       string `json:"path"`
    Size       int64  `json:"size"`
    Checksum   string `json:"checksum"`  // SHA256
    Modified   bool   `json:"modified"`  // 是否被修改
    Encrypted  bool   `json:"encrypted"` // 是否加密
}
```

### 3.3 Snapshot

```go
// Snapshot 快照（只存元数据引用，不复制数据）
type Snapshot struct {
    ID          string    `json:"id" db:"id"`
    VolumeID    string    `json:"volume_id" db:"volume_id"`
    Name        string    `json:"name" db:"name"`
    Description string    `json:"description,omitempty" db:"description"`

    // 核心：Snapshot 只存储 Layer 引用
    LayerID     string `json:"layer_id" db:"layer_id"` // 指向某个 Delta Layer

    // 可选：标签和元数据
    Tags        []string          `json:"tags,omitempty" db:"tags"`
    Metadata    map[string]string `json:"metadata,omitempty" db:"metadata"`

    // 时间戳
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`

    // 统计
    SizeBytes   int64 `json:"size_bytes" db:"size_bytes"`   // 引用的 Layer 大小
    FileCount   int32 `json:"file_count" db:"file_count"`   // 文件数量
}

// SnapshotRestoreConfig 快照恢复配置
type SnapshotRestoreConfig struct {
    SnapshotID      string `json:"snapshot_id"`
    TargetLayerID   string `json:"target_layer_id,omitempty"`  // 可选，恢复到指定 Layer
    CreateNewWorking bool   `json:"create_new_working"`        // 是否创建新的 Working Layer
}
```

---

## 四、VolumeManager 实现

### 4.1 核心接口

```go
// VolumeManager 卷管理器（集成在 Procd 内）
type VolumeManager struct {
    // S3 客户端
    s3Client *s3.Client

    // 加密管理器
    crypto *CryptoManager

    // 本地缓存目录
    cacheRoot   string  // /var/lib/sandbox0
    preloadRoot string  // /var/lib/sandbox0/preload

    // 运行中的 volumes
    volumes sync.Map  // volumeID -> *Volume

    // 配置
    config *VolumeManagerConfig
}

type VolumeManagerConfig struct {
    S3Region          string
    S3Endpoint        string
    CacheSizeLimit    string  // 如 "10Gi"
    PreloadDir        string
}

// Mount 挂载 Volume
func (vm *VolumeManager) Mount(ctx context.Context, req *MountRequest) (*MountResponse, error)

// Unmount 卸载 Volume
func (vm *VolumeManager) Unmount(ctx context.Context, volumeID string) error

// CreateSnapshot 创建快照
func (vm *VolumeManager) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (*Snapshot, error)

// RestoreSnapshot 恢复快照
func (vm *VolumeManager) RestoreSnapshot(ctx context.Context, req *RestoreSnapshotRequest) error

// ListSnapshots 列出快照
func (vm *VolumeManager) ListSnapshots(ctx context.Context, volumeID string) ([]*Snapshot, error)
```

### 4.2 Mount 实现

```go
// MountRequest 挂载请求
type MountRequest struct {
    VolumeID    string `json:"volume_id"`
    SandboxID   string `json:"sandbox_id"`
    MountPoint  string `json:"mount_point"`  // 如: /workspace
    ReadOnly    bool   `json:"read_only,omitempty"`
    SnapshotID  string `json:"snapshot_id,omitempty"`  // 可选，从快照恢复

    // 预热配置（来自 Template）
    WarmupConfig *VolumeWarmupConfig `json:"warmup_config,omitempty"`
}

// MountResponse 挂载响应
type MountResponse struct {
    VolumeID    string `json:"volume_id"`
    MountPoint  string `json:"mount_point"`
    LayerChain  []string `json:"layer_chain"`  // 当前 Layer 链
    IsFromCache bool    `json:"is_from_cache"`  // 是否来自预热缓存
}

// Mount 挂载 Volume（高性能，<100ms）
func (vm *VolumeManager) Mount(ctx context.Context, req *MountRequest) (*MountResponse, error) {
    // 1. 加载 Volume 元数据（从 PG 或本地缓存）
    volume, err := vm.loadVolume(ctx, req.VolumeID)
    if err != nil {
        return nil, fmt.Errorf("load volume: %w", err)
    }

    // 2. 检查是否只读挂载
    if volume.MountCount > 0 && !req.ReadOnly {
        return nil, fmt.Errorf("volume already mounted in read-write mode")
    }

    // 3. 如果指定了 SnapshotID，先恢复
    if req.SnapshotID != "" {
        if err := vm.RestoreSnapshot(ctx, &RestoreSnapshotRequest{
            VolumeID:   req.VolumeID,
            SnapshotID: req.SnapshotID,
        }); err != nil {
            return nil, fmt.Errorf("restore snapshot: %w", err)
        }
    }

    // 4. 准备 Layer 链（按需下载）
    layerDirs, err := vm.prepareLayerChain(ctx, volume, req.WarmupConfig)
    if err != nil {
        return nil, fmt.Errorf("prepare layer chain: %w", err)
    }

    // 5. 创建 Working Layer（如果不存在）
    if volume.WorkingLayerID == "" || req.ReadOnly {
        newWorking, err := vm.createWorkingLayer(ctx, volume)
        if err != nil {
            return nil, fmt.Errorf("create working layer: %w", err)
        }
        volume.WorkingLayerID = newWorking.ID
    }

    // 6. 挂载 OverlayFS
    mergedPath := filepath.Join(vm.cacheRoot, "volumes", volume.ID, "merged")
    if err := vm.mountOverlayFS(mergedPath, layerDirs, volume.WorkingLayerID, req.ReadOnly); err != nil {
        return nil, fmt.Errorf("mount overlay: %w", err)
    }

    // 7. Bind mount 到用户容器
    if err := syscall.Mount(mergedPath, req.MountPoint, "none", syscall.MS_BIND, ""); err != nil {
        return nil, fmt.Errorf("bind mount: %w", err)
    }

    // 8. 更新 Volume 状态
    volume.MountCount++
    volume.LastAccessedAt = time.Now()

    return &MountResponse{
        VolumeID:    volume.ID,
        MountPoint:  req.MountPoint,
        LayerChain:  vm.getLayerChainIDs(volume),
        IsFromCache: vm.isFromPreloadCache(volume),
    }, nil
}

// prepareLayerChain 准备 Layer 链（异步下载，不阻塞冷启动）
func (vm *VolumeManager) prepareLayerChain(ctx context.Context, volume *Volume, warmup *VolumeWarmupConfig) ([]string, error) {
    var layerDirs []string
    current := vm.getLayer(volume.BaseLayerID)

    // 从底到顶遍历 Layer 链
    for current != nil {
        var localPath string

        // 1. 检查是否在预热缓存中
        if warmup != nil && warmup.Enabled {
            if cachedPath := vm.checkPreloadCache(current.ID); cachedPath != "" {
                localPath = cachedPath
                vm.metrics.CacheHits.Inc()
            }
        }

        // 2. 检查是否已下载
        if localPath == "" {
            localPath = filepath.Join(vm.cacheRoot, "layers", current.ID)
            if !exists(localPath) {
                // 3. 按需下载（异步）
                if err := vm.downloadLayerAsync(ctx, current, localPath); err != nil {
                    return nil, err
                }
            }
        }

        layerDirs = append([]string{localPath}, layerDirs...)

        // 移动到上一层
        if current.BaseLayerID != "" {
            current = vm.getLayer(current.BaseLayerID)
        } else {
            break
        }
    }

    return layerDirs, nil
}

// downloadLayerAsync 异步下载 Layer（不阻塞挂载）
func (vm *VolumeManager) downloadLayerAsync(ctx context.Context, layer *Layer, localPath string) error {
    // 如果是 Working Layer，不需要下载
    if layer.Type == LayerTypeWorking {
        return nil
    }

    // 创建目录
    os.MkdirAll(localPath, 0755)

    // 异步下载
    go func() {
        // 从 S3 下载
        if err := vm.downloadLayerFromS3(context.Background(), layer, localPath); err != nil {
            log.Printf("download layer %s failed: %v", layer.ID, err)
            return
        }

        // 解密（如果需要）
        if layer.Encrypted {
            if err := vm.crypto.DecryptLayer(localPath); err != nil {
                log.Printf("decrypt layer %s failed: %v", layer.ID, err)
            }
        }
    }()

    return nil
}

// mountOverlayFS 挂载 OverlayFS（内核级 CoW，高性能）
func (vm *VolumeManager) mountOverlayFS(mergedPath string, lowerDirs []string, workingLayerID string, readOnly bool) error {
    // 创建目录
    os.MkdirAll(mergedPath, 0755)
    upperDir := filepath.Join(vm.cacheRoot, "layers", workingLayerID, "upper")
    workDir := filepath.Join(vm.cacheRoot, "layers", workingLayerID, "work")
    os.MkdirAll(upperDir, 0755)
    os.MkdirAll(workDir, 0755)

    // 拼接 lowerdir
    lowerdir := strings.Join(lowerDirs, ":")

    // 挂载选项
    opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerdir, upperDir, workDir)
    if readOnly {
        opts = fmt.Sprintf("lowerdir=%s", lowerdir)  // 只读模式不需要 upper/work
    }

    // 挂载 OverlayFS
    return syscall.Mount("overlay", mergedPath, "overlay", 0, opts)
}
```

### 4.3 Snapshot 实现

```go
// CreateSnapshot 创建快照（元数据引用，~0ms）
func (vm *VolumeManager) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (*Snapshot, error) {
    volume := vm.getVolume(req.VolumeID)

    // 1. 将当前 Working Layer 转换为 Delta Layer（如果未上传）
    workingLayer := vm.getLayer(volume.WorkingLayerID)

    // 2. 上传 Working Layer 到 S3（如果未上传）
    if workingLayer.S3Path == "" {
        if err := vm.uploadLayerToS3(ctx, workingLayer); err != nil {
            return nil, fmt.Errorf("upload working layer: %w", err)
        }
        workingLayer.Type = LayerTypeDelta
    }

    // 3. 创建 Snapshot（只存元数据）
    snapshot := &Snapshot{
        ID:       generateID("snap"),
        VolumeID: volume.ID,
        Name:     req.Name,
        LayerID:  workingLayer.ID,  // 关键：只引用 Layer
        Metadata: req.Metadata,
        CreatedAt: time.Now(),
    }

    // 4. 保存到 S3（只是元数据文件）
    snapshotPath := fmt.Sprintf("snapshots/%s.json", snapshot.ID)
    if err := vm.uploadSnapshotMetadata(ctx, snapshot, snapshotPath); err != nil {
        return nil, err
    }

    return snapshot, nil
}

// RestoreSnapshot 恢复快照（指针切换，~10ms）
func (vm *VolumeManager) RestoreSnapshot(ctx context.Context, req *RestoreSnapshotRequest) error {
    // 1. 加载 Snapshot 元数据
    snapshot, err := vm.loadSnapshot(ctx, req.SnapshotID)
    if err != nil {
        return err
    }

    // 2. 验证 Layer 存在
    targetLayer := vm.getLayer(snapshot.LayerID)
    if targetLayer == nil {
        return fmt.Errorf("layer %s not found", snapshot.LayerID)
    }

    // 3. 关键优化：只需更新指针，无需下载数据
    volume := vm.getVolume(req.VolumeID)
    volume.WorkingLayerID = snapshot.LayerID

    // 4. 如果需要创建新的 Working Layer
    if req.CreateNewWorking {
        newWorking, err := vm.createWorkingLayer(ctx, volume)
        if err != nil {
            return err
        }
        volume.WorkingLayerID = newWorking.ID
    }

    // 5. 异步准备 Layer（Lazy Load）
    go vm.prepareLayerChainAsync(ctx, volume, nil)

    return nil
}
```

### 4.4 预热实现

```go
// PreloadLayers 预加载 Base Layer（空闲池 Pod 启动时调用）
func (vm *VolumeManager) PreloadLayers(ctx context.Context, req *PreloadRequest) error {
    for _, layerID := range req.BaseLayerIDs {
        // 检查是否已缓存
        cachedPath := filepath.Join(vm.preloadRoot, layerID)
        if exists(cachedPath) {
            continue
        }

        // 获取 Layer 元数据
        layer, err := vm.getLayerMetadata(ctx, layerID)
        if err != nil {
            return err
        }

        // 下载到预热缓存
        if err := vm.downloadLayerFromS3(ctx, layer, cachedPath); err != nil {
            return err
        }

        log.Printf("Preloaded layer %s to %s", layerID, cachedPath)
    }

    return nil
}

type PreloadRequest struct {
    BaseLayerIDs []string `json:"base_layer_ids"`
    RefreshInterval string `json:"refresh_interval,omitempty"`  // 如 "24h"
}

// RefreshPreloadedCache 刷新预热缓存（定期调用）
func (vm *VolumeManager) RefreshPreloadedCache(ctx context.Context, layerIDs []string) error {
    for _, layerID := range layerIDs {
        // 获取最新 Layer 元数据
        layer, err := vm.getLayerMetadata(ctx, layerID)
        if err != nil {
            continue
        }

        // 检查本地版本是否过期
        cachedPath := filepath.Join(vm.preloadRoot, layerID)
        if vm.isLayerExpired(cachedPath, layer.Checksum) {
            // 重新下载
            os.RemoveAll(cachedPath)
            if err := vm.downloadLayerFromS3(ctx, layer, cachedPath); err != nil {
                log.Printf("Refresh layer %s failed: %v", layerID, err)
            }
        }
    }

    return nil
}
```

---

## 五、性能优化总结

| 操作 | 传统方案 | Sandbox0Volume 优化 | 提升 |
|------|----------|---------------------|------|
| **Snapshot** | tar + upload (数秒~分钟) | 元数据引用 (~0ms) | **1000x+** |
| **Restore** | download + untar (数秒~分钟) | 指针切换 (~10ms) | **1000x+** |
| **Cold Start** | 10s~1min | **<100ms** | **100x+** |
| **首次访问** | N/A | Lazy Load (~100ms) | - |

### 关键技术

1. **Copy-on-Write 分层**：Snapshot 只存元数据引用
2. **OverlayFS**：内核级 CoW，零拷贝合并
3. **Lazy Load**：按需下载 Layer，不阻塞冷启动
4. **预热缓存**：空闲池 Pod 预加载常用 Layer
5. **异步上传**：Working Layer 定期后台上传

---

## 六、完整流程（<100ms 冷启动）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│              完整的冷启动流程（<100ms）                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 用户请求创建 Sandbox                                                    │
│     POST /api/v1/sandboxes/claim                                            │
│     {                                                                       │
│         "template_id": "python-dev",                                        │
│         "volume_config": {                                                  │
│             "snapshot_id": "snap-latest"  # 可选，指定恢复的快照            │
│         }                                                                   │
│     }                                                                       │
│                                                                             │
│  2. Manager 认领空闲池 Pod (~10ms)                                          │
│     - 更新 labels: idle → active                                            │
│     - 传递 Volume 配置给 Procd                                              │
│                                                                             │
│  3. Manager 调用 Procd API (~5ms)                                           │
│     POST /api/v1/volumes/vol-123/mount                                      │
│     {                                                                       │
│         "sandbox_id": "sb-abc",                                            │
│         "snapshot_id": "snap-latest",                                       │
│         "warmup_config": {...}                                              │
│     }                                                                       │
│                                                                             │
│  4. Procd VolumeManager 处理 (~20ms)                                        │
│     a. 恢复 Snapshot（指针切换）                                            │
│     b. 检查预热缓存（命中）                                                  │
│     c. 挂载 OverlayFS（内核 CoW）                                           │
│     d. Bind mount 到用户容器                                                │
│                                                                             │
│  5. Sandbox Ready (~40ms 总耗时)                                            │
│     ────────────────────────────────────────────────────────────────────   │
│     用户看到完整的文件系统，可以立即使用                                      │
│                                                                             │
│  6. 后台异步任务（不阻塞用户）                                               │
│     - Lazy Load: 首次访问时下载 Layer                                       │
│     - Writeback: 定期上传 Working Layer 到 S3                               │
│     - Cache Refresh: 定期更新预热缓存                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 七、多租户数据加密

### 7.1 加密策略

```go
// CryptoManager 加密管理器
type CryptoManager struct {
    kmsClient kms.Client
    cache     sync.Map  // volumeID -> encryptionKey
}

// EncryptLayer 加密 Layer（客户端加密）
func (cm *CryptoManager) EncryptLayer(layerPath string, key []byte) error {
    // 1. 遍历所有文件
    // 2. 分块加密（AES-256-GCM）
    // 3. 上传加密后的数据到 S3
    return nil
}

// DecryptLayer 解密 Layer（按需解密）
func (cm *CryptoManager) DecryptLayer(layerPath string) error {
    // Lazy Decrypt: 首次访问文件时才解密
    return nil
}
```

### 7.2 密钥管理

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        密钥管理架构                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  KMS (AWS KMS / GCP KMS / Vault)                                           │
│       │                                                                     │
│       │ 1. Volume 创建时生成密钥                                            │
│       ▼                                                                     │
│  Procd CryptoManager                                                        │
│       │                                                                     │
│       │ 2. 从 KMS 获取密钥，缓存在内存                                       │
│       ▼                                                                     │
│  Volume 加密                                                                │
│       │                                                                     │
│       │ 3. 每个 Volume 独立的 AES-256-GCM 密钥                              │
│       │ 4. 每个 Layer 独立的 IV（初始化向量）                                │
│       ▼                                                                     │
│  S3 加密存储                                                                │
│                                                                             │
│  安全策略：                                                                 │
│  - 密钥不出 Procd 内存                                                     │
│  - Procd 退出时密钥清空                                                     │
│  - 支持密钥轮换                                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 八、与 Manager 的交互

### 8.1 Volume 元数据存储

Volume 的元数据存储在 PGSQL 中，由 Manager 管理：

```sql
-- Volumes 表（由 Manager 管理）
CREATE TABLE volumes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    team_id TEXT NOT NULL,
    s3_bucket TEXT NOT NULL,
    s3_prefix TEXT NOT NULL,
    base_layer_id TEXT NOT NULL,
    working_layer_id TEXT,
    encryption_key_id TEXT,
    ...
);
```

### 8.2 Procd 调用流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Manager → Procd Volume 挂载流程                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. Manager 从 PG 获取 Volume 元数据                                        │
│     SELECT * FROM volumes WHERE id = 'vol-123';                             │
│                                                                             │
│  2. Manager 调用 Procd API 挂载 Volume                                     │
│     POST /api/v1/volumes/vol-123/mount                                      │
│     {                                                                       │
│         "sandbox_id": "sb-abc",                                            │
│         "mount_point": "/workspace",                                        │
│         "s3_bucket": "sandbox0-volumes",                                   │
│         "s3_prefix": "volumes/vol-123",                                     │
│         "base_layer_id": "base-py311",                                      │
│         "snapshot_id": "snap-latest"                                        │
│     }                                                                       │
│                                                                             │
│  3. Procd VolumeManager 处理挂载                                            │
│     - 从 S3 下载 Layer（或使用缓存）                                        │
│     - 创建 OverlayFS                                                       │
│     - Bind mount 到指定路径                                                 │
│                                                                             │
│  4. 返回挂载结果                                                            │
│     {                                                                       │
│         "volume_id": "vol-123",                                             │
│         "mount_point": "/workspace",                                        │
│         "layer_chain": ["base-py311", "working-xyz"]                        │
│     }                                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 九、HTTP API（Procd 提供的接口）

```http
# 挂载 Volume
POST /api/v1/volumes/{volume_id}/mount
{
    "sandbox_id": "sb-abc123",
    "mount_point": "/workspace",
    "read_only": false,
    "snapshot_id": "snap-001",
    "warmup_config": {
        "enabled": true,
        "base_layer_ids": ["base-python-3.11"]
    }
}

# 卸载 Volume
POST /api/v1/volumes/{volume_id}/unmount

# 获取 Volume 状态
GET /api/v1/volumes/{volume_id}

# 创建快照
POST /api/v1/volumes/{volume_id}/snapshots

# 恢复快照
POST /api/v1/volumes/{volume_id}/restore

# 预加载 Layer
POST /api/v1/volumes/preload
```
