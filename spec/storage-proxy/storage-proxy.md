# Storage Proxy - Design Specification

## 一、设计目标

Storage Proxy 是一个独立的服务，负责管理所有持久化存储访问，将 JuiceFS 完全从 Procd 中移除。

### 核心原则

1. **凭证隔离**：所有 S3、PostgreSQL 凭证仅在 Proxy 中存储
2. **零 JuiceFS 修改**：使用 JuiceFS 官方 Go SDK，无需修改源码
3. **网络隔离兼容**：通过 packet marking 绕过 Procd 网络规则
4. **高性能**：gRPC over HTTP/2，支持流式传输

---

## 二、架构设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Storage Proxy Architecture                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Procd (Pod)                     Storage Proxy (Independent Service)         │
│  ┌─────────────────────┐       ┌─────────────────────────────────────────┐ │
│  │ /workspace (FUSE)   │       │ gRPC Server                            │ │
│  │   └─ test.txt       │◄──────├─► FileSystemService                    │ │
│  │                     │ gRPC  │  ├─ Read/Write/Create/Mkdir           │ │
│  │ RemoteFS (FUSE)     │       │  ├─ GetAttr/Lookup/ReadDir            │ │
│  │   └─ gRPC Client    │       │  └─ Rename/Flush/Fsync               │ │
│  └─────────────────────┘       └─────────────────────────────────────────┘ │
│           ▲                                   │                             │
│           │                                   ▼                             │
│      nftables                    ┌─────────────────────────────────────────┐│
│      mark==0x2 → ACCEPT          │ JuiceFS Embedded Library (SDK Mode)    ││
│                                   │  ┌───────────────────────────────────┐││
│                                   │  │ vfs.VFS (In-memory, no FUSE)      │││
│                                   │  ├─► meta.Client → PostgreSQL        │││
│                                   │  └─► chunk.CachedStore → S3          │││
│                                   │  └───────────────────────────────────┘││
│                                   └─────────────────────────────────────────┘│
│                                                              │               │
│                                                              ▼               │
│                                   ┌─────────────────────────────────────────┐│
│                                   │ Storage Backend (Real Credentials)      │││
│                                   │  ├── PostgreSQL (juicefs metadata)      │││
│                                   │  └── S3 (chunk data)                   │││
│                                   └─────────────────────────────────────────┘│
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、组件设计

### 3.1 服务组成

```
storage-proxy/
├── gRPC Server (port 8080)
│   └── FileSystemService
│
├── JuiceFS SDK Layer
│   ├── vfs.VFS (in-memory filesystem)
│   ├── meta.Client (PostgreSQL)
│   └── chunk.CachedStore (S3 + local cache)
│
├── Volume Management
│   ├── Mount/Unmount volumes
│   ├── Volume lifecycle
│   └─→ Cache management
│
└── Security Layer
    ├── JWT token validation
    ├── Volume access control
    ├── Audit logging
    └─→ Rate limiting
```

---

## 四、gRPC Protocol Definition

### 4.1 File System Service

