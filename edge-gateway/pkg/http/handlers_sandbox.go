package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/edge-gateway/pkg/middleware"
	"github.com/sandbox0-ai/infra/pkg/auth"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"go.uber.org/zap"
)

type sandboxClaimRequest struct {
	Template  string `json:"template"`
	Namespace string `json:"namespace"`
}

type sandboxRouteRequest struct {
	TemplateID string `json:"template_id"`
	Namespace  string `json:"namespace,omitempty"`
}

type sandboxRouteResponse struct {
	ClusterID          string `json:"cluster_id"`
	InternalGatewayURL string `json:"internal_gateway_url"`
}

func (s *Server) routeSandboxClaim(c *gin.Context) {
	authCtx := middleware.GetAuthContext(c)
	if authCtx == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authentication"})
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var claim sandboxClaimRequest
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &claim); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
	}

	targetURL := ""
	if s.cfg.SchedulerEnabled && s.cfg.SchedulerURL != "" && s.schedulerClient != nil && claim.Template != "" {
		route, routeErr := s.fetchSandboxRoute(c, authCtx, claim.Template, claim.Namespace)
		if routeErr != nil {
			s.logger.Warn("Scheduler route failed, falling back to default",
				zap.Error(routeErr),
				zap.String("template", claim.Template),
			)
		} else if route.InternalGatewayURL != "" {
			targetURL = route.InternalGatewayURL
		}
	}

	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if targetURL == "" {
		s.proxyRouter.ProxyToTarget()(c)
		return
	}

	proxyHandler, err := s.proxyRouter.ProxyToURL(targetURL)
	if err != nil {
		s.logger.Warn("Invalid target URL from scheduler, falling back to default",
			zap.String("target_url", targetURL),
			zap.Error(err),
		)
		s.proxyRouter.ProxyToTarget()(c)
		return
	}

	proxyHandler(c)
}

func (s *Server) fetchSandboxRoute(c *gin.Context, authCtx *auth.AuthContext, templateID, namespace string) (*sandboxRouteResponse, error) {
	token, err := s.internalAuthGen.Generate(
		"scheduler",
		authCtx.TeamID,
		authCtx.UserID,
		internalauth.GenerateOptions{
			Permissions: authCtx.Permissions,
			RequestID:   middleware.GetRequestID(c),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("generate scheduler token: %w", err)
	}

	payload := sandboxRouteRequest{
		TemplateID: templateID,
		Namespace:  namespace,
	}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal route request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/sandboxes/route", s.cfg.SchedulerURL)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create route request: %w", err)
	}

	req.Header.Set(internalauth.DefaultTokenHeader, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Team-ID", authCtx.TeamID)
	if authCtx.UserID != "" {
		req.Header.Set("X-User-ID", authCtx.UserID)
	}

	resp, err := s.schedulerClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute route request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("route request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var route sandboxRouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&route); err != nil {
		return nil, fmt.Errorf("decode route response: %w", err)
	}

	return &route, nil
}
