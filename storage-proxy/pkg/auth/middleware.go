package auth

import (
	"net/http"

	"github.com/sandbox0-ai/sandbox0/pkg/internalauth"
	obsmetrics "github.com/sandbox0-ai/sandbox0/pkg/observability/metrics"
	"go.uber.org/zap"
)

// HTTPAuthenticator handles HTTP request authentication using internalauth.
type HTTPAuthenticator struct {
	validator *internalauth.Validator
	logger    *zap.Logger
	metrics   *obsmetrics.StorageProxyMetrics
}

// NewHTTPAuthenticator creates a new HTTP authenticator.
func NewHTTPAuthenticator(validator *internalauth.Validator, logger *zap.Logger) *HTTPAuthenticator {
	return &HTTPAuthenticator{
		validator: validator,
		logger:    logger,
	}
}

func (a *HTTPAuthenticator) SetMetrics(metrics *obsmetrics.StorageProxyMetrics) {
	if a == nil {
		return
	}
	a.metrics = metrics
}

// Middleware returns an HTTP middleware for authentication.
func (a *HTTPAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from header
		tokenString := r.Header.Get("X-Internal-Token")
		if tokenString == "" {
			// Also try lowercase as headers are case-insensitive but good to be sure
			tokenString = r.Header.Get("x-internal-token")
		}

		if tokenString == "" {
			a.recordAuth("http", "missing_token", false)
			http.Error(w, "missing authentication token", http.StatusUnauthorized)
			return
		}

		// Validate token
		claims, err := a.validator.Validate(tokenString)
		if err != nil {
			a.recordAuth("http", "invalid_token", false)
			a.logger.Warn("Authentication failed",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Error(err),
			)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		a.recordAuth("http", "", true)

		// Add claims to context
		ctx := internalauth.WithClaims(r.Context(), claims)

		a.logger.Debug("Request authenticated",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("team_id", claims.TeamID),
			zap.String("caller", claims.Caller),
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *HTTPAuthenticator) recordAuth(protocol, errorType string, success bool) {
	if a == nil || a.metrics == nil {
		return
	}
	status := "success"
	if !success {
		status = "error"
		a.metrics.AuthenticationErrors.WithLabelValues(errorType).Inc()
	}
	a.metrics.AuthenticationTotal.WithLabelValues(protocol, status).Inc()
}

// HealthCheckMiddleware allows health check endpoints without authentication
func (a *HTTPAuthenticator) HealthCheckMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		a.Middleware(next).ServeHTTP(w, r)
	})
}
