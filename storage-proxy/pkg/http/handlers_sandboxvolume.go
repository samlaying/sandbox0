package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
)

type createSandboxVolumeRequest struct {
	CacheSize  string `json:"cache_size"`
	Prefetch   int    `json:"prefetch"`
	BufferSize string `json:"buffer_size"`
	Writeback  bool   `json:"writeback"`
	ReadOnly   bool   `json:"read_only"`
}

func (s *Server) createSandboxVolume(w http.ResponseWriter, r *http.Request) {
	// Get claims from context (populated by middleware)
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createSandboxVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	teamId, userId := claims.TeamID, claims.UserID
	if teamId == "" || userId == "" {
		http.Error(w, "team_id and user_id are required", http.StatusBadRequest)
		return
	}

	// Set defaults if not provided
	if req.CacheSize == "" {
		req.CacheSize = "1G"
	}
	if req.BufferSize == "" {
		req.BufferSize = "32M"
	}

	vol := &db.SandboxVolume{
		ID:         uuid.New().String(),
		TeamID:     teamId,
		UserID:     userId,
		CacheSize:  req.CacheSize,
		Prefetch:   req.Prefetch,
		BufferSize: req.BufferSize,
		Writeback:  req.Writeback,
		ReadOnly:   req.ReadOnly,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := s.repo.CreateSandboxVolume(r.Context(), vol); err != nil {
		s.logger.WithError(err).Error("Failed to create sandbox volume")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(vol)
}

func (s *Server) listSandboxVolumes(w http.ResponseWriter, r *http.Request) {
	// Get claims from context
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Use team_id from trusted token
	teamID := claims.TeamID
	if teamID == "" {
		http.Error(w, "team_id is required in token", http.StatusBadRequest)
		return
	}

	volumes, err := s.repo.ListSandboxVolumesByTeam(r.Context(), teamID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to list sandbox volumes")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(volumes)
}

func (s *Server) getSandboxVolume(w http.ResponseWriter, r *http.Request) {
	// Get claims from context
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	vol, err := s.repo.GetSandboxVolume(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		s.logger.WithError(err).Error("Failed to get sandbox volume")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Check if the volume belongs to the requesting team
	if vol.TeamID != claims.TeamID {
		s.logger.WithField("vol_team", vol.TeamID).WithField("req_team", claims.TeamID).Warn("Unauthorized access attempt to sandbox volume")
		http.Error(w, "not found", http.StatusNotFound) // Don't reveal existence
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vol)
}

func (s *Server) deleteSandboxVolume(w http.ResponseWriter, r *http.Request) {
	// Get claims from context
	claims := internalauth.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	// Get the volume to verify ownership
	vol, err := s.repo.GetSandboxVolume(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		s.logger.WithError(err).Error("Failed to get sandbox volume")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Check if the volume belongs to the requesting team
	if vol.TeamID != claims.TeamID {
		s.logger.WithField("vol_team", vol.TeamID).WithField("req_team", claims.TeamID).Warn("Unauthorized delete attempt to sandbox volume")
		http.Error(w, "not found", http.StatusNotFound) // Don't reveal existence
		return
	}

	// Check for active mounts via repository
	// Using 15 seconds as heartbeat timeout (same as coordinator.HeartbeatTimeout)
	const heartbeatTimeout = 15
	mounts, err := s.repo.GetActiveMounts(r.Context(), id, heartbeatTimeout)
	if err != nil {
		s.logger.WithError(err).Error("Failed to check active mounts")
		http.Error(w, "failed to check active mounts", http.StatusInternalServerError)
		return
	}

	if len(mounts) > 0 {
		// Volume has active mounts, cannot delete
		s.logger.WithField("volume_id", id).WithField("active_mounts", len(mounts)).Warn("Attempted to delete volume with active mounts")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":         "volume has active mounts",
			"active_mounts": len(mounts),
			"mounts":        mounts,
		})
		return
	}

	// No active mounts, proceed with deletion
	if err := s.repo.DeleteSandboxVolume(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		s.logger.WithError(err).Error("Failed to delete sandbox volume")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	s.logger.WithField("volume_id", id).WithField("team_id", vol.TeamID).Info("Sandbox volume deleted")
	w.WriteHeader(http.StatusNoContent)
}