```protobuf
// pkg/proto/filesystem.proto

syntax = "proto3";

package sandbox0.fs;

option go_package = "sandbox0/proto/fs";

// FileSystem service for remote file operations
service FileSystem {
  // File operations
  rpc Read(ReadRequest) returns (ReadResponse);
  rpc Write(WriteRequest) returns (WriteResponse);
  rpc Create(CreateRequest) returns (NodeResponse);
  rpc Mkdir(MkdirRequest) returns (NodeResponse);
  rpc Unlink(UnlinkRequest) returns (Empty);

  // Metadata operations
  rpc GetAttr(GetAttrRequest) returns (GetAttrResponse);
  rpc SetAttr(SetAttrRequest) returns (SetAttrResponse);
  rpc Lookup(LookupRequest) returns (NodeResponse);
  rpc ReadDir(ReadDirRequest) returns (ReadDirResponse);

  // Extended operations
  rpc Rename(RenameRequest) returns (Empty);
  rpc Flush(FlushRequest) returns (Empty);
  rpc Fsync(FsyncRequest) returns (Empty);
}

// Read request
message ReadRequest {
  string inode = 1;      // Inode identifier
  int64 offset = 2;      // Read offset
  int64 size = 3;        // Number of bytes to read
  uint64 handle_id = 4;  // File handle from Open
}

message ReadResponse {
  bytes data = 1;        // File data
  bool eof = 2;          // End of file marker
}

// Write request
message WriteRequest {
  string inode = 1;      // Inode identifier
  int64 offset = 2;      // Write offset
  bytes data = 3;        // Data to write
  uint64 handle_id = 4;  // File handle from Open
}

message WriteResponse {
  int64 bytes_written = 1;  // Number of bytes written
}

// Create file
message CreateRequest {
  string parent = 1;     // Parent directory inode
  string name = 2;       // File name
  uint32 mode = 3;       // File mode (permissions)
  uint32 flags = 4;      // Open flags
}

// Create directory
message MkdirRequest {
  string parent = 1;     // Parent directory inode
  string name = 2;       // Directory name
  uint32 mode = 3;       // Directory mode
}

// Node response (for Create, Mkdir, Lookup)
message NodeResponse {
  string inode = 1;          // Created/existing inode
  uint64 generation = 2;     // Generation number
  GetAttrResponse attr = 3;  // Node attributes
  uint64 handle_id = 4;      // File handle for operations
}

// Get file/directory attributes
message GetAttrRequest {
  string inode = 1;      // Inode to query
}

message GetAttrResponse {
  uint64 ino = 1;        // Inode number
  uint32 mode = 2;       // File type and mode
  uint32 nlink = 3;      // Number of hard links
  uint32 uid = 4;        // Owner user ID
  uint32 gid = 5;        // Owner group ID
  uint64 rdev = 6;       // Device ID (if special file)
  uint64 size = 7;       // Size in bytes
  uint64 blocks = 8;     // Number of 512-byte blocks
  int64 atime_sec = 9;   // Last access time (seconds)
  int64 atime_nsec = 10; // Last access time (nanoseconds)
  int64 mtime_sec = 11;  // Last modification time (seconds)
  int64 mtime_nsec = 12; // Last modification time (nanoseconds)
  int64 ctime_sec = 13;  // Last status change time (seconds)
  int64 ctime_nsec = 14; // Last status change time (nanoseconds)
}

// Set attributes
message SetAttrRequest {
  string inode = 1;         // Inode to modify
  uint32 valid = 2;         // Mask of fields to set
  GetAttrResponse attr = 3;  // Attributes to set
}

message SetAttrResponse {
  GetAttrResponse attr = 1;  // Updated attributes
}

// Lookup entry in directory
message LookupRequest {
  string parent = 1;     // Parent directory inode
  string name = 2;       // Entry name to lookup
}

// Read directory
message ReadDirRequest {
  string inode = 1;      // Directory inode
  uint64 handle_id = 2;  // Directory handle
  int64 offset = 3;      // Read offset
}

message ReadDirResponse {
  repeated DirEntry entries = 1;  // Directory entries
  bool eof = 2;                   // End of directory marker
}

message DirEntry {
  string inode = 1;            // Entry inode
  uint64 offset = 2;           // Entry offset
  string name = 3;             // Entry name
  GetAttrResponse attr = 4;    // Entry attributes
}

// Unlink (delete) file
message UnlinkRequest {
  string parent = 1;     // Parent directory inode
  string name = 2;       // File name to delete
}

// Rename entry
message RenameRequest {
  string old_parent = 1;  // Old parent directory inode
  string old_name = 2;    // Old name
  string new_parent = 3;  // New parent directory inode
  string new_name = 4;    // New name
}

// Flush file
message FlushRequest {
  uint64 handle_id = 1;  // File handle to flush
}

// Synchronize file
message FsyncRequest {
  uint64 handle_id = 1;  // File handle to sync
  bool datasync = 2;     // Sync only data
}

message Empty {}
```

