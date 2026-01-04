# Procd - Volume Management (RemoteFS Client)

## 一、设计目标

Procd 的 VolumeManager 负责将远程文件系统挂载到 Pod 内的指定路径。实际的存储操作（S3、PostgreSQL）完全由 **Storage Proxy** 处理，Procd 只是一个客户端。

### 核心原则

1. **零存储凭证**：Procd 不持有任何 S3、PostgreSQL 凭证
2. **轻量级**：只负责 FUSE 挂载和 gRPC 客户端
3. **网络隔离兼容**：通过 packet marking 绕过用户网络规则
4. **快速挂载**：<50ms 挂载延迟

---

## 二、架构设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Procd Volume Architecture                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Procd (PID=1, in Pod)                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ VolumeManager                                                          │   │
│  │ ┌───────────────────────────────────────────────────────────────┐   │   │
│  │ │ Mount/Unmount API                                              │   │   │
│  │ │  - Mount(volumeID, mountPoint, token)                         │   │   │
│  │ │  - Unmount(volumeID)                                           │   │   │
│  │ └───────────────────────────────────────────────────────────────┘   │   │
│  │                           │                                            │   │
│  │                           ▼                                            │   │
│  │ ┌───────────────────────────────────────────────────────────────┐   │   │
│  │ │ RemoteFS (FUSE filesystem)                                     │   │   │
│  │ │  ├── Implements fuse.Filesystem interface                     │   │   │
│  │ │  ├── Forwards all operations to gRPC client                   │   │   │
│  │ │  └─→ gRPC call to Storage Proxy                               │   │   │
│  │ └───────────────────────────────────────────────────────────────┘   │   │
│  │                           │                                            │   │
│  │                           ▼                                            │   │
│  │ ┌───────────────────────────────────────────────────────────────┐   │   │
│  │ │ gRPC Client                                                    │   │   │
│  │ │  ├── Connection to Storage Proxy                              │   │   │
│  │ │  ├── Packet marking (SO_MARK=0x2)                             │   │   │
│  │ │  └─→ Bypass nftables rules                                     │   │   │
│  │ └───────────────────────────────────────────────────────────────┘   │   │
│  │                                                                        │   │
│  │ /workspace (FUSE mount point)                                         │   │
│  │ └─→ User files accessed here                                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │ gRPC (mark=0x2)                        │
│                                    ▼                                        │
│                          Storage Proxy                                     │
│                          (Has all credentials)                            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、数据结构定义

### 3.1 VolumeManager

```go
// VolumeManager manages remote filesystem mounts in Procd
type VolumeManager struct {
    mu     sync.RWMutex
    mounts map[string]*MountContext  // volumeID -> MountContext

    // Configuration
    proxyURL string  // Storage Proxy gRPC address
}

// MountContext represents an active mount
type MountContext struct {
    VolumeID   string
    MountPoint string
    Token      string  // JWT auth token (in-memory only)

    // FUSE
    fuseConn  *fuse.Conn
    fuseServerCancel context.CancelFunc

    // gRPC client
    grpcClient fs.FileSystemClient

    MountedAt time.Time
}
```

### 3.2 Mount Request/Response

```go
// MountRequest request to mount a volume
type MountRequest struct {
    VolumeID   string `json:"volume_id"`
    SandboxID  string `json:"sandbox_id"`
    MountPoint string `json:"mount_point"`  // e.g., "/workspace"
    Token      string `json:"token"`        // JWT auth token from Manager
}

// MountResponse response for mount request
type MountResponse struct {
    VolumeID   string `json:"volume_id"`
    MountPoint string `json:"mount_point"`
    MountedAt  string `json:"mounted_at"`  // ISO timestamp
}

// UnmountRequest request to unmount a volume
type UnmountRequest struct {
    VolumeID string `json:"volume_id"`
}
```

---

## 四、RemoteFS 实现

### 4.1 RemoteFS 结构

```go
// RemoteFS implements fuse.Filesystem via gRPC client
type RemoteFS struct {
    client   fs.FileSystemClient  // gRPC client
    volumeID string
    token    string
    rootInode string
}

// RemoteFSNode represents a file/directory node
type RemoteFSNode struct {
    inode string
    fs    *RemoteFS
    attr  *fs.GetAttrResponse
}

// RemoteFileHandle represents an open file
type RemoteFileHandle struct {
    inode string
    fs    *RemoteFS
    id    uint64  // Handle ID
}
```

