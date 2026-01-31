package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/pkg/gateway/spec"
	"github.com/sandbox0-ai/infra/pkg/proxy"
)

// === File System Handlers (→ Procd) ===

// handleFileOperation handles file operations (GET, POST, DELETE).
// Route: /api/v1/sandboxes/:id/files/*path
func (s *Server) handleFileOperation(c *gin.Context) {
	sandboxID := c.Param("id")
	filePath := c.Param("path")
	if sandboxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	if c.Request.Method == http.MethodGet && filePath == "/watch" {
		s.handleFileWatch(c)
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return // Error response already sent
	}

	if c.Request.Method == http.MethodPost && filePath == "/move" {
		c.Request.URL.Path = "/api/v1/files/move"
	} else {
		c.Request.URL.Path = "/api/v1/files" + filePath
	}

	s.proxyToProcd(c, procdURL)
}

// handleFileWatch handles WebSocket connection for file watching
// Route: WS /api/v1/sandboxes/:id/files/watch
func (s *Server) handleFileWatch(c *gin.Context) {
	sandboxID := c.Param("id")
	if sandboxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	procdURL, err := s.getProcdURL(c, sandboxID)
	if err != nil {
		return
	}

	requestModifier, err := s.buildProcdRequestModifier(c)
	if err != nil {
		return
	}

	// Handle WebSocket upgrade for file watching
	wsProxy := proxy.NewWebSocketProxy(s.logger, proxy.WithRequestModifier(requestModifier))
	c.Request.URL.Path = "/api/v1/files/watch"
	wsProxy.Proxy(procdURL)(c)
}
