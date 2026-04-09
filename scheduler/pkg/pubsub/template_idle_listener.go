package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	obsmetrics "github.com/sandbox0-ai/sandbox0/pkg/observability/metrics"
	"github.com/sandbox0-ai/sandbox0/pkg/pubsub"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TemplateIdleHandler handles template idle events.
type TemplateIdleHandler func(event pubsub.TemplateIdleEvent)

// StartTemplateIdleListener starts a LISTEN loop for template idle events.
func StartTemplateIdleListener(ctx context.Context, databaseURL string, logger *zap.Logger, tracer trace.Tracer, metrics *obsmetrics.SchedulerMetrics, handler TemplateIdleHandler) {
	go func() {
		backoff := time.Second
		for {
			if ctx.Err() != nil {
				return
			}

			if err := listenOnce(ctx, databaseURL, logger, tracer, metrics, handler); err != nil {
				if metrics != nil {
					metrics.TemplateIdleRestarts.Inc()
					metrics.TemplateIdleEvents.WithLabelValues("listener_error").Inc()
				}
				logger.Warn("Template idle listener stopped, retrying",
					zap.Error(err),
					zap.Duration("backoff", backoff),
				)
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if backoff < 10*time.Second {
					backoff *= 2
				}
				continue
			}
			backoff = time.Second
		}
	}()
}

func listenOnce(ctx context.Context, databaseURL string, logger *zap.Logger, tracer trace.Tracer, metrics *obsmetrics.SchedulerMetrics, handler TemplateIdleHandler) error {
	listenCtx := ctx
	var span trace.Span
	if tracer != nil {
		listenCtx, span = tracer.Start(ctx, "scheduler.template_idle_listener")
		defer span.End()
	}

	conn, err := pgx.Connect(listenCtx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() {
		_ = conn.Close(listenCtx)
	}()

	_, err = conn.Exec(listenCtx, "LISTEN "+pubsub.TemplateIdleChannel)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	logger.Info("Template idle listener started")

	for {
		notification, err := conn.WaitForNotification(listenCtx)
		if err != nil {
			if listenCtx.Err() != nil {
				return listenCtx.Err()
			}
			return fmt.Errorf("wait for notification: %w", err)
		}

		var event pubsub.TemplateIdleEvent
		if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil {
			if metrics != nil {
				metrics.TemplateIdleEvents.WithLabelValues("decode_error").Inc()
			}
			logger.Warn("Failed to decode template idle event", zap.Error(err))
			continue
		}

		if event.ClusterID == "" || event.TemplateID == "" {
			if metrics != nil {
				metrics.TemplateIdleEvents.WithLabelValues("invalid").Inc()
			}
			logger.Warn("Invalid template idle event payload",
				zap.String("cluster_id", event.ClusterID),
				zap.String("template_id", event.TemplateID),
			)
			continue
		}

		if metrics != nil {
			metrics.TemplateIdleEvents.WithLabelValues("success").Inc()
		}
		if tracer != nil {
			_, eventSpan := tracer.Start(listenCtx, "scheduler.template_idle_event",
				trace.WithAttributes(
					attribute.String("cluster_id", event.ClusterID),
					attribute.String("template_id", event.TemplateID),
					attribute.Int("idle_count", int(event.IdleCount)),
					attribute.Int("active_count", int(event.ActiveCount)),
				),
			)
			eventSpan.End()
		}
		if handler != nil {
			handler(event)
		}
	}
}