### 4.2 FUSE Operations (gRPC Client)

```go
// Attr implements fs.Node
func (n *RemoteFSNode) Attr(ctx context.Context, a *fuse.Attr) error {
    req := &fs.GetAttrRequest{Inode: n.inode}
    resp, err := n.fs.client.GetAttr(withAuth(ctx, n.fs.token), req)
    if err != nil {
        return err
    }

    a.Inode = resp.Ino
    a.Mode = syscallMode(resp.Mode)
    a.Size = resp.Size
    a.Mtime = time.Unix(resp.MtimeSec, resp.MtimeNsec)
    a.Atime = time.Unix(resp.AtimeSec, resp.AtimeNsec)
    a.Ctime = time.Unix(resp.CtimeSec, resp.CtimeNsec)
    a.Nlink = resp.Nlink
    a.Uid = resp.Uid
    a.Gid = resp.Gid
    a.Rdev = resp.Rdev
    a.Blocks = resp.Blocks

    return nil
}

// Create implements fs.NodeCreater
func (n *RemoteFSNode) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
    creq := &fs.CreateRequest{
        Parent: n.inode,
        Name:   req.Name,
        Mode:   uint32(req.Mode),
        Flags:  uint32(req.Flags),
    }

    cresp, err := n.fs.client.Create(withAuth(ctx, n.fs.token), creq)
    if err != nil {
        return nil, nil, err
    }

    node := &RemoteFSNode{
        inode: cresp.Inode,
        fs:    n.fs,
        attr:  cresp.Attr,
    }

    handle := &RemoteFileHandle{
        inode: cresp.Inode,
        fs:    n.fs,
        id:    cresp.HandleId,
    }

    resp.Attr = fuseAttrFrom(cresp.Attr)

    return node, handle, nil
}

// Lookup implements fs.NodeRequestLookuper
func (n *RemoteFSNode) Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (fs.Node, error) {
    lreq := &fs.LookupRequest{
        Parent: n.inode,
        Name:   req.Name,
    }

    lresp, err := n.fs.client.Lookup(withAuth(ctx, n.fs.token), lreq)
    if err != nil {
        return nil, err
    }

    node := &RemoteFSNode{
        inode: lresp.Inode,
        fs:    n.fs,
        attr:  lresp.Attr,
    }

    resp.Attr = fuseAttrFrom(lresp.Attr)
    resp.EntryValid = time.Hour

    return node, nil
}

// Read implements fs.HandleReader
func (h *RemoteFileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
    rreq := &fs.ReadRequest{
        Inode:    h.inode,
        HandleId: h.id,
        Offset:   req.Offset,
        Size:     int64(len(resp.Data)),
    }

    rresp, err := h.fs.client.Read(withAuth(ctx, h.fs.token), rreq)
    if err != nil {
        return err
    }

    n := copy(resp.Data, rresp.Data)
    resp.Data = resp.Data[:n]

    return nil
}

// Write implements fs.HandleWriter
func (h *RemoteFileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
    wreq := &fs.WriteRequest{
        Inode:    h.inode,
        HandleId: h.id,
        Offset:   req.Offset,
        Data:     req.Data,
    }

    wresp, err := h.fs.client.Write(withAuth(ctx, h.fs.token), wreq)
    if err != nil {
        return err
    }

    resp.Size = int(wresp.BytesWritten)
    return nil
}

// Mkdir implements fs.NodeMkdirer
func (n *RemoteFSNode) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
    mreq := &fs.MkdirRequest{
        Parent: n.inode,
        Name:   req.Name,
        Mode:   uint32(req.Mode),
    }

    mresp, err := n.fs.client.Mkdir(withAuth(ctx, n.fs.token), mreq)
    if err != nil {
        return nil, err
    }

    return &RemoteFSNode{
        inode: mresp.Inode,
        fs:    n.fs,
        attr:  mresp.Attr,
    }, nil
}

// Readdir implements fs.NodeReaddirer
func (n *RemoteFSNode) Readdir(ctx context.Context) (fuse.Dirent, error) {
    req := &fs.ReadDirRequest{
        Inode: n.inode,
    }

    resp, err := n.fs.client.ReadDir(withAuth(ctx, n.fs.token), req)
    if err != nil {
        return nil, err
    }

    var entries []fuse.Dirent
    for _, e := range resp.Entries {
        entries = append(entries, fuse.Dirent{
            Inode: e.Inode,
            Type:  fuse.DT_File,
            Name:  e.Name,
        })
    }

    return entries, nil
}

// Unlink implements fs.NodeRemover
func (n *RemoteFSNode) Unlink(ctx context.Context, req *fuse.RemoveRequest) error {
    ureq := &fs.UnlinkRequest{
        Parent: n.inode,
        Name:   req.Name,
    }

    _, err := n.fs.client.Unlink(withAuth(ctx, n.fs.token), ureq)
    return err
}

// Rename implements fs.NodeRenamer
func (n *RemoteFSNode) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
    newParent := newDir.(*RemoteFSNode).inode

    rreq := &fs.RenameRequest{
        OldParent: n.inode,
        OldName:   req.OldName,
        NewParent: newParent,
        NewName:   req.NewName,
    }

    _, err := n.fs.client.Rename(withAuth(ctx, n.fs.token), rreq)
    return err
}

// Flush implements fs.NodeFlusher
func (h *RemoteFileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
    freq := &fs.FlushRequest{
        HandleId: h.id,
    }

    _, err := h.fs.client.Flush(withAuth(ctx, h.fs.token), freq)
    return err
}

// Fsync implements fs.NodeFsyncer
func (h *RemoteFileHandle) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
    freq := &fs.FsyncRequest{
        HandleId: h.id,
        Datasync: req.Fdatasync,
    }

    _, err := h.fs.client.Fsync(withAuth(ctx, h.fs.token), freq)
    return err
}
```

