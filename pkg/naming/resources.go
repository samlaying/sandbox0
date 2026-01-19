package naming

import "fmt"

// TeamName returns a Kubernetes-safe name for a team resource.
func TeamName(teamID string) (string, error) {
	return slugWithHash(fmt.Sprintf("team-%s", teamID), dnsLabelMaxLen)
}

// UserName returns a Kubernetes-safe name for a user resource.
func UserName(userID string) (string, error) {
	return slugWithHash(fmt.Sprintf("user-%s", userID), dnsLabelMaxLen)
}

// ClusterName returns a Kubernetes-safe name for a cluster resource.
func ClusterName(clusterID string) (string, error) {
	key, err := encodeClusterID(clusterID)
	if err != nil {
		return "", err
	}
	return slugWithHash(fmt.Sprintf("cluster-%s", key), dnsLabelMaxLen)
}

// VolumeName returns a Kubernetes-safe name for a sandbox volume resource.
func VolumeName(teamID, volumeID string) (string, error) {
	return slugWithHash(fmt.Sprintf("vol-%s-%s", teamID, volumeID), dnsLabelMaxLen)
}

// SnapshotName returns a Kubernetes-safe name for a sandbox volume snapshot resource.
func SnapshotName(volumeID, snapshotID string) (string, error) {
	return slugWithHash(fmt.Sprintf("snap-%s-%s", volumeID, snapshotID), dnsLabelMaxLen)
}