---

## 五、FileSystemServer 实现

### 5.1 核心结构

```go
// pkg/storageproxy/server.go

package storageproxy

import (
    "context"
    "io"
    "sync"

    "github.com/juicefs/juicefs/v2/vfs"
    "github.com/juicefs/juicefs/v2/meta"
    "github.com/juicefs/juicefs/v2/chunk"
    "sandbox0/proto/fs"
)

// FileSystemServer implements the gRPC FileSystem service
type FileSystemServer struct {
    fs.UnimplementedFileSystemServer

    // JuiceFS instances per volume
    mu      sync.RWMutex
    volumes map[string]*VolumeContext  // volumeID -> VolumeContext

    // Configuration
    config *ServerConfig
}

// VolumeContext holds JuiceFS VFS instance for a volume
type VolumeContext struct {
    VolumeID   string
    VFS        *vfs.VFS
    MetaClient meta.Meta
    ChunkStore chunk.ChunkStore
    Config     *VolumeConfig
    MountedAt  time.Time
}

// ServerConfig server configuration
type ServerConfig struct {
    DefaultMetaURL     string
    DefaultS3Region    string
    DefaultS3Endpoint  string
    DefaultCacheSize   string
    DefaultCacheDir    string
    JWTSecret          string
}

// NewFileSystemServer creates a new file system server
func NewFileSystemServer(config *ServerConfig) *FileSystemServer {
    return &FileSystemServer{
        volumes: make(map[string]*VolumeContext),
        config:  config,
    }
}
```

### 5.2 Volume Mounting (SDK Mode, No FUSE)

```go
// MountVolume mounts a JuiceFS volume using SDK mode (in-memory, no FUSE)
func (s *FileSystemServer) MountVolume(ctx context.Context, volumeID string, config *VolumeConfig) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Check if already mounted
    if _, exists := s.volumes[volumeID]; exists {
        return fmt.Errorf("volume %s already mounted", volumeID)
    }

    // 1. Initialize JuiceFS metadata client
    metaConf := meta.DefaultConf()
    metaConf.MetaURL = config.MetaURL
    metaConf.ReadOnly = config.ReadOnly

    metaClient := meta.NewClient(config.MetaURL, metaConf)

    // Load or create format
    format, err := metaClient.Load(true)
    if err != nil {
        return fmt.Errorf("load juicefs format: %w", err)
    }

    // 2. Initialize S3 object storage
    blob := s.createS3Storage(config, format)

    // 3. Initialize chunk store with local cache
    chunkConf := &chunk.Config{
        CacheDir:   config.CacheDir,
        CacheSize:  s.parseCacheSize(config.CacheSize),
        BlockSize:  format.BlockSize * 1024,
        Compress:   format.Compression,
        Prefetch:   config.Prefetch,
        BufferSize: s.parseBufferSize(config.BufferSize),
        Writeback:  config.Writeback,
    }

    chunkStore := chunk.NewCachedStore(blob, *chunkConf, prometheus.DefaultRegisterer, prometheus.DefaultRegistry)

    // 4. Create JuiceFS VFS (in-memory, NO FUSE)
    vfsConf := &vfs.Config{
        Meta:   metaConf,
        Format: *format,
        Chunk:  chunkConf,
    }

    vfsInstance := vfs.NewVFS(vfsConf, metaClient, chunkStore, prometheus.DefaultRegisterer, prometheus.DefaultRegistry)

    // 5. Store volume context
    s.volumes[volumeID] = &VolumeContext{
        VolumeID:   volumeID,
        VFS:        vfsInstance,
        MetaClient: metaClient,
        ChunkStore: chunkStore,
        Config:     config,
        MountedAt:  time.Now(),
    }

    log.Printf("Mounted volume %s (SDK mode, no FUSE)", volumeID)

    return nil
}

// UnmountVolume unmounts a volume
func (s *FileSystemServer) UnmountVolume(ctx context.Context, volumeID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    ctx, ok := s.volumes[volumeID]
    if !ok {
        return fmt.Errorf("volume %s not mounted", volumeID)
    }

    // Flush all pending writes
    if err := ctx.VFS.FlushAll(""); err != nil {
        log.Printf("Flush warning: %v", err)
    }

    // Close metadata session
    if err := ctx.MetaClient.CloseSession(); err != nil {
        log.Printf("Close session warning: %v", err)
    }

    // Shutdown object storage client
    object.Shutdown(ctx.ChunkStore)

    delete(s.volumes, volumeID)

    log.Printf("Unmounted volume %s", volumeID)

    return nil
}

// getVolume retrieves volume context
func (s *FileSystemServer) getVolume(volumeID string) (*VolumeContext, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    ctx, ok := s.volumes[volumeID]
    if !ok {
        return nil, fmt.Errorf("volume %s not mounted", volumeID)
    }

    return ctx, nil
}
```

