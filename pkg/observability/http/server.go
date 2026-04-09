package http

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// ServerConfig configures HTTP server observability middleware.
type ServerConfig struct {
	ServiceName string
	Tracer      trace.Tracer
	Logger      *zap.Logger
	Registry    prometheus.Registerer
	Disabled    bool
}

type serverMetrics struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	activeRequests  *prometheus.GaugeVec
}

func newServerMetrics(serviceName string, registry prometheus.Registerer) *serverMetrics {
	if serviceName == "" || registry == nil {
		return nil
	}

	factory := promauto.With(registry)
	return &serverMetrics{
		requestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: serviceName + "_http_server_requests_total",
				Help: "Total number of HTTP server requests",
			},
			[]string{"method", "route", "status"},
		),
		requestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    serviceName + "_http_server_request_duration_seconds",
				Help:    "HTTP server request duration in seconds",
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
			},
			[]string{"method", "route"},
		),
		activeRequests: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: serviceName + "_http_server_active_requests",
				Help: "Number of in-flight HTTP server requests",
			},
			[]string{"method", "route"},
		),
	}
}

// ServerMiddleware returns net/http middleware with tracing and optional logging.
func ServerMiddleware(cfg ServerConfig) func(http.Handler) http.Handler {
	metrics := newServerMetrics(cfg.ServiceName, cfg.Registry)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.Disabled {
				next.ServeHTTP(w, r)
				return
			}

			tracer := cfg.Tracer
			if tracer == nil {
				tracer = otel.Tracer("http-server")
			}

			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			start := time.Now()
			route := r.URL.Path
			if metrics != nil {
				metrics.activeRequests.WithLabelValues(r.Method, route).Inc()
				defer metrics.activeRequests.WithLabelValues(r.Method, route).Dec()
			}

			spanName := fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.host", r.Host),
					attribute.String("http.scheme", r.URL.Scheme),
					attribute.String("http.target", r.URL.Path),
				),
			)
			defer span.End()

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			status := wrapped.statusCode
			span.SetAttributes(attribute.Int("http.status_code", status))
			if status >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
			}
			if metrics != nil {
				statusLabel := fmt.Sprintf("%d", status)
				metrics.requestsTotal.WithLabelValues(r.Method, route, statusLabel).Inc()
				metrics.requestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
			}

			if cfg.Logger != nil {
				level := zap.InfoLevel
				if status >= 500 {
					level = zap.ErrorLevel
				} else if status >= 400 {
					level = zap.WarnLevel
				}
				cfg.Logger.Log(level, "HTTP request",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", status),
					zap.Duration("latency", time.Since(start)),
					zap.String("client_ip", r.RemoteAddr),
					zap.String("trace_id", span.SpanContext().TraceID().String()),
					zap.String("span_id", span.SpanContext().SpanID().String()),
				)
			}
		})
	}
}

// GinMiddleware returns gin middleware with tracing and optional logging.
func GinMiddleware(cfg ServerConfig) gin.HandlerFunc {
	metrics := newServerMetrics(cfg.ServiceName, cfg.Registry)
	return func(c *gin.Context) {
		if cfg.Disabled {
			c.Next()
			return
		}

		tracer := cfg.Tracer
		if tracer == nil {
			tracer = otel.Tracer("http-server")
		}

		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		start := time.Now()
		route := c.Request.URL.Path
		if metrics != nil {
			metrics.activeRequests.WithLabelValues(c.Request.Method, route).Inc()
			defer metrics.activeRequests.WithLabelValues(c.Request.Method, route).Dec()
		}

		spanName := fmt.Sprintf("HTTP %s %s", c.Request.Method, c.Request.URL.Path)
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", c.Request.Method),
				attribute.String("http.url", c.Request.URL.String()),
				attribute.String("http.host", c.Request.Host),
				attribute.String("http.scheme", c.Request.URL.Scheme),
				attribute.String("http.target", c.Request.URL.Path),
			),
		)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		status := c.Writer.Status()
		if matchedRoute := c.FullPath(); matchedRoute != "" {
			route = matchedRoute
			span.SetAttributes(attribute.String("http.route", matchedRoute))
			span.SetName(fmt.Sprintf("HTTP %s %s", c.Request.Method, matchedRoute))
		}

		span.SetAttributes(attribute.Int("http.status_code", status))
		if status >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		}
		if metrics != nil {
			statusLabel := fmt.Sprintf("%d", status)
			metrics.requestsTotal.WithLabelValues(c.Request.Method, route, statusLabel).Inc()
			metrics.requestDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
		}

		if cfg.Logger != nil {
			level := zap.InfoLevel
			if status >= 500 {
				level = zap.ErrorLevel
			} else if status >= 400 {
				level = zap.WarnLevel
			}
			cfg.Logger.Log(level, "HTTP request",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Int("status", status),
				zap.Duration("latency", time.Since(start)),
				zap.String("client_ip", c.ClientIP()),
				zap.String("trace_id", span.SpanContext().TraceID().String()),
				zap.String("span_id", span.SpanContext().SpanID().String()),
			)
		}
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
