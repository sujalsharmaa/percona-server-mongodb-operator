package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PerconaServerMongoDBClusterSyncSpec uses CR existence as the lifecycle
// signal — there is no `enabled` field; creating starts replication and
// deleting tears down owned resources.
type PerconaServerMongoDBClusterSyncSpec struct {
	ClusterName string `json:"clusterName"`
	Image       string `json:"image"`

	ImagePullPolicy  corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	Resources                corev1.ResourceRequirements `json:"resources,omitempty"`
	NodeSelector             map[string]string           `json:"nodeSelector,omitempty"`
	Tolerations              []corev1.Toleration         `json:"tolerations,omitempty"`
	Affinity                 *PodAffinity                `json:"affinity,omitempty"`
	Annotations              map[string]string           `json:"annotations,omitempty"`
	Labels                   map[string]string           `json:"labels,omitempty"`
	RuntimeClassName         *string                     `json:"runtimeClassName,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext     `json:"containerSecurityContext,omitempty"`
	PodSecurityContext       *corev1.PodSecurityContext  `json:"podSecurityContext,omitempty"`

	Source ClusterSyncSource `json:"source"`

	// Expose fronts the PCSM HTTP API on port 2242.
	Expose Expose `json:"expose,omitempty"`

	// ExcludeNamespaces lists MongoDB namespaces
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`

	// Mode is the user intent for the PCSM lifecycle and the sole driver
	// of /start, /pause, /resume, /reset, /finalize HTTP calls.
	// +kubebuilder:default=paused
	Mode ClusterSyncMode `json:"mode,omitempty"`
}

// ClusterSyncMode is the user-controlled lifecycle intent for PCSM. All
// transitions are explicit and user-driven; the controller never auto-
// transitions between values.
// +kubebuilder:validation:Enum={paused,running,reset,finalized}
type ClusterSyncMode string

const (
	ClusterSyncModePaused    ClusterSyncMode = "paused"
	ClusterSyncModeRunning   ClusterSyncMode = "running"
	ClusterSyncModeReset     ClusterSyncMode = "reset"
	ClusterSyncModeFinalized ClusterSyncMode = "finalized"
)

type ClusterSyncSource struct {
	// URI is the source connection string WITHOUT credentials; credentials
	// from CredentialsSecret are injected at runtime, percent-encoded.
	URI string `json:"uri"`

	// CredentialsSecret is a same-namespace Secret with `username` and
	// `password` keys.
	CredentialsSecret string `json:"credentialsSecret"`

	TLS *ClusterSyncTLS `json:"tls,omitempty"`
}

// ClusterSyncTLS configures only the PCSM-to-source TLS; target TLS is
// auto-derived from the target PSMDB CR's existing secrets.
type ClusterSyncTLS struct {
	Enabled bool   `json:"enabled,omitempty"`
	Secret  string `json:"secret,omitempty"`
}

type ClusterSyncState string

const (
	ClusterSyncStateNew         ClusterSyncState = ""
	ClusterSyncStatePending     ClusterSyncState = "pending"
	ClusterSyncStateInitialSync ClusterSyncState = "initialSync"
	ClusterSyncStateReplicating ClusterSyncState = "replicating"
	ClusterSyncStatePaused      ClusterSyncState = "paused"
	ClusterSyncStateFailed      ClusterSyncState = "failed"
	ClusterSyncStateFinalized   ClusterSyncState = "finalized"
)

const (
	ConditionClusterSyncInitialSyncComplete = "InitialSyncComplete"
	ConditionClusterSyncReplicating         = "Replicating"
	ConditionClusterSyncFinalized           = "Finalized"
)

type PerconaServerMongoDBClusterSyncStatus struct {
	// Mode mirrors spec.mode after the corresponding HTTP call has
	// succeeded; the controller compares the two to decide whether a
	// transition is required. Empty until the first mode has been
	// applied.
	Mode ClusterSyncMode `json:"mode,omitempty"`

	// State is the PCSM-reported runtime state from GET /status, distinct
	// from spec.mode/status.mode: those are user intent, this is what
	// PCSM is actually doing.
	State ClusterSyncState `json:"state,omitempty"`

	LagTimeSeconds int64  `json:"lagTimeSeconds,omitempty"`
	Error          string `json:"error,omitempty"`

	// StartedAt is set once when PCSM leaves "pending"; not updated on
	// restart.
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// FinalizerDeleteAfterFinalize opts the user in to deleting the CR
// itself after post-finalize cleanup completes.
const FinalizerDeleteAfterFinalize = "percona.com/delete-clustersync-after-finalize"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaServerMongoDBClusterSync is the Schema for the
// perconaservermongodbclustersyncs API.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName="psmdb-clustersync"
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=".spec.clusterName",description="Target cluster name"
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=".spec.mode",description="User-requested lifecycle mode"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state",description="Replication state"
// +kubebuilder:printcolumn:name="Lag(s)",type=integer,JSONPath=".status.lagTimeSeconds",description="Replication lag in seconds"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp",description="Created time"
type PerconaServerMongoDBClusterSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PerconaServerMongoDBClusterSyncSpec   `json:"spec,omitempty"`
	Status PerconaServerMongoDBClusterSyncStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaServerMongoDBClusterSyncList contains a list of PerconaServerMongoDBClusterSync.
type PerconaServerMongoDBClusterSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PerconaServerMongoDBClusterSync `json:"items"`
}