### 5.3 FUSE Operations Implementation

```go
// GetAttr implements FUSE getattr
func (s *FileSystemServer) GetAttr(ctx context.Context, req *fs.GetAttrRequest) (*fs.GetAttrResponse, error) {
    // Extract and validate token
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    // Get volume context
    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Get attributes from JuiceFS
    attr, err := volCtx.VFS.GetAttr(req.Inode, false)
    if err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    // Convert to protobuf response
    return &fs.GetAttrResponse{
        Ino:       attr.Inode,
        Mode:      uint32(attr.Mode),
        Nlink:     uint32(attr.Nlink),
        Uid:       attr.Uid,
        Gid:       attr.Gid,
        Rdev:      attr.Rdev,
        Size:      attr.Size,
        Blocks:    attr.Blocks,
        AtimeSec:  attr.Atime.Unix(),
        AtimeNsec: int64(attr.Atime.Nanosecond()),
        MtimeSec:  attr.Mtime.Unix(),
        MtimeNsec: int64(attr.Mtime.Nanosecond()),
        CtimeSec:  attr.Ctime.Unix(),
        CtimeNsec: int64(attr.Ctime.Nanosecond()),
    }, nil
}

// Read implements FUSE read
func (s *FileSystemServer) Read(ctx context.Context, req *fs.ReadRequest) (*fs.ReadResponse, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Allocate buffer
    buf := make([]byte, req.Size)

    // Read from JuiceFS
    n, err := volCtx.VFS.Read(req.Inode, buf, req.Offset)
    if err != nil && err != io.EOF {
        return nil, status.Error(codes.Internal, err.Error())
    }

    return &fs.ReadResponse{
        Data: buf[:n],
        Eof:  err == io.EOF,
    }, nil
}

// Write implements FUSE write
func (s *FileSystemServer) Write(ctx context.Context, req *fs.WriteRequest) (*fs.WriteResponse, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Write to JuiceFS
    n, err := volCtx.VFS.Write(req.Inode, req.Data, req.Offset, nil)
    if err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    return &fs.WriteResponse{
        BytesWritten: int64(n),
    }, nil
}

// Create implements FUSE create
func (s *FileSystemServer) Create(ctx context.Context, req *fs.CreateRequest) (*fs.NodeResponse, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Create file in JuiceFS
    inode, attr, err := volCtx.VFS.Create(req.Parent, req.Name, req.Flags, req.Mode, 0, 0)
    if err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    return &fs.NodeResponse{
        Inode:      inode,
        Generation: 0,
        Attr:       s.convertAttr(attr),
    }, nil
}

// Mkdir implements FUSE mkdir
func (s *FileSystemServer) Mkdir(ctx context.Context, req *fs.MkdirRequest) (*fs.NodeResponse, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Create directory in JuiceFS
    inode, attr, err := volCtx.VFS.Mkdir(req.Parent, req.Name, req.Mode, 0)
    if err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    return &fs.NodeResponse{
        Inode:      inode,
        Generation: 0,
        Attr:       s.convertAttr(attr),
    }, nil
}

// Unlink implements FUSE unlink
func (s *FileSystemServer) Unlink(ctx context.Context, req *fs.UnlinkRequest) (*fs.Empty, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Unlink file in JuiceFS
    if err := volCtx.VFS.Unlink(req.Parent, req.Name, false); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    return &fs.Empty{}, nil
}

// Lookup implements FUSE lookup
func (s *FileSystemServer) Lookup(ctx context.Context, req *fs.LookupRequest) (*fs.NodeResponse, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Lookup entry in JuiceFS
    inode, attr, err := volCtx.VFS.Lookup(req.Parent, req.Name)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    return &fs.NodeResponse{
        Inode:      inode,
        Generation: 0,
        Attr:       s.convertAttr(attr),
    }, nil
}

// ReadDir implements FUSE readdir
func (s *FileSystemServer) ReadDir(ctx context.Context, req *fs.ReadDirRequest) (*fs.ReadDirResponse, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Read directory from JuiceFS
    entries, err := volCtx.VFS.Readdir(req.Inode, req.Offset)
    if err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    // Convert entries
    var result []*fs.DirEntry
    for _, e := range entries {
        result = append(result, &fs.DirEntry{
            Inode:  e.Inode,
            Offset: e.Offset,
            Name:   e.Name,
            Attr:   s.convertAttr(e.Attr),
        })
    }

    return &fs.ReadDirResponse{
        Entries: result,
        Eof:     false,
    }, nil
}

// Rename implements FUSE rename
func (s *FileSystemServer) Rename(ctx context.Context, req *fs.RenameRequest) (*fs.Empty, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Rename in JuiceFS
    if err := volCtx.VFS.Rename(req.OldParent, req.OldName, req.NewParent, req.NewName, false); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    return &fs.Empty{}, nil
}

// Flush implements FUSE flush
func (s *FileSystemServer) Flush(ctx context.Context, req *fs.FlushRequest) (*fs.Empty, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Flush is mostly a no-op in JuiceFS (writes are buffered)
    // But we can sync metadata here
    return &fs.Empty{}, nil
}

// Fsync implements FUSE fsync
func (s *FileSystemServer) Fsync(ctx context.Context, req *fs.FsyncRequest) (*fs.Empty, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Sync file to S3
    // This is handled by JuiceFS chunk store's writeback cache
    return &fs.Empty{}, nil
}

// SetAttr implements FUSE setattr
func (s *FileSystemServer) SetAttr(ctx context.Context, req *fs.SetAttrRequest) (*fs.SetAttrResponse, error) {
    claims, err := s.authenticate(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, err.Error())
    }

    volCtx, err := s.getVolume(claims.VolumeID)
    if err != nil {
        return nil, status.Error(codes.NotFound, err.Error())
    }

    // Convert protobuf attr to JuiceFS attr
    attr := &vfs.Attr{
        Inode: req.Attr.Ino,
        Mode:  req.Attr.Mode,
        Uid:   req.Attr.Uid,
        Gid:   req.Attr.Gid,
        Size:  req.Attr.Size,
        Atime: time.Unix(req.Attr.AtimeSec, req.Attr.AtimeNsec),
        Mtime: time.Unix(req.Attr.MtimeSec, req.Attr.MtimeNsec),
        Ctime: time.Unix(req.Attr.CtimeSec, req.Attr.CtimeNsec),
    }

    // Set attributes in JuiceFS
    if err := volCtx.VFS.SetAttr(req.Inode, attr, req.Valid, false); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }

    return &fs.SetAttrResponse{
        Attr: s.convertAttr(attr),
    }, nil
}

// Helper: convert vfs.Attr to fs.GetAttrResponse
func (s *FileSystemServer) convertAttr(attr *vfs.Attr) *fs.GetAttrResponse {
    return &fs.GetAttrResponse{
        Ino:       attr.Inode,
        Mode:      uint32(attr.Mode),
        Nlink:     uint32(attr.Nlink),
        Uid:       attr.Uid,
        Gid:       attr.Gid,
        Rdev:      attr.Rdev,
        Size:      attr.Size,
        Blocks:    attr.Blocks,
        AtimeSec:  attr.Atime.Unix(),
        AtimeNsec: int64(attr.Atime.Nanosecond()),
        MtimeSec:  attr.Mtime.Unix(),
        MtimeNsec: int64(attr.Mtime.Nanosecond()),
        CtimeSec:  attr.Ctime.Unix(),
        CtimeNsec: int64(attr.Ctime.Nanosecond()),
    }
}
```

