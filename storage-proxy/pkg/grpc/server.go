package grpc

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/sandbox0-ai/storage-proxy/pkg/audit"
	"github.com/sandbox0-ai/storage-proxy/pkg/auth"
	"github.com/sandbox0-ai/storage-proxy/pkg/volume"
	pb "github.com/sandbox0-ai/storage-proxy/proto/fs"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/juicedata/juicefs/pkg/vfs"
)

// FileSystemServer implements the gRPC FileSystem service
type FileSystemServer struct {
	pb.UnimplementedFileSystemServer

	volMgr  *volume.Manager
	auditor *audit.Logger
	logger  *logrus.Logger
}

// NewFileSystemServer creates a new file system server
func NewFileSystemServer(volMgr *volume.Manager, auditor *audit.Logger, logger *logrus.Logger) *FileSystemServer {
	return &FileSystemServer{
		volMgr:  volMgr,
		auditor: auditor,
		logger:  logger,
	}
}

// MountVolume mounts a volume
func (s *FileSystemServer) MountVolume(ctx context.Context, req *pb.MountVolumeRequest) (*pb.MountVolumeResponse, error) {
	config := &volume.VolumeConfig{
		MetaURL:        req.Config.MetaUrl,
		S3Bucket:       req.Config.S3Bucket,
		S3Prefix:       req.Config.S3Prefix,
		S3Region:       req.Config.S3Region,
		S3Endpoint:     req.Config.S3Endpoint,
		S3AccessKey:    req.Config.S3AccessKey,
		S3SecretKey:    req.Config.S3SecretKey,
		S3SessionToken: req.Config.S3SessionToken,
		CacheDir:       req.Config.CacheDir,
		CacheSize:      req.Config.CacheSize,
		Prefetch:       int(req.Config.Prefetch),
		BufferSize:     req.Config.BufferSize,
		Writeback:      req.Config.Writeback,
		ReadOnly:       req.Config.ReadOnly,
	}

	err := s.volMgr.MountVolume(ctx, req.VolumeId, config)
	if err != nil {
		s.logger.WithError(err).WithField("volume_id", req.VolumeId).Error("Failed to mount volume")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.MountVolumeResponse{
		VolumeId:  req.VolumeId,
		MountedAt: time.Now().Unix(),
	}, nil
}

// UnmountVolume unmounts a volume
func (s *FileSystemServer) UnmountVolume(ctx context.Context, req *pb.UnmountVolumeRequest) (*pb.Empty, error) {
	err := s.volMgr.UnmountVolume(ctx, req.VolumeId)
	if err != nil {
		s.logger.WithError(err).WithField("volume_id", req.VolumeId).Error("Failed to unmount volume")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.Empty{}, nil
}

// GetAttr implements FUSE getattr
func (s *FileSystemServer) GetAttr(ctx context.Context, req *pb.GetAttrRequest) (*pb.GetAttrResponse, error) {
	// Extract claims for audit logging
	claims, _ := auth.GetClaims(ctx)

	// Get volume context
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Get attributes from JuiceFS
	var attr meta.Attr
	inode := meta.Ino(req.Inode)
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.GetAttr(vfsCtx, inode, &attr)
	if st != 0 {
		s.logger.WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"inode":     req.Inode,
			"error":     st,
		}).Error("GetAttr failed")
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	// Audit log
	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "getattr",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return convertAttr(&attr), nil
}

// Lookup implements FUSE lookup
func (s *FileSystemServer) Lookup(ctx context.Context, req *pb.LookupRequest) (*pb.NodeResponse, error) {
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Lookup entry in JuiceFS
	var inode meta.Ino
	var attr meta.Attr
	parent := meta.Ino(req.Parent)
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Lookup(vfsCtx, parent, req.Name, &inode, &attr, true)
	if st != 0 {
		if st == syscall.ENOENT {
			return nil, status.Error(codes.NotFound, "entry not found")
		}
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "lookup",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.NodeResponse{
		Inode:      uint64(inode),
		Generation: 0,
		Attr:       convertAttr(&attr),
	}, nil
}

// Open implements FUSE open
func (s *FileSystemServer) Open(ctx context.Context, req *pb.OpenRequest) (*pb.OpenResponse, error) {
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	inode := meta.Ino(req.Inode)
	var attr meta.Attr

	// Open file in JuiceFS
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Open(vfsCtx, inode, req.Flags, &attr)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	// Generate a handle ID (simple sequential ID)
	handleID := uint64(time.Now().UnixNano())

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "open",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return &pb.OpenResponse{
		HandleId: handleID,
	}, nil
}

// Read implements FUSE read - Note: This is simplified, real implementation would use VFS layer
func (s *FileSystemServer) Read(ctx context.Context, req *pb.ReadRequest) (*pb.ReadResponse, error) {
	startTime := time.Now()
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// For now, return empty data - full implementation would use VFS.Read
	// which requires proper handle management and chunk reading
	s.logger.Warn("Read operation not fully implemented - returning empty data")
	_ = volCtx // suppress unused variable warning

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "read",
			Inode:     req.Inode,
			Size:      0,
			Latency:   time.Since(startTime),
			Status:    "success",
		})
	}

	return &pb.ReadResponse{
		Data: []byte{},
		Eof:  true,
	}, nil
}

// Write implements FUSE write - Note: This is simplified
func (s *FileSystemServer) Write(ctx context.Context, req *pb.WriteRequest) (*pb.WriteResponse, error) {
	startTime := time.Now()
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// For now, acknowledge write - full implementation would use VFS.Write
	s.logger.Warn("Write operation not fully implemented")
	_ = volCtx // suppress unused variable warning

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "write",
			Inode:     req.Inode,
			Size:      int64(len(req.Data)),
			Latency:   time.Since(startTime),
			Status:    "success",
		})
	}

	return &pb.WriteResponse{
		BytesWritten: int64(len(req.Data)),
	}, nil
}

