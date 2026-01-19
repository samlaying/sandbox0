package naming

import (
	"fmt"
	"strings"
)

// ParsedSandboxName provides decoded fields from a sandbox name.
type ParsedSandboxName struct {
	ClusterID  string
	ClusterKey string
}

// ParsedReplicasetName provides decoded fields from a replicaset name.
type ParsedReplicasetName struct {
	ClusterID  string
	ClusterKey string
}

// ParseSandboxName extracts the cluster ID from a sandbox (pod) name.
// Accepts names generated from ReplicaSet-based sandbox names.
func ParseSandboxName(name string) (*ParsedSandboxName, error) {
	clusterKey, err := extractClusterKey(name, sandboxNamePrefix)
	if err != nil {
		return nil, err
	}
	clusterID, err := decodeClusterKey(clusterKey)
	if err != nil {
		return nil, err
	}
	return &ParsedSandboxName{
		ClusterID:  clusterID,
		ClusterKey: clusterKey,
	}, nil
}

// ParseReplicasetName extracts the cluster ID from a replicaset name.
func ParseReplicasetName(name string) (*ParsedReplicasetName, error) {
	clusterKey, err := extractClusterKey(name, sandboxNamePrefix)
	if err != nil {
		return nil, err
	}
	clusterID, err := decodeClusterKey(clusterKey)
	if err != nil {
		return nil, err
	}
	return &ParsedReplicasetName{
		ClusterID:  clusterID,
		ClusterKey: clusterKey,
	}, nil
}

func extractClusterKey(name, prefix string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is empty")
	}
	expected := prefix + "-"
	if !strings.HasPrefix(name, expected) {
		return "", fmt.Errorf("name '%s' does not start with '%s'", name, expected)
	}
	rest := strings.TrimPrefix(name, expected)
	parts := strings.SplitN(rest, "-", 2)
	if len(parts) < 2 || parts[0] == "" {
		return "", fmt.Errorf("name '%s' does not include cluster key", name)
	}
	return parts[0], nil
}
