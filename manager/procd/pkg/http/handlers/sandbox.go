// Package handlers provides HTTP handlers for Procd.
package handlers

import (
	"net/http"

	ctxpkg "github.com/sandbox0-ai/infra/manager/procd/pkg/context"
	"go.uber.org/zap"
)

// SandboxHandler handles sandbox-level HTTP requests.
type SandboxHandler struct {
	manager *ctxpkg.Manager
	logger  *zap.Logger
}

// NewSandboxHandler creates a new sandbox handler.
func NewSandboxHandler(manager *ctxpkg.Manager, logger *zap.Logger) *SandboxHandler {
	return &SandboxHandler{
		manager: manager,
		logger:  logger,
	}
}

// PauseAllResponse is the response body for pause all operation.
type PauseAllResponse struct {
	Paused bool   `json:"paused"`
	Error  string `json:"error,omitempty"`
}

// ResumeAllResponse is the response body for resume all operation.
type ResumeAllResponse struct {
	Resumed bool   `json:"resumed"`
	Error   string `json:"error,omitempty"`
}

// Pause pauses all running contexts and their child processes.
// This sends SIGSTOP to all process groups managed by procd.
func (h *SandboxHandler) Pause(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Pausing all contexts")

	err := h.manager.PauseAll()
	if err != nil {
		h.logger.Error("Failed to pause all contexts", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, PauseAllResponse{
			Paused: false,
			Error:  err.Error(),
		})
		return
	}

	h.logger.Info("All contexts paused successfully")
	writeJSON(w, http.StatusOK, PauseAllResponse{
		Paused: true,
	})
}

// Resume resumes all paused contexts and their child processes.
// This sends SIGCONT to all process groups managed by procd.
func (h *SandboxHandler) Resume(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Resuming all contexts")

	err := h.manager.ResumeAll()
	if err != nil {
		h.logger.Error("Failed to resume all contexts", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ResumeAllResponse{
			Resumed: false,
			Error:   err.Error(),
		})
		return
	}

	h.logger.Info("All contexts resumed successfully")
	writeJSON(w, http.StatusOK, ResumeAllResponse{
		Resumed: true,
	})
}
