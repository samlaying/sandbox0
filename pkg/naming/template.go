package naming

import "fmt"

const (
	ScopePublic = "public"
	ScopeTeam   = "team"
)

// TenantKey returns a stable short key for a team ID.
// It is intentionally short to keep derived Kubernetes resource names within limits.
func TenantKey(teamID string) string {
	return shortHash(teamID)
}

// TeamKey is an alias for TenantKey to keep naming consistent.
func TeamKey(teamID string) string {
	return TenantKey(teamID)
}

// TemplateNameForCluster returns a Kubernetes-safe name for storing a template in a cluster.
//
// For public templates, the name is the templateID.
// For team-scoped templates, the name includes a stable team key to avoid cross-tenant collisions.
func TemplateNameForCluster(scope, teamID, templateID string) string {
	if scope != ScopeTeam {
		name, err := slugWithHash(templateID, dnsLabelMaxLen)
		if err != nil {
			return fmt.Sprintf("tpl-%s", shortHash(templateID))
		}
		return name
	}

	teamKey := TeamKey(teamID)
	prefix := fmt.Sprintf("t-%s-", teamKey)
	remaining := dnsLabelMaxLen - len(prefix)
	if remaining <= 0 {
		return fmt.Sprintf("t-%s-%s", teamKey, shortHash(templateID))
	}
	name, err := slugWithHash(templateID, remaining)
	if err != nil {
		return fmt.Sprintf("t-%s-%s", teamKey, shortHash(templateID))
	}
	return prefix + name
}