---

## 六、安全认证

### 6.1 JWT Token Authentication

```go
// pkg/storageproxy/auth.go

package storageproxy

import (
    "context"
    "strings"

    "github.com/golang-jwt/jwt/v5"
)

// TokenClaims represents JWT token claims
type TokenClaims struct {
    VolumeID  string `json:"volume_id"`
    SandboxID string `json:"sandbox_id"`
    TeamID    string `json:"team_id"`
    jwt.RegisteredClaims
}

// authenticate validates JWT token and returns claims
func (s *FileSystemServer) authenticate(ctx context.Context) (*TokenClaims, error) {
    // Extract token from metadata
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return nil, fmt.Errorf("missing metadata")
    }

    authHeaders := md["authorization"]
    if len(authHeaders) == 0 {
        return nil, fmt.Errorf("missing authorization header")
    }

    auth := authHeaders[0]
    if !strings.HasPrefix(auth, "Bearer ") {
        return nil, fmt.Errorf("invalid authorization format")
    }

    tokenString := strings.TrimPrefix(auth, "Bearer ")

    // Parse and validate token
    token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return []byte(s.config.JWTSecret), nil
    })

    if err != nil {
        return nil, err
    }

    claims, ok := token.Claims.(*TokenClaims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token")
    }

    // Validate volume access
    if !s.checkVolumeAccess(claims.VolumeID, claims.SandboxID) {
        return nil, fmt.Errorf("access denied to volume %s", claims.VolumeID)
    }

    return claims, nil
}

// checkVolumeAccess validates sandbox has access to volume
func (s *FileSystemServer) checkVolumeAccess(volumeID, sandboxID string) bool {
    // Query PostgreSQL or cache to check access
    // For now, simple check
    return true
}
```