### 4.3 Helper Functions

```go
// withAuth adds Bearer token to context
func withAuth(ctx context.Context, token string) context.Context {
    md := metadata.Pairs("authorization", "Bearer "+token)
    return metadata.NewOutgoingContext(ctx, md)
}

// syscallMode converts protobuf mode to syscall mode
func syscallMode(mode uint32) uint32 {
    return mode & 0777
}

// fuseAttrFrom converts protobuf attr to fuse.Attr
func fuseAttrFrom(attr *fs.GetAttrResponse) fuse.Attr {
    return fuse.Attr{
        Inode:       attr.Ino,
        Mode:       syscallMode(attr.Mode),
        Nlink:      attr.Nlink,
        Uid:        attr.Uid,
        Gid:        attr.Gid,
        Rdev:       attr.Rdev,
        Size:       attr.Size,
        Blocks:     attr.Blocks,
        Mtime:      time.Unix(attr.MtimeSec, attr.MtimeNsec),
        Atime:      time.Unix(attr.AtimeSec, attr.AtimeNsec),
        Ctime:      time.Unix(attr.CtimeSec, attr.CtimeNsec),
    }
}
```

---

## 五、VolumeManager 实现

### 5.1 Mount Volume

```go
// Mount mounts a remote filesystem
func (vm *VolumeManager) Mount(ctx context.Context, req *MountRequest) (*MountResponse, error) {
    vm.mu.Lock()
    defer vm.mu.Unlock()

    // Check if already mounted
    if _, exists := vm.mounts[req.VolumeID]; exists {
        return nil, fmt.Errorf("volume %s already mounted", req.VolumeID)
    }

    // Create gRPC connection with packet marking
    conn, err := vm.createGRPCConnection()
    if err != nil {
        return nil, fmt.Errorf("create grpc connection: %w", err)
    }

    client := fs.NewFileSystemClient(conn)

    // Create RemoteFS
    remoteFS := &RemoteFS{
        client:   client,
        volumeID: req.VolumeID,
        token:    req.Token,
        rootInode: "1",  // Root inode is always "1"
    }

    // Ensure mount point directory exists
    if err := os.MkdirAll(req.MountPoint, 0755); err != nil {
        return nil, fmt.Errorf("create mount point: %w", err)
    }

    // Mount FUSE
    fuseConn, err := fuse.Mount(req.MountPoint,
        fuse.FSName("sandbox0"),
        fuse.Subtype("remote"),
        fuse.LocalVolume(),
        fuse.AllowOther(),
    )
    if err != nil {
        return nil, fmt.Errorf("fuse mount: %w", err)
    }

    // Create context for FUSE server
    serverCtx, cancel := context.WithCancel(context.Background())

    // Serve FUSE in goroutine
    go func() {
        defer cancel()
        if err := fs.Serve(fuseConn, remoteFS); err != nil {
            log.Printf("FUSE serve error: %v", err)
        }
    }()

    // Wait for mount to be ready
    select {
    case <-fuseConn.Ready:
        if err := fuseConn.MountError; err != nil {
            fuseConn.Close()
            return nil, fmt.Errorf("mount ready: %w", err)
        }
    case <-time.After(10 * time.Second):
        fuseConn.Close()
        return nil, fmt.Errorf("mount timeout")
    }

    // Store mount context
    vm.mounts[req.VolumeID] = &MountContext{
        VolumeID:          req.VolumeID,
        MountPoint:        req.MountPoint,
        Token:             req.Token,
        FuseConn:          fuseConn,
        FuseServerCancel:  cancel,
        GrpcClient:        client,
        MountedAt:         time.Now(),
    }

    log.Printf("Mounted volume %s at %s", req.VolumeID, req.MountPoint)

    return &MountResponse{
        VolumeID:   req.VolumeID,
        MountPoint: req.MountPoint,
        MountedAt:  time.Now().Format(time.RFC3339),
    }, nil
}
```

