package templatenaming

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

const (
	ScopePublic = "public"
	ScopeTeam   = "team"
)

// TenantKey returns a stable short key for a team ID.
// It is intentionally short to keep derived Kubernetes resource names within limits.
func TenantKey(teamID string) string {
	sum := sha1.Sum([]byte(teamID))
	return hex.EncodeToString(sum[:])[:8]
}

// TemplateNameForCluster returns a Kubernetes-safe name for storing a template in a cluster.
//
// For public templates, the name is the templateID.
// For team-scoped templates, the name includes a stable team key to avoid cross-tenant collisions.
func TemplateNameForCluster(scope, teamID, templateID string) string {
	if scope != ScopeTeam {
		return templateID
	}

	teamKey := TenantKey(teamID)
	prefix := fmt.Sprintf("t-%s-", teamKey)
	name := prefix + templateID

	// Kubernetes names are limited to 63 chars (DNS-1123 label).
	if len(name) <= 63 {
		return name
	}

	// Truncate the template ID and add a suffix hash to keep uniqueness.
	tplHash := TenantKey(templateID)
	remaining := 63 - len(prefix)
	if remaining <= 0 {
		// Extremely defensive fallback.
		return fmt.Sprintf("t-%s-%s", teamKey, tplHash)
	}

	// Need: <prefix><truncated>-<tplHash>
	minusHashLen := 1 + len(tplHash)
	if remaining <= minusHashLen {
		return prefix + tplHash
	}

	truncLen := remaining - minusHashLen
	return prefix + templateID[:truncLen] + "-" + tplHash
}

