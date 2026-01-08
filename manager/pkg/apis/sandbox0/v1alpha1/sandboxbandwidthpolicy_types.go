package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SandboxBandwidthPolicy defines bandwidth policy for a sandbox
type SandboxBandwidthPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SandboxBandwidthPolicySpec   `json:"spec"`
	Status SandboxBandwidthPolicyStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SandboxBandwidthPolicyList contains a list of SandboxBandwidthPolicy
type SandboxBandwidthPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SandboxBandwidthPolicy `json:"items"`
}

// SandboxBandwidthPolicySpec defines the desired state of SandboxBandwidthPolicy
type SandboxBandwidthPolicySpec struct {
	// SandboxID is the unique identifier of the sandbox this policy applies to
	SandboxID string `json:"sandboxId"`

	// TeamID is the team that owns this sandbox
	TeamID string `json:"teamId"`

	// EgressRateLimit defines egress rate limiting
	EgressRateLimit *RateLimitSpec `json:"egressRateLimit,omitempty"`

	// IngressRateLimit defines ingress rate limiting
	IngressRateLimit *RateLimitSpec `json:"ingressRateLimit,omitempty"`

	// Accounting defines traffic accounting configuration
	Accounting *AccountingSpec `json:"accounting,omitempty"`
}

// RateLimitSpec defines rate limiting specification
type RateLimitSpec struct {
	// RateBps is the rate limit in bits per second
	RateBps int64 `json:"rateBps"`

	// BurstBytes is the burst size in bytes
	BurstBytes int64 `json:"burstBytes,omitempty"`

	// CeilBps is the ceiling rate in bits per second (for HTB)
	CeilBps int64 `json:"ceilBps,omitempty"`
}

// AccountingSpec defines traffic accounting configuration
type AccountingSpec struct {
	// Enabled enables traffic accounting
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// ReportIntervalSeconds is the interval for reporting traffic statistics
	// Fixed at 10 seconds per platform policy
	// +kubebuilder:default=10
	ReportIntervalSeconds int32 `json:"reportIntervalSeconds,omitempty"`
}

// SandboxBandwidthPolicyStatus defines the observed state of SandboxBandwidthPolicy
type SandboxBandwidthPolicyStatus struct {
	// ObservedGeneration is the generation observed by netd
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastAppliedTime is the last time the policy was applied
	LastAppliedTime metav1.Time `json:"lastAppliedTime,omitempty"`

	// Phase is the current phase of the policy
	// +kubebuilder:validation:Enum=Pending;Applied;Failed
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the status
	Message string `json:"message,omitempty"`

	// CurrentStats contains current bandwidth statistics
	CurrentStats *BandwidthStats `json:"currentStats,omitempty"`
}

// BandwidthStats contains bandwidth usage statistics
type BandwidthStats struct {
	// EgressBytes is the total egress bytes
	EgressBytes int64 `json:"egressBytes,omitempty"`

	// IngressBytes is the total ingress bytes
	IngressBytes int64 `json:"ingressBytes,omitempty"`

	// EgressPackets is the total egress packets
	EgressPackets int64 `json:"egressPackets,omitempty"`

	// IngressPackets is the total ingress packets
	IngressPackets int64 `json:"ingressPackets,omitempty"`

	// ConnectionCount is the total number of connections
	ConnectionCount int64 `json:"connectionCount,omitempty"`

	// LastUpdated is the last time stats were updated
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}