### 5.2 Create gRPC Connection with Packet Marking

```go
// createGRPCConnection creates gRPC connection with packet marking
func (vm *VolumeManager) createGRPCConnection() (*grpc.ClientConn, error) {
    // Custom dialer that sets SO_MARK socket option
    dialer := &net.Dialer{
        Control: func(network, address string, c syscall.RawConn) error {
            var opErr error
            err := c.Control(func(fd uintptr) {
                // Set SO_MARK = 0x2 to bypass nftables rules
                opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 0x24, 0x2)
            })
            if err != nil {
                return err
            }
            return opErr
        },
    }

    // Create gRPC connection
    return grpc.Dial(vm.proxyURL,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
            return dialer.DialContext(ctx, "tcp", addr)
        }),
        grpc.WithDefaultCallOptions(
            grpc.MaxCallRecvMsgSize(100*1024*1024),  // 100MB max message size
        ),
    )
}
```

### 5.3 Unmount Volume

```go
// Unmount unmounts a volume
func (vm *VolumeManager) Unmount(ctx context.Context, volumeID string) error {
    vm.mu.Lock()
    defer vm.mu.Unlock()

    mountCtx, exists := vm.mounts[volumeID]
    if !exists {
        return fmt.Errorf("volume %s not mounted", volumeID)
    }

    // Cancel FUSE server
    mountCtx.FuseServerCancel()

    // Close FUSE connection
    if err := mountCtx.FuseConn.Close(); err != nil {
        log.Printf("Warning: close fuse conn: %v", err)
    }

    // Unmount filesystem
    if err := syscall.Unmount(mountCtx.MountPoint, 0); err != nil {
        return fmt.Errorf("unmount: %w", err)
    }

    // Close gRPC connection
    if closer, ok := mountCtx.GrpcClient.(interface{ Close() error }); ok {
        closer.Close()
    }

    delete(vm.mounts, volumeID)

    log.Printf("Unmounted volume %s", volumeID)

    return nil
}
```

---

## 六、HTTP API

### 6.1 Mount Volume

```http
POST /api/v1/volumes/mount
Content-Type: application/json

{
    "volume_id": "vol-abc123",
    "sandbox_id": "sb-def456",
    "mount_point": "/workspace",
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}

Response: 200 OK
{
    "volume_id": "vol-abc123",
    "mount_point": "/workspace",
    "mounted_at": "2024-01-01T00:00:00Z"
}

Error Response: 409 Conflict
{
    "error": "volume_vol-abc123_already_mounted"
}
```

### 6.2 Unmount Volume

```http
POST /api/v1/volumes/unmount
Content-Type: application/json

{
    "volume_id": "vol-abc123"
}

Response: 200 OK
{}

Error Response: 404 Not Found
{
    "error": "volume_not_mounted"
}
```

