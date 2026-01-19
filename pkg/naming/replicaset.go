package naming

import (
	"fmt"
	"strings"

	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
)

func clusterIDOrDefault(clusterID *string) string {
	if clusterID != nil && *clusterID != "" {
		return *clusterID
	}
	return defaultClusterID
}

// ReplicasetNameForTemplate generates a ReplicaSet name for a template.
func ReplicasetNameForTemplate(template *v1alpha1.SandboxTemplate) (string, error) {
	clusterID := clusterIDOrDefault(template.Spec.ClusterId)
	return ReplicasetName(clusterID, template.Name)
}

// SandboxNameForTemplate generates a sandbox (pod) name for a template.
func SandboxNameForTemplate(template *v1alpha1.SandboxTemplate, randSuffix string) (string, error) {
	clusterID := clusterIDOrDefault(template.Spec.ClusterId)
	return SandboxName(clusterID, template.Name, randSuffix)
}

// ReplicasetName generates a Kubernetes-safe ReplicaSet name.
// Format: rs-<clusterKey>-<templateKey>
func ReplicasetName(clusterID, templateName string) (string, error) {
	clusterKey, err := encodeClusterID(clusterID)
	if err != nil {
		return "", err
	}
	prefix := fmt.Sprintf("%s-%s-", sandboxNamePrefix, clusterKey)
	remaining := replicaSetMaxLen - len(prefix)
	if remaining <= 0 {
		return "", fmt.Errorf("cluster key too long to build replicaset name")
	}
	templateKey, err := slugWithHash(templateName, remaining)
	if err != nil {
		return "", err
	}
	name := prefix + templateKey
	if err := validateDNSLabel(name); err != nil {
		return "", err
	}
	return name, nil
}

// SandboxName generates a sandbox (pod) name using the ReplicaSet name and a random suffix.
func SandboxName(clusterID, templateName, randSuffix string) (string, error) {
	if randSuffix == "" {
		return "", fmt.Errorf("randSuffix is empty")
	}
	if strings.Contains(randSuffix, "-") {
		return "", fmt.Errorf("randSuffix cannot contain hyphens")
	}
	if len(randSuffix) > podRandSuffixLen {
		return "", fmt.Errorf("randSuffix is too long (%d > %d)", len(randSuffix), podRandSuffixLen)
	}
	rsName, err := ReplicasetName(clusterID, templateName)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s", rsName, randSuffix)
	if err := validateDNSLabel(name); err != nil {
		return "", err
	}
	return name, nil
}

// CheckTemplate validates template naming constraints for K8s resources.
func CheckTemplate(template *v1alpha1.SandboxTemplate) error {
	clusterID := clusterIDOrDefault(template.Spec.ClusterId)
	if _, err := ReplicasetName(clusterID, template.Name); err != nil {
		return err
	}
	return nil
}
