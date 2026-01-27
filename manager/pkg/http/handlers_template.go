package http

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	"github.com/sandbox0-ai/infra/pkg/gateway/spec"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
)

// listTemplates lists available templates
func (s *Server) listTemplates(c *gin.Context) {
	// Get team ID from claims (optional, maybe filter by visibility later)
	claims := internalauth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		spec.JSONError(c, http.StatusUnauthorized, spec.CodeUnauthorized, "missing authentication")
		return
	}

	templates, err := s.templateService.ListTemplates(c.Request.Context())
	if err != nil {
		s.logger.Error("Failed to list templates", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to list templates: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{
		"templates": templates,
		"count":     len(templates),
	})
}

// getTemplate gets a template by ID
func (s *Server) getTemplate(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id is required")
		return
	}

	template, err := s.templateService.GetTemplate(c.Request.Context(), templateID)
	if err != nil {
		if errors.IsNotFound(err) {
			spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "template not found")
			return
		}
		s.logger.Error("Failed to get template", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to get template: %v", err))
		return
	}
	spec.JSONSuccess(c, http.StatusOK, template)
}

// createTemplate creates a new template
func (s *Server) createTemplate(c *gin.Context) {
	var template v1alpha1.SandboxTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	created, err := s.templateService.CreateTemplate(c.Request.Context(), &template)
	if err != nil {
		s.logger.Error("Failed to create template", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to create template: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusCreated, created)
}

// updateTemplate updates an existing template
func (s *Server) updateTemplate(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id is required")
		return
	}

	var template v1alpha1.SandboxTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Ensure ID matches
	if template.Name != "" && template.Name != templateID {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id in path does not match body")
		return
	}
	template.Name = templateID

	updated, err := s.templateService.UpdateTemplate(c.Request.Context(), &template)
	if err != nil {
		if errors.IsNotFound(err) {
			spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "template not found")
			return
		}
		s.logger.Error("Failed to update template", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to update template: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, updated)
}

// deleteTemplate deletes a template
func (s *Server) deleteTemplate(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id is required")
		return
	}

	err := s.templateService.DeleteTemplate(c.Request.Context(), templateID)
	if err != nil {
		if errors.IsNotFound(err) {
			spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "template not found")
			return
		}
		s.logger.Error("Failed to delete template", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to delete template: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{"message": "template deleted"})
}

// WarmPoolRequest represents the request body for warming the pool
type WarmPoolRequest struct {
	Count int32 `json:"count"`
}

// warmPool warms the pool for a template
func (s *Server) warmPool(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id is required")
		return
	}

	var req WarmPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	err := s.templateService.WarmPool(c.Request.Context(), templateID, req.Count)
	if err != nil {
		if errors.IsNotFound(err) {
			spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "template not found")
			return
		}
		s.logger.Error("Failed to warm pool", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to warm pool: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{"message": "pool warming triggered"})
}