### 6.2 Token Generation (Manager Side)

```go
// pkg/manager/storage_token.go

package manager

import (
    "time"

    "github.com/golang-jwt/jwt/v5"
)

// GenerateStorageToken generates a JWT token for storage access
func (s *ManagerService) GenerateStorageToken(volumeID, sandboxID string) (string, error) {
    // Get volume info
    volume, err := s.store.GetVolume(context.Background(), volumeID)
    if err != nil {
        return "", err
    }

    // Create claims
    claims := &storageproxy.TokenClaims{
        VolumeID:  volumeID,
        SandboxID: sandboxID,
        TeamID:    volume.TeamID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            Issuer:    "sandbox0-manager",
        },
    }

    // Sign token
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(s.config.JWTSecret))
}
```

---

## 七、S3 Storage Creation

```go
// pkg/storageproxy/s3.go

package storageproxy

import (
    "context"

    "github.com/juicefs/juicefs/v2/object"
)

// createS3Storage creates S3 object storage for JuiceFS
func (s *FileSystemServer) createS3Storage(config *VolumeConfig, format *meta.Format) object.ObjectStorage {
    // Create S3 client configuration
    endpoint := config.S3Endpoint
    if endpoint == "" {
        endpoint = fmt.Sprintf("s3.%s.amazonaws.com", config.S3Region)
    }

    // Build S3 URL
    s3URL := fmt.Sprintf("s3://%s:%s@%s/%s",
        config.S3AccessKey,
        config.S3SecretKey,
        endpoint,
        config.S3Bucket,
    )

    // Add prefix if specified
    if config.S3Prefix != "" {
        s3URL += "/" + strings.TrimPrefix(config.S3Prefix, "/")
    }

    // Create object storage using JuiceFS object package
    obj, err := object.NewObjectStorage(s3URL, &object.Quark{
        Endpoint:  endpoint,
        AccessKey: config.S3AccessKey,
        SecretKey: config.S3SecretKey,
        Region:    config.S3Region,
        SessionToken: config.S3SessionToken, // For IRSA
    })

    if err != nil {
        log.Fatalf("Failed to create S3 storage: %v", err)
    }

    return obj
}
```

