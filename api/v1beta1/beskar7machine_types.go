package v1beta1

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

// Beskar7MachineSpec defines the desired state of Beskar7Machine.
type Beskar7MachineSpec struct {
	// ProviderID is the unique identifier as specified by the cloud provider.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// ImageURL is the URL of the OS image to use for the machine.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^(https?|ftp|file)://.*\\.(iso|img|qcow2|vmdk|raw|vhd|vhdx|ova|ovf)(\\.(gz|bz2|xz|zip|tar|tgz|tbz2|txz))?$"
	ImageURL string `json:"imageURL"`

	// ConfigURL is the URL of the configuration to use for the machine.
	// +kubebuilder:validation:Pattern="^(https?|file)://.*\\.(yaml|yml|json|toml|conf|cfg|ini|properties)$"
	// +optional
	ConfigURL string `json:"configURL,omitempty"`

	// OSFamily is the operating system family to use for the machine.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=kairos;flatcar;LeapMicro
	OSFamily string `json:"osFamily"`

	// ProvisioningMode is the mode to use for provisioning the machine.
	// +kubebuilder:validation:Enum=RemoteConfig;PreBakedISO;PXE;iPXE
	// +kubebuilder:default="RemoteConfig"
	// +optional
	ProvisioningMode string `json:"provisioningMode,omitempty"`

	// BootMode specifies the boot mode for the machine (UEFI or Legacy).
	// +kubebuilder:validation:Enum=UEFI;Legacy
	// +kubebuilder:default="UEFI"
	// +optional
	BootMode string `json:"bootMode,omitempty"`
}

// Beskar7MachineStatus defines the observed state of Beskar7Machine.
type Beskar7MachineStatus struct {
	// Ready indicates whether the machine is ready
	Ready bool `json:"ready,omitempty"`

	// Phase represents the current phase of the machine
	Phase *string `json:"phase,omitempty"`

	// FailureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	FailureReason *string `json:"failureReason,omitempty"`

	// FailureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Addresses contains the associated addresses for the machine.
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Conditions defines current service state of the Beskar7Machine.
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=beskar7machines,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this Beskar7Machine belongs"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/machine-name",description="Machine to which this Beskar7Machine belongs"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Beskar7Machine phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of Beskar7Machine"
// +kubebuilder:object:generate=true

// Beskar7Machine is the Schema for the beskar7machines API.
type Beskar7Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Beskar7MachineSpec   `json:"spec,omitempty"`
	Status Beskar7MachineStatus `json:"status,omitempty"`
}

// GetConditions returns the observations of the operational state of the Beskar7Machine resource.
func (m *Beskar7Machine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the underlying service state of the Beskar7Machine to the pre-defined clusterv1.Conditions.
func (m *Beskar7Machine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// Beskar7MachineList contains a list of Beskar7Machine.
type Beskar7MachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Beskar7Machine `json:"items"`
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Beskar7MachineSpec) DeepCopyInto(out *Beskar7MachineSpec) {
	*out = *in
	if in.ProviderID != nil {
		in, out := &in.ProviderID, &out.ProviderID
		*out = new(string)
		**out = **in
	}
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Beskar7MachineStatus) DeepCopyInto(out *Beskar7MachineStatus) {
	*out = *in
	if in.Phase != nil {
		in, out := &in.Phase, &out.Phase
		*out = new(string)
		**out = **in
	}
	if in.FailureReason != nil {
		in, out := &in.FailureReason, &out.FailureReason
		*out = new(string)
		**out = **in
	}
	if in.FailureMessage != nil {
		in, out := &in.FailureMessage, &out.FailureMessage
		*out = new(string)
		**out = **in
	}
	if in.Addresses != nil {
		in, out := &in.Addresses, &out.Addresses
		*out = make([]clusterv1.MachineAddress, len(*in))
		copy(*out, *in)
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(clusterv1.Conditions, len(*in))
		copy(*out, *in)
	}
}

func init() {
	SchemeBuilder.Register(&Beskar7Machine{}, &Beskar7MachineList{})
}
