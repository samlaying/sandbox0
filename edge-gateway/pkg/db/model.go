package db

import "time"

// APIKey represents an API key stored in the database
type APIKey struct {
	ID         string    `json:"id"`
	KeyValue   string    `json:"key_value"`
	TeamID     string    `json:"team_id"`
	CreatedBy  string    `json:"created_by"`
	Name       string    `json:"name"`
	Type       string    `json:"type"` // 'user', 'service', 'internal'
	Roles      []string  `json:"roles"`
	IsActive   bool      `json:"is_active"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastUsed   time.Time `json:"last_used_at"`
	UsageCount int64     `json:"usage_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
