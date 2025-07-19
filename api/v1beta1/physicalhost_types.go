package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// PhysicalHost states
const (
	// StateNone is the default state before reconciliation
	StateNone = ""
	// StateEnrolling indicates the controller is trying to establish connection
	StateEnrolling = "Enrolling"
	// StateAvailable indicates the host is ready to be claimed
	StateAvailable = "Available"
	// StateClaimed indicates the host is reserved by a consumer
	StateClaimed = "Claimed"
	// StateProvisioning indicates the host is being configured
	StateProvisioning = "Provisioning"
	// StateProvisioned indicates the host has been successfully configured
	StateProvisioned = "Provisioned"
	// StateDeprovisioning indicates the host is being cleaned up
	StateDeprovisioning = "Deprovisioning"
	// StateError indicates the host is in an error state
	StateError = "Error"
	// StateUnknown indicates the host state could not be determined
	StateUnknown = "Unknown"
)

// RedfishConnection contains the information needed to connect to a Redfish service
type RedfishConnection struct {
	// Address is the URL of the Redfish service
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^(https?://)[a-zA-Z0-9.-]+(:[0-9]+)?(/.*)?$"
	Address string `json:"address"`

	// CredentialsSecretRef is the name of the secret containing the Redfish credentials
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	CredentialsSecretRef string `json:"credentialsSecretRef"`

	// InsecureSkipVerify determines whether to skip TLS certificate verification
	// +kubebuilder:default=false
	// +optional
	InsecureSkipVerify *bool `json:"insecureSkipVerify,omitempty"`
}

// HardwareDetails contains information about the physical host hardware
type HardwareDetails struct {
	// Manufacturer is the manufacturer of the physical host
	Manufacturer string `json:"manufacturer,omitempty"`

	// Model is the model of the physical host
	Model string `json:"model,omitempty"`

	// SerialNumber is the serial number of the physical host
	SerialNumber string `json:"serialNumber,omitempty"`

	// Status contains the current status of the host
	Status HardwareStatus `json:"status,omitempty"`
}

// HardwareStatus contains the current status of the host hardware
type HardwareStatus struct {
	// Health is the health status of the host
	// +optional
	Health string `json:"health,omitempty"`

	// HealthRollup is the overall health status
	// +optional
	HealthRollup string `json:"healthRollup,omitempty"`

	// State is the current state of the host
	// +optional
	State string `json:"state,omitempty"`
}

// PhysicalHostSpec defines the desired state of PhysicalHost
type PhysicalHostSpec struct {
	// RedfishConnection contains the connection details for the Redfish endpoint
	RedfishConnection RedfishConnection `json:"redfishConnection"`

	// ConsumerRef is a reference to the Beskar7Machine that is using this host
	// +optional
	ConsumerRef *corev1.ObjectReference `json:"consumerRef,omitempty"`

	// BootISOSource is the URL of the ISO image to use for provisioning
	// +optional
	BootISOSource *string `json:"bootIsoSource,omitempty"`

	// UserDataSecretRef is a reference to a secret containing cloud-init user data
	// +optional
	UserDataSecretRef *corev1.ObjectReference `json:"userDataSecretRef,omitempty"`
}

// PhysicalHostStatus defines the observed state of PhysicalHost
type PhysicalHostStatus struct {
	// Ready indicates if the host is ready and enrolled
	// +optional
	Ready bool `json:"ready,omitempty"`

	// State represents the current state of the host
	// +optional
	State string `json:"state,omitempty"`

	// ObservedPowerState is the last observed power state from Redfish endpoint
	// +optional
	ObservedPowerState string `json:"observedPowerState,omitempty"`

	// ErrorMessage contains details on the last error encountered
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`

	// HardwareDetails contains information about the physical host hardware
	// +optional
	HardwareDetails HardwareDetails `json:"hardwareDetails,omitempty"`

	// Addresses contains the associated addresses for the host
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Conditions defines current service state of the PhysicalHost
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// Redfish and provisioning conditions and reasons
const (
	RedfishConnectionReadyCondition clusterv1.ConditionType = "RedfishConnectionReady"
	HostAvailableCondition          clusterv1.ConditionType = "HostAvailable"
	HostProvisionedCondition        clusterv1.ConditionType = "HostProvisioned"

	// Reasons
	MissingCredentialsReason      string = "MissingCredentials"
	SecretGetFailedReason         string = "SecretGetFailed"
	SecretNotFoundReason          string = "SecretNotFound"
	MissingSecretDataReason       string = "MissingSecretData"
	RedfishConnectionFailedReason string = "RedfishConnectionFailed"
	RedfishQueryFailedReason      string = "RedfishQueryFailed"
	WaitingForBootInfoReason      string = "WaitingForBootInfo"
	ProvisioningReason            string = "Provisioning"
	SetBootISOFailedReason        string = "SetBootISOFailed"
	PowerOnFailedReason           string = "PowerOnFailed"
	DeprovisioningReason          string = "Deprovisioning"
	EjectMediaFailedReason        string = "EjectMediaFailed"
	PowerOffFailedReason          string = "PowerOffFailed"
)

// RedfishConnectionInfo contains the information needed to connect to a Redfish service
type RedfishConnectionInfo struct {
	// Address is the URL of the Redfish service
	Address string `json:"address"`

	// CredentialsSecretRef is the name of the secret containing the Redfish credentials
	CredentialsSecretRef string `json:"credentialsSecretRef"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=physicalhosts,scope=Namespaced,categories=cluster-api,shortName=ph
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state",description="Current state of the Physical Host"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready",description="Indicates if the host is ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Creation timestamp"
// +kubebuilder:storageversion

// PhysicalHost is the Schema for the physicalhosts API
type PhysicalHost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PhysicalHostSpec   `json:"spec,omitempty"`
	Status PhysicalHostStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PhysicalHostList contains a list of PhysicalHost
type PhysicalHostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PhysicalHost `json:"items"`
}

// GetConditions returns the conditions for the PhysicalHost
func (h *PhysicalHost) GetConditions() clusterv1.Conditions {
	return h.Status.Conditions
}

// SetConditions sets the conditions for the PhysicalHost
func (h *PhysicalHost) SetConditions(conditions clusterv1.Conditions) {
	h.Status.Conditions = conditions
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PhysicalHostSpec) DeepCopyInto(out *PhysicalHostSpec) {
	*out = *in
	// RedfishConnection is a value type, so direct assignment is fine
	out.RedfishConnection = in.RedfishConnection
	if in.ConsumerRef != nil {
		in, out := &in.ConsumerRef, &out.ConsumerRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
	if in.BootISOSource != nil {
		in, out := &in.BootISOSource, &out.BootISOSource
		*out = new(string)
		**out = **in
	}
	if in.UserDataSecretRef != nil {
		in, out := &in.UserDataSecretRef, &out.UserDataSecretRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PhysicalHostStatus) DeepCopyInto(out *PhysicalHostStatus) {
	*out = *in
	// HardwareDetails is a value type, so direct assignment is fine
	out.HardwareDetails = in.HardwareDetails
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
	SchemeBuilder.Register(&PhysicalHost{}, &PhysicalHostList{})
}
