package naming

import "testing"

func TestReplicasetAndSandboxNames(t *testing.T) {
	clusterID := "aws/us-east-1"
	templateName := "basic-template"

	rsName, err := ReplicasetName(clusterID, templateName)
	if err != nil {
		t.Fatalf("replicaset name: %v", err)
	}
	if len(rsName) > replicaSetMaxLen {
		t.Fatalf("replicaset name too long: %d", len(rsName))
	}

	sandboxName, err := SandboxName(clusterID, templateName, "abcde")
	if err != nil {
		t.Fatalf("sandbox name: %v", err)
	}

	parsed, err := ParseSandboxName(sandboxName)
	if err != nil {
		t.Fatalf("parse sandbox name: %v", err)
	}
	if parsed.ClusterID != clusterID {
		t.Fatalf("expected clusterID %s, got %s", clusterID, parsed.ClusterID)
	}
}

func TestTemplateNameForCluster(t *testing.T) {
	name := TemplateNameForCluster(ScopeTeam, "team-123", "my-template-name")
	if name == "" {
		t.Fatalf("expected template name to be non-empty")
	}
	if len(name) > dnsLabelMaxLen {
		t.Fatalf("template name too long: %d", len(name))
	}
	if err := validateDNSLabel(name); err != nil {
		t.Fatalf("template name invalid: %v", err)
	}
}

func TestSlugWithHashTruncates(t *testing.T) {
	input := "This-Is-A-Very-Long-Template-Name-With-Invalid---Chars"
	name, err := slugWithHash(input, 20)
	if err != nil {
		t.Fatalf("slugWithHash: %v", err)
	}
	if len(name) > 20 {
		t.Fatalf("expected length <= 20, got %d", len(name))
	}
	if err := validateDNSLabel(name); err != nil {
		t.Fatalf("generated name invalid: %v", err)
	}
}
