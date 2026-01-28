package proxy

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WebSocketProxy handles WebSocket connections proxying
type WebSocketProxy struct {
	logger *zap.Logger
	// requestModifiers are applied before proxying.
	requestModifiers []RequestModifier
	upgrader         websocket.Upgrader
}

// NewWebSocketProxy creates a new WebSocket proxy
func NewWebSocketProxy(logger *zap.Logger, opts ...Option) *WebSocketProxy {
	parsedOpts := collectOptions(opts...)
	return &WebSocketProxy{
		logger:           logger,
		requestModifiers: parsedOpts.requestModifiers,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}
}

// Proxy creates a WebSocket proxy handler
func (p *WebSocketProxy) Proxy(targetURL *url.URL) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if this is a WebSocket upgrade request
		if !isWebSocketUpgrade(c.Request) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "expected WebSocket upgrade request",
			})
			return
		}

		applyRequestModifiers(c.Request, p.requestModifiers)

		// Create target URL
		wsURL := *targetURL
		if wsURL.Scheme == "http" {
			wsURL.Scheme = "ws"
		} else if wsURL.Scheme == "https" {
			wsURL.Scheme = "wss"
		}
		wsURL.Path = c.Request.URL.Path
		wsURL.RawQuery = c.Request.URL.RawQuery

		upstreamHeaders := cloneWebSocketHeaders(c.Request.Header)
		upstreamConn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), upstreamHeaders)
		if err != nil {
			p.logger.Error("Failed to connect to upstream WebSocket",
				zap.String("target", wsURL.String()),
				zap.Error(err),
			)
			c.JSON(http.StatusBadGateway, gin.H{
				"error": "upstream websocket unavailable",
			})
			return
		}
		defer upstreamConn.Close()

		downstreamConn, err := p.upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			p.logger.Error("Failed to upgrade downstream WebSocket", zap.Error(err))
			return
		}
		defer downstreamConn.Close()

		errChan := make(chan error, 2)
		go func() { errChan <- proxyWebSocket(downstreamConn, upstreamConn) }()
		go func() { errChan <- proxyWebSocket(upstreamConn, downstreamConn) }()
		<-errChan
	}
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade request
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func cloneWebSocketHeaders(headers http.Header) http.Header {
	upstreamHeaders := headers.Clone()
	upstreamHeaders.Del("Connection")
	upstreamHeaders.Del("Upgrade")
	upstreamHeaders.Del("Sec-WebSocket-Key")
	upstreamHeaders.Del("Sec-WebSocket-Version")
	upstreamHeaders.Del("Sec-WebSocket-Extensions")
	return upstreamHeaders
}

func proxyWebSocket(dst *websocket.Conn, src *websocket.Conn) error {
	for {
		msgType, msg, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if err := dst.WriteMessage(msgType, msg); err != nil {
			return err
		}
	}
}