---

## 八、HTTP Management API

### 8.1 Volume Management

```http
# Mount volume (called by Manager when claiming sandbox)
POST /api/v1/volumes/{volume_id}/mount
Authorization: Bearer {manager_token}

{
    "sandbox_id": "sb-abc123",
    "config": {
        "meta_url": "postgres://postgres:5432/sandbox0",
        "s3_bucket": "sandbox0-volumes",
        "s3_prefix": "teams/team-456/volumes/vol-123",
        "s3_region": "us-east-1",
        "cache_size": "10Gi",
        "cache_dir": "/var/lib/storage-proxy/cache/vol-123"
    }
}

Response: 200 OK
{
    "volume_id": "vol-123",
    "mounted_at": "2024-01-01T00:00:00Z"
}

# Unmount volume
POST /api/v1/volumes/{volume_id}/unmount
Authorization: Bearer {manager_token}

Response: 200 OK

# Get volume status
GET /api/v1/volumes/{volume_id}
Authorization: Bearer {manager_token}

Response: 200 OK
{
    "volume_id": "vol-123",
    "is_mounted": true,
    "mounted_at": "2024-01-01T00:00:00Z",
    "cache_stats": {
        "used_bytes": 1234567890,
        "total_bytes": 10737418240
    }
}
```

### 8.2 Cache Management

```http
# Clear volume cache
POST /api/v1/volumes/{volume_id}/cache/clear
Authorization: Bearer {manager_token}

Response: 200 OK
{
    "freed_bytes": 1234567890
}

# Get cache statistics
GET /api/v1/volumes/{volume_id}/cache/stats
Authorization: Bearer {manager_token}

Response: 200 OK
{
    "used_bytes": 1234567890,
    "total_bytes": 10737418240,
    "hit_rate": 0.85,
    "entries": 12345
}
```

---

## 九、部署配置

### 9.1 Kubernetes Deployment

```yaml
apiVersion: v1
kind: Service
metadata:
  name: storage-proxy
  namespace: sandbox0-system
spec:
  ports:
  - port: 8080
    name: grpc
    targetPort: 8080
  - port: 8081
    name: http
    targetPort: 8081
  selector:
    app: storage-proxy
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: storage-proxy
  namespace: sandbox0-system
spec:
  serviceName: storage-proxy
  replicas: 3
  selector:
    matchLabels:
      app: storage-proxy
  template:
    metadata:
      labels:
        app: storage-proxy
    spec:
      containers:
      - name: storage-proxy
        image: sandbox0/storage-proxy:latest
        ports:
        - containerPort: 8080
          name: grpc
        - containerPort: 8081
          name: http
        env:
        - name: POSTGRES_URL
          valueFrom:
            secretKeyRef:
              name: postgres-credentials
              key: url
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: aws-credentials
              key: access_key_id
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: aws-credentials
              key: secret_access_key
        - name: AWS_REGION
          value: "us-east-1"
        - name: CACHE_ROOT
          value: "/var/lib/storage-proxy/cache"
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: jwt-secret
              key: secret
        volumeMounts:
        - name: cache
          mountPath: /var/lib/storage-proxy/cache
        resources:
          requests:
            cpu: "2"
            memory: "4Gi"
          limits:
            cpu: "4"
            memory: "8Gi"
      volumes:
      - name: cache
        emptyDir:
          sizeLimit: 100Gi
```

