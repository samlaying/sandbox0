package daemon

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type daemonMetrics struct {
	redirectSyncTotal    *prometheus.CounterVec
	redirectSyncDuration prometheus.Histogram
	meteringFlushTotal   *prometheus.CounterVec
	ready                prometheus.Gauge
}

func newDaemonMetrics(registry prometheus.Registerer) *daemonMetrics {
	if registry == nil {
		return nil
	}

	factory := promauto.With(registry)
	return &daemonMetrics{
		redirectSyncTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "netd_redirect_sync_total",
			Help: "Total number of redirect sync attempts",
		}, []string{"status"}),
		redirectSyncDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "netd_redirect_sync_duration_seconds",
			Help:    "Duration of redirect sync runs",
			Buckets: prometheus.DefBuckets,
		}),
		meteringFlushTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "netd_metering_flush_total",
			Help: "Total number of metering flush attempts",
		}, []string{"status"}),
		ready: factory.NewGauge(prometheus.GaugeOpts{
			Name: "netd_ready",
			Help: "Whether netd is ready (1) or not ready (0)",
		}),
	}
}

func (m *daemonMetrics) observeRedirectSync(status string, duration time.Duration) {
	if m == nil {
		return
	}
	m.redirectSyncTotal.WithLabelValues(status).Inc()
	m.redirectSyncDuration.Observe(duration.Seconds())
}

func (m *daemonMetrics) observeMeteringFlush(status string) {
	if m == nil {
		return
	}
	m.meteringFlushTotal.WithLabelValues(status).Inc()
}

func (m *daemonMetrics) setReady(ready bool) {
	if m == nil {
		return
	}
	if ready {
		m.ready.Set(1)
		return
	}
	m.ready.Set(0)
}