// Create implements FUSE create
func (s *FileSystemServer) Create(ctx context.Context, req *pb.CreateRequest) (*pb.NodeResponse, error) {
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Create file in JuiceFS
	parent := meta.Ino(req.Parent)
	var inode meta.Ino
	var attr meta.Attr

	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Create(vfsCtx, parent, req.Name, uint16(req.Mode), 0, req.Flags, &inode, &attr)
	if st != 0 {
		s.logger.WithError(syscall.Errno(st)).WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"parent":    req.Parent,
			"name":      req.Name,
		}).Error("Create failed")
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "create",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.NodeResponse{
		Inode:      uint64(inode),
		Generation: 0,
		Attr:       convertAttr(&attr),
		HandleId:   uint64(time.Now().UnixNano()),
	}, nil
}

// Mkdir implements FUSE mkdir
func (s *FileSystemServer) Mkdir(ctx context.Context, req *pb.MkdirRequest) (*pb.NodeResponse, error) {
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Create directory in JuiceFS
	parent := meta.Ino(req.Parent)
	var inode meta.Ino
	var attr meta.Attr

	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Mkdir(vfsCtx, parent, req.Name, uint16(req.Mode), 0, 0, &inode, &attr)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "mkdir",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.NodeResponse{
		Inode:      uint64(inode),
		Generation: 0,
		Attr:       convertAttr(&attr),
	}, nil
}

// Unlink implements FUSE unlink
func (s *FileSystemServer) Unlink(ctx context.Context, req *pb.UnlinkRequest) (*pb.Empty, error) {
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Unlink file in JuiceFS
	parent := meta.Ino(req.Parent)
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Unlink(vfsCtx, parent, req.Name)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "unlink",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.Empty{}, nil
}

// ReadDir implements FUSE readdir
func (s *FileSystemServer) ReadDir(ctx context.Context, req *pb.ReadDirRequest) (*pb.ReadDirResponse, error) {
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Read directory from JuiceFS
	inode := meta.Ino(req.Inode)
	var entries []*meta.Entry
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Readdir(vfsCtx, inode, 1, &entries)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	// Convert entries
	var result []*pb.DirEntry
	for _, e := range entries {
		result = append(result, &pb.DirEntry{
			Inode:  uint64(e.Inode),
			Offset: 0,
			Name:   string(e.Name),
			Type:   uint32(e.Attr.Typ),
			Attr:   convertAttr(e.Attr),
		})
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "readdir",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return &pb.ReadDirResponse{
		Entries: result,
		Eof:     false,
	}, nil
}

// Rename implements FUSE rename
func (s *FileSystemServer) Rename(ctx context.Context, req *pb.RenameRequest) (*pb.Empty, error) {
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Rename in JuiceFS
	oldParent := meta.Ino(req.OldParent)
	newParent := meta.Ino(req.NewParent)
	var inode meta.Ino
	var attr meta.Attr

	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Rename(vfsCtx, oldParent, req.OldName, newParent, req.NewName, req.Flags, &inode, &attr)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "rename",
			Inode:     uint64(oldParent),
			Path:      fmt.Sprintf("%s -> %s", req.OldName, req.NewName),
			Status:    "success",
		})
	}

	return &pb.Empty{}, nil
}

// SetAttr implements FUSE setattr
func (s *FileSystemServer) SetAttr(ctx context.Context, req *pb.SetAttrRequest) (*pb.SetAttrResponse, error) {
	claims, _ := auth.GetClaims(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	inode := meta.Ino(req.Inode)
	var attr meta.Attr

	// Set attributes in JuiceFS
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.SetAttr(vfsCtx, inode, uint16(req.Valid), 0, &attr)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "setattr",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return &pb.SetAttrResponse{
		Attr: convertAttr(&attr),
	}, nil
}

// Flush implements FUSE flush
func (s *FileSystemServer) Flush(ctx context.Context, req *pb.FlushRequest) (*pb.Empty, error) {
	// Flush is mostly a no-op in JuiceFS (writes are buffered)
	return &pb.Empty{}, nil
}

// Fsync implements FUSE fsync
func (s *FileSystemServer) Fsync(ctx context.Context, req *pb.FsyncRequest) (*pb.Empty, error) {
	// Fsync - data is synced by chunk store's writeback cache
	return &pb.Empty{}, nil
}

// Release implements FUSE release (close)
func (s *FileSystemServer) Release(ctx context.Context, req *pb.ReleaseRequest) (*pb.Empty, error) {
	// Release handle (cleanup if needed)
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Close the inode (simplified - would need proper handle tracking)
	// For now, just return success
	_ = volCtx
	return &pb.Empty{}, nil
}

// Helper: convert meta.Attr to protobuf GetAttrResponse
func convertAttr(attr *meta.Attr) *pb.GetAttrResponse {
	return &pb.GetAttrResponse{
		Ino:       uint64(meta.RootInode), // Would need proper inode tracking
		Mode:      uint32(attr.Mode),
		Nlink:     attr.Nlink,
		Uid:       attr.Uid,
		Gid:       attr.Gid,
		Rdev:      uint64(attr.Rdev),
		Size:      attr.Length,
		Blocks:    0,
		AtimeSec:  attr.Atime,
		AtimeNsec: int64(attr.Atimensec),
		MtimeSec:  attr.Mtime,
		MtimeNsec: int64(attr.Mtimensec),
		CtimeSec:  attr.Ctime,
		CtimeNsec: int64(attr.Ctimensec),
	}
}