### 9.2 Procd Configuration

```yaml
# Procd container env
env:
  - name: STORAGE_PROXY_URL
    value: "storage-proxy.sandbox0-system.svc.cluster.local:8080"
```

---

## 十、监控与观测

### 10.1 Metrics

```go
// Exposed Prometheus metrics

// Volume metrics
storage_proxy_volumes_total
storage_proxy_volumes_mounted
storage_proxy_volumes_mount_errors_total

// Operation metrics
storage_proxy_operations_total{operation="read|write|create|mkdir|unlink|lookup|readdir"}
storage_proxy_operations_duration_seconds{operation="...", quantile="0.5|0.9|0.99"}
storage_proxy_operations_errors_total{operation="...", error_type="..."}

// Cache metrics
storage_proxy_cache_hit_rate
storage_proxy_cache_used_bytes
storage_proxy_cache_total_bytes

// S3 metrics
storage_proxy_s3_operations_total{operation="put|get|delete"}
storage_proxy_s3_bytes_total{operation="put|get"}
storage_proxy_s3_duration_seconds{operation="put|get"}
```

### 10.2 Audit Logging

```go
// All operations logged with:
// - Timestamp
// - VolumeID
// - SandboxID
// - Operation (read/write/create/...)
// - Path/Key
// - Size (for read/write)
// - Latency
// - Error (if any)

type AuditLog struct {
    Timestamp  time.Time `json:"timestamp"`
    VolumeID   string    `json:"volume_id"`
    SandboxID  string    `json:"sandbox_id"`
    Operation  string    `json:"operation"`
    Path       string    `json:"path"`
    Size       int64     `json:"size,omitempty"`
    LatencyMs  int64     `json:"latency_ms"`
    Error      string    `json:"error,omitempty"`
}
```

---

## 十一、故障处理

### 11.1 Volume Auto-Remount

```go
// If gRPC call fails with "volume not mounted", trigger remount
func (s *FileSystemServer) ensureVolumeMounted(volumeID string) error {
    s.mu.RLock()
    _, exists := s.volumes[volumeID]
    s.mu.RUnlock()

    if exists {
        return nil
    }

    // Load volume config from PostgreSQL
    config, err := s.loadVolumeConfig(volumeID)
    if err != nil {
        return err
    }

    // Mount volume
    return s.MountVolume(context.Background(), volumeID, config)
}
```

### 11.2 Connection Recovery

```go
// If PostgreSQL connection fails, reconnect
func (s *FileSystemServer) reconnectMetaClient(volumeID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    ctx, ok := s.volumes[volumeID]
    if !ok {
        return fmt.Errorf("volume not mounted")
    }

    // Reconnect
    if err := ctx.MetaClient.Reconnect(); err != nil {
        return err
    }

    log.Printf("Reconnected metadata client for volume %s", volumeID)
    return nil
}
```

---

## 十二、优势总结

| 特性 | 说明 |
|------|------|
| **凭证隔离** | 所有 S3/PG 凭证仅在 Proxy 中 |
| **零 JuiceFS 修改** | 使用官方 Go SDK |
| **完整 POSIX** | FUSE 提供完整文件系统语义 |
| **网络隔离兼容** | Packet marking 绕过 Procd 防火墙 |
| **高性能** | gRPC over HTTP/2，支持流式传输 |
| **集中式缓存** | Proxy 端缓存，多 Pod 共享 |
| **审计日志** | 所有操作集中记录 |
| **独立扩展** | Proxy 可独立于 Sandbox 扩展 |