### 6.3 Get Mount Status

```http
GET /api/v1/volumes/status

Response: 200 OK
{
    "mounts": [
        {
            "volume_id": "vol-abc123",
            "mount_point": "/workspace",
            "mounted_at": "2024-01-01T00:00:00Z",
            "mounted_duration_sec": 3600
        }
    ]
}
```

---

## 七、与 Manager 的交互

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Manager → Procd Mount Flow                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Manager claims idle Pod                                                  │
│     PATCH /api/v1/pods/{pod_id}                                             │
│     {                                                                       │
│       "labels": {"state": "active", "sandbox_id": "sb-123"}                  │
│     }                                                                       │
│                                                                              │
│  2. Manager generates storage token                                          │
│     token = GenerateStorageToken(volumeID, sandboxID)                        │
│     → JWT signed with JWT_SECRET                                            │
│                                                                              │
│  3. Manager calls Procd API to mount volume                                  │
│     POST http://procd-{pod-id}:8080/api/v1/volumes/mount                     │
│     {                                                                       │
│       "volume_id": "vol-456",                                               │
│       "sandbox_id": "sb-123",                                               │
│       "mount_point": "/workspace",                                          │
│       "token": "eyJhbGc..."                                                 │
│     }                                                                       │
│                                                                              │
│  4. Procd VolumeManager mounts RemoteFS                                      │
│     ├─ Create gRPC connection (with SO_MARK=0x2)                            │
│     ├─ Mount FUSE at /workspace                                             │
│     ├─ Start FUSE server (forwards to gRPC)                                 │
│     └─ Return success                                                       │
│                                                                              │
│  5. User can now access files in /workspace                                 │
│     All file operations: User → FUSE → gRPC → Storage Proxy → S3/PG          │
│                                                                              │
│  6. On sandbox deletion                                                      │
│     Manager calls Procd API to unmount                                       │
│     POST http://procd-{pod-id}:8080/api/v1/volumes/unmount                   │
│     {"volume_id": "vol-456"}                                                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 八、网络配置

### 8.1 nftables Configuration

```bash
# nftables rules in Procd (applied on startup)
table inet sb0-firewall {
    chain SANDBOX0_OUTPUT {
        # Proxy bypass (highest priority)
        meta mark & 0x2 == 0x2 accept

        # Private IP blacklist (for user traffic)
        ip daddr @predef_deny drop

        # User deny list
        ip daddr @user_deny drop

        # Whitelist mode: redirect TCP to proxy
        meta l4proto tcp tcp dport != 8080 redirect to 127.0.0.1:1080
    }
}
```

### 8.2 Environment Variables

```yaml
# Procd container environment
env:
  - name: STORAGE_PROXY_URL
    value: "storage-proxy.sandbox0-system.svc.cluster.local:8080"
```

---

## 九、错误处理

```go
var (
    ErrVolumeAlreadyMounted = errors.New("volume_already_mounted")
    ErrVolumeNotMounted    = errors.New("volume_not_mounted")
    ErrInvalidMountPoint   = errors.New("invalid_mount_point")
    ErrMountTimeout        = errors.New("mount_timeout")
    ErrUnmountFailed       = errors.New("unmount_failed")
    ErrConnectionFailed    = errors.New("grpc_connection_failed")
)
```

---

## 十、性能特性

| Operation | Latency | Notes |
|-----------|---------|-------|
| Mount | ~30-50ms | gRPC connect + FUSE mount |
| Read (cached) | ~2-3ms | gRPC roundtrip |
| Write | ~5-10ms | gRPC + async write |
| Create | ~3-5ms | gRPC roundtrip |
| Lookup | ~1-2ms | gRPC roundtrip |

---

## 十一、优势总结

| 特性 | 说明 |
|------|------|
| **零凭证** | Procd 不持有任何 S3/PG 凭证 |
| **轻量级** | 只负责 FUSE + gRPC 客户端 |
| **网络隔离** | Packet marking 绕过用户规则 |
| **快速挂载** | <50ms 延迟 |
| **简化架构** | 无需 JuiceFS 嵌入 |
| **集中式存储** | 所有存储逻辑在 Proxy |
