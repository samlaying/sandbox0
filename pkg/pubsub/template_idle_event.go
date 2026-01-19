package pubsub

import "time"

// TemplateIdleChannel is the PostgreSQL NOTIFY channel for template stats updates.
const TemplateIdleChannel = "template_idle_events"

// TemplateIdleEvent represents template idle/active counts in a cluster.
type TemplateIdleEvent struct {
	ClusterID   string    `json:"cluster_id"`
	TemplateID  string    `json:"template_id"`
	IdleCount   int32     `json:"idle_count"`
	ActiveCount int32     `json:"active_count"`
	Timestamp   time.Time `json:"ts"`
}

// NowUTC returns a UTC timestamp for event payloads.
func NowUTC() time.Time {
	return time.Now().UTC()
}
