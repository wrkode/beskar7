package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// InfrastructureReadyCondition reports on the readiness of the infrastructure provider.
	InfrastructureReadyCondition clusterv1.ConditionType = "InfrastructureReady"
	// PhysicalHostAssociatedCondition indicates whether the Beskar7Machine has
	// successfully associated with a PhysicalHost.
	PhysicalHostAssociatedCondition clusterv1.ConditionType = "PhysicalHostAssociated"
)

// Reasons for condition failures
const (
	// PhysicalHostAssociationFailedReason (Severity=Warning) indicates that the Beskar7Machine
	// failed to associate with a PhysicalHost.
	PhysicalHostAssociationFailedReason string = "PhysicalHostAssociationFailed"
	// WaitingForPhysicalHostReason (Severity=Info) indicates that the Beskar7Machine
	// is waiting for an available PhysicalHost to be claimed.
	WaitingForPhysicalHostReason string = "WaitingForPhysicalHost"
	// PhysicalHostNotReadyReason (Severity=Info) indicates that the associated PhysicalHost
	// is not yet in a Ready state (e.g., still provisioning).
	PhysicalHostNotReadyReason string = "PhysicalHostNotReady"
	// PhysicalHostErrorReason (Severity=Error) indicates that the associated PhysicalHost
	// is in an Error state.
	PhysicalHostErrorReason string = "PhysicalHostError"
	// ReleasePhysicalHostFailedReason (Severity=Warning) indicates that releasing the
	// associated PhysicalHost failed during deletion.
	ReleasePhysicalHostFailedReason string = "ReleasePhysicalHostFailed"
)

// Beskar7MachineSpec defines the desired state of Beskar7Machine
type Beskar7MachineSpec struct {
	// ProviderID is the unique identifier for the instance assigned by the provider.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// OSFamily specifies the target operating system family for this machine.
	// This determines how provisioning is handled (e.g., kernel parameters for config).
	// Examples: "kairos", "talos", "flatcar", "LeapMicro".
	// +kubebuilder:validation:Enum=kairos;talos;flatcar;LeapMicro
	OSFamily string `json:"osFamily"`

	// ImageURL specifies the URL of the boot ISO image.
	// This can be a generic installer ISO (for remote config) or a pre-baked ISO.
	ImageURL string `json:"imageURL"`

	// ProvisioningMode specifies how the OS should be provisioned.
	// "RemoteConfig": The OS will fetch its configuration from ConfigURL.
	// "PreBakedISO": The ImageURL points to an ISO that already has the configuration embedded.
	// If omitted and ConfigURL is provided, "RemoteConfig" is assumed.
	// If omitted and ConfigURL is not provided, "PreBakedISO" is assumed (though an image without config might not be useful).
	// +kubebuilder:validation:Enum=RemoteConfig;PreBakedISO
	// +optional
	ProvisioningMode string `json:"provisioningMode,omitempty"`

	// ConfigURL is the URL from which the OS should fetch its configuration file.
	// Required if ProvisioningMode is "RemoteConfig".
	// +optional
	ConfigURL string `json:"configURL,omitempty"`
}

// Beskar7MachineStatus defines the observed state of Beskar7Machine
type Beskar7MachineStatus struct {
	// Ready indicates the machine is ready.
	// +optional
	Ready bool `json:"ready"`

	// Addresses contains the associated addresses for the machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// IPAddresses contains the IP addresses of the machine, categorized by type.
	// +optional
	IPAddresses struct {
		// InternalIPs contains the internal IP addresses of the machine.
		// +optional
		InternalIPs []string `json:"internalIPs,omitempty"`

		// ExternalIPs contains the external IP addresses of the machine.
		// +optional
		ExternalIPs []string `json:"externalIPs,omitempty"`

		// PreferredIP is the preferred IP address to use for this machine.
		// This is typically the first internal IP, or if none exists, the first external IP.
		// +optional
		PreferredIP string `json:"preferredIP,omitempty"`
	} `json:"ipAddresses,omitempty"`

	// Phase represents the current phase of machine provisioning.
	// E.g., Pending, Provisioning, Provisioned, Failed, Deleting
	// +optional
	Phase *string `json:"phase,omitempty"`

	// FailureReason will be set in the case when there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// FailureMessage will be set in the case when there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Conditions defines current service state of the Beskar7Machine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster.x-k8s.io/cluster-name",description="Cluster to which this Beskar7Machine belongs"
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.phase",description="Current state of the Beskar7Machine"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
//+kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID",description="Provider ID"
//+kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind=='Machine')].name",description="Machine object which owns this Beskar7Machine"

// Beskar7Machine is the Schema for the beskar7machines API
type Beskar7Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Beskar7MachineSpec   `json:"spec,omitempty"`
	Status Beskar7MachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// Beskar7MachineList contains a list of Beskar7Machine
type Beskar7MachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Beskar7Machine `json:"items"`
}

// GetConditions returns the observations of the operational state of the Beskar7Machine resource.
func (m *Beskar7Machine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the underlying service state of the Beskar7Machine to the pre-defined clusterv1.Conditions.
func (m *Beskar7Machine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Beskar7Machine{}, &Beskar7MachineList{})
}
