package setup

import (
	"fmt"
	"testing"
	"time"
)

// Cluster represents a lightweight fake cluster used by integration tests.
type Cluster struct {
	t         *testing.T
	Name      string
	Namespace string
	StartedAt time.Time
}

// NewFakeCluster creates a new in-process cluster scaffold for tests.
func NewFakeCluster(t *testing.T) *Cluster {
	t.Helper()

	return &Cluster{
		t:         t,
		Name:      "fake-cluster",
		Namespace: fmt.Sprintf("test-%d", time.Now().UnixNano()),
		StartedAt: time.Now(),
	}
}

// Cleanup tears down resources created by the fake cluster.
func (c *Cluster) Cleanup() {
	if c == nil || c.t == nil {
		return
	}
	c.t.Helper()
}
