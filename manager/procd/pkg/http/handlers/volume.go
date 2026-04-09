package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sandbox0-ai/sandbox0/manager/procd/pkg/volume"
	obsmetrics "github.com/sandbox0-ai/sandbox0/pkg/observability/metrics"
	"go.uber.org/zap"
)

// VolumeHandler handles SandboxVolume-related HTTP requests.
type VolumeHandler struct {
	manager *volume.Manager
	logger  *zap.Logger
	metrics *obsmetrics.ProcdMetrics
}

// NewVolumeHandler creates a new volume handler.
func NewVolumeHandler(manager *volume.Manager, logger *zap.Logger, metrics ...*obsmetrics.ProcdMetrics) *VolumeHandler {
	var procdMetrics *obsmetrics.ProcdMetrics
	if len(metrics) > 0 {
		procdMetrics = metrics[0]
	}
	return &VolumeHandler{
		manager: manager,
		logger:  logger,
		metrics: procdMetrics,
	}
}

func (h *VolumeHandler) observeVolumeOperation(operation, status string, started time.Time) {
	if h.metrics == nil {
		return
	}
	h.metrics.VolumeOperationsTotal.WithLabelValues(operation, status).Inc()
	h.metrics.VolumeOperationDuration.WithLabelValues(operation).Observe(time.Since(started).Seconds())
}

// Mount mounts a SandboxVolume.
func (h *VolumeHandler) Mount(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var req volume.MountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.observeVolumeOperation("mount", "invalid_request", started)
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.SandboxVolumeID == "" {
		h.observeVolumeOperation("mount", "invalid_request", started)
		writeError(w, http.StatusBadRequest, "invalid_sandboxvolume_id", "sandboxvolume_id is required")
		return
	}

	if req.MountPoint == "" {
		h.observeVolumeOperation("mount", "invalid_request", started)
		writeError(w, http.StatusBadRequest, "invalid_mount_point", "mount_point is required")
		return
	}

	resp, err := h.manager.Mount(r.Context(), &req)
	if err != nil {
		h.logger.Warn("Failed to mount volume",
			zap.String("sandboxvolume_id", req.SandboxVolumeID),
			zap.String("mount_point", req.MountPoint),
			zap.Error(err),
		)
		if err == volume.ErrVolumeAlreadyMounted {
			h.observeVolumeOperation("mount", "already_mounted", started)
			writeError(w, http.StatusConflict, "already_mounted", err.Error())
			return
		}
		if err == volume.ErrVolumeMountInProgress {
			h.observeVolumeOperation("mount", "mount_in_progress", started)
			writeError(w, http.StatusConflict, "mount_in_progress", err.Error())
			return
		}
		if err == volume.ErrMountPointInUse {
			h.observeVolumeOperation("mount", "mount_point_in_use", started)
			writeError(w, http.StatusConflict, "mount_point_in_use", err.Error())
			return
		}
		if err == volume.ErrInvalidMountPoint {
			h.observeVolumeOperation("mount", "invalid_mount_point", started)
			writeError(w, http.StatusBadRequest, "invalid_mount_point", err.Error())
			return
		}
		h.observeVolumeOperation("mount", "error", started)
		writeError(w, http.StatusInternalServerError, "mount_failed", err.Error())
		return
	}

	h.logger.Info("Mounted volume",
		zap.String("sandboxvolume_id", req.SandboxVolumeID),
		zap.String("mount_point", req.MountPoint),
		zap.String("mount_session_id", resp.MountSessionID),
	)
	h.observeVolumeOperation("mount", "success", started)
	writeJSON(w, http.StatusOK, resp)
}

// Unmount unmounts a SandboxVolume.
func (h *VolumeHandler) Unmount(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var req volume.UnmountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.observeVolumeOperation("unmount", "invalid_request", started)
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.SandboxVolumeID == "" {
		h.observeVolumeOperation("unmount", "invalid_request", started)
		writeError(w, http.StatusBadRequest, "invalid_sandboxvolume_id", "sandboxvolume_id is required")
		return
	}
	if req.MountSessionID == "" {
		h.observeVolumeOperation("unmount", "invalid_request", started)
		writeError(w, http.StatusBadRequest, "invalid_mount_session_id", "mount_session_id is required")
		return
	}

	err := h.manager.Unmount(r.Context(), req.SandboxVolumeID, req.MountSessionID)
	if err != nil {
		if err == volume.ErrVolumeNotMounted {
			h.observeVolumeOperation("unmount", "not_mounted", started)
			writeJSON(w, http.StatusOK, map[string]bool{"unmounted": true})
			return
		}
		if err == volume.ErrMountSessionNotFound {
			h.observeVolumeOperation("unmount", "session_not_found", started)
			writeJSON(w, http.StatusOK, map[string]bool{"unmounted": true})
			return
		}
		h.logger.Warn("Failed to unmount volume",
			zap.String("sandboxvolume_id", req.SandboxVolumeID),
			zap.String("mount_session_id", req.MountSessionID),
			zap.Error(err),
		)
		h.observeVolumeOperation("unmount", "error", started)
		writeError(w, http.StatusInternalServerError, "unmount_failed", err.Error())
		return
	}

	h.logger.Info("Unmounted volume",
		zap.String("sandboxvolume_id", req.SandboxVolumeID),
		zap.String("mount_session_id", req.MountSessionID),
	)
	h.observeVolumeOperation("unmount", "success", started)
	writeJSON(w, http.StatusOK, map[string]bool{"unmounted": true})
}

// Status returns the status of all mounts.
func (h *VolumeHandler) Status(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	status := h.manager.GetStatus()
	h.observeVolumeOperation("status", "success", started)
	writeJSON(w, http.StatusOK, map[string]any{
		"mounts": status,
	})
}
