package http

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"github.com/sandbox0-ai/infra/pkg/templatenaming"
	"github.com/sandbox0-ai/infra/scheduler/pkg/db"
	"go.uber.org/zap"
)

// SandboxRouteRequest represents a request to select a cluster for sandbox creation.
type SandboxRouteRequest struct {
	TemplateID string `json:"template_id"`
	Namespace  string `json:"namespace,omitempty"`
}

// SandboxRouteResponse represents routing decision result.
type SandboxRouteResponse struct {
	ClusterID          string `json:"cluster_id"`
	InternalGatewayURL string `json:"internal_gateway_url"`
	TemplateID         string `json:"template_id"`
	Scope              string `json:"scope"`
	TeamID             string `json:"team_id,omitempty"`
	SelectedBy         string `json:"selected_by"`
}

// routeSandbox selects a cluster for sandbox creation based on template idle stats.
func (s *Server) routeSandbox(c *gin.Context) {
	var req SandboxRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.TemplateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template_id is required"})
		return
	}

	claims := internalauth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authentication"})
		return
	}

	template, err := s.repo.GetTemplateForTeam(c.Request.Context(), claims.TeamID, req.TemplateID)
	if err != nil {
		s.logger.Error("Failed to get template for routing", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to route sandbox"})
		return
	}
	if template == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}

	allocations, err := s.repo.ListAllocationsByTemplate(c.Request.Context(), template.Scope, template.TeamID, template.TemplateID)
	if err != nil {
		s.logger.Error("Failed to list template allocations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to route sandbox"})
		return
	}
	if len(allocations) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no clusters available for template"})
		return
	}

	clusters, err := s.repo.ListEnabledClusters(c.Request.Context())
	if err != nil {
		s.logger.Error("Failed to list enabled clusters", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to route sandbox"})
		return
	}

	clusterMap := make(map[string]*db.Cluster, len(clusters))
	for _, cluster := range clusters {
		clusterMap[cluster.ClusterID] = cluster
	}

	clusterTemplateID := templatenaming.TemplateNameForCluster(template.Scope, template.TeamID, template.TemplateID)
	maxAge := s.cfg.ReconcileInterval * 2

	var selected *db.Cluster
	selectedBy := "weight"
	var selectedAlloc *db.TemplateAllocation
	var bestIdle int32 = -1

	for _, alloc := range allocations {
		cluster := clusterMap[alloc.ClusterID]
		if cluster == nil || !cluster.Enabled {
			continue
		}

		age, ok := s.reconciler.GetTemplateStatsAge(cluster.ClusterID)
		if !ok || age > maxAge {
			continue
		}

		idleCount, ok := s.reconciler.GetTemplateIdleCount(cluster.ClusterID, clusterTemplateID)
		if !ok || idleCount <= 0 {
			continue
		}

		if selected == nil ||
			idleCount > bestIdle ||
			(idleCount == bestIdle && alloc.MaxIdle > selectedAlloc.MaxIdle) ||
			(idleCount == bestIdle && alloc.MaxIdle == selectedAlloc.MaxIdle && cluster.Weight > selected.Weight) {
			selected = cluster
			selectedAlloc = alloc
			bestIdle = idleCount
			selectedBy = "idle"
		}
	}

	if selected == nil {
		totalWeight := 0
		for _, alloc := range allocations {
			cluster := clusterMap[alloc.ClusterID]
			if cluster == nil || !cluster.Enabled {
				continue
			}
			if cluster.Weight <= 0 {
				continue
			}
			totalWeight += cluster.Weight
		}

		if totalWeight == 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no clusters available for template"})
			return
		}

		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		choice := rng.Intn(totalWeight)
		running := 0
		for _, alloc := range allocations {
			cluster := clusterMap[alloc.ClusterID]
			if cluster == nil || !cluster.Enabled {
				continue
			}
			if cluster.Weight <= 0 {
				continue
			}
			running += cluster.Weight
			if choice < running {
				selected = cluster
				selectedAlloc = alloc
				break
			}
		}
	}

	if selected == nil || selectedAlloc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no clusters available for template"})
		return
	}

	s.logger.Info("Sandbox route selected",
		zap.String("template_id", template.TemplateID),
		zap.String("scope", template.Scope),
		zap.String("team_id", template.TeamID),
		zap.String("cluster_id", selected.ClusterID),
		zap.String("selected_by", selectedBy),
	)

	c.JSON(http.StatusOK, SandboxRouteResponse{
		ClusterID:          selected.ClusterID,
		InternalGatewayURL: selected.InternalGatewayURL,
		TemplateID:         template.TemplateID,
		Scope:              template.Scope,
		TeamID:             template.TeamID,
		SelectedBy:         selectedBy,
	})
}
