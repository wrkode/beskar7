package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// PhysicalHost states - simplified for iPXE + inspection workflow
const (
	// StateNone is the default state before reconciliation
	StateNone = ""
	// StateUnknown indicates the host state could not be determined
	StateUnknown = "Unknown"
	// StateEnrolling indicates the controller is trying to establish connection
	StateEnrolling = "Enrolling"
	// StateAvailable indicates the host is ready to be claimed
	StateAvailable = "Available"
	// StateInUse indicates the host is claimed and being used by a Beskar7Machine
	StateInUse = "InUse"
	// StateInspecting indicates the inspection image is running on the host
	StateInspecting = "Inspecting"
	// StateReady indicates inspection is complete and host is ready for provisioning
	StateReady = "Ready"
	// StateError indicates the host is in an error state
	StateError = "Error"
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
// Simplified for power management only - provisioning happens via iPXE + inspection.
type PhysicalHostSpec struct {
	// RedfishConnection contains the connection details for the Redfish endpoint
	RedfishConnection RedfishConnection `json:"redfishConnection"`

	// ConsumerRef is a reference to the Beskar7Machine that is using this host
	// +optional
	ConsumerRef *corev1.ObjectReference `json:"consumerRef,omitempty"`
}

// InspectionPhase represents the current phase of hardware inspection
type InspectionPhase string

const (
	// InspectionPhasePending indicates inspection has not started
	InspectionPhasePending InspectionPhase = "Pending"
	// InspectionPhaseBooting indicates the inspection image is booting
	InspectionPhaseBooting InspectionPhase = "Booting"
	// InspectionPhaseInProgress indicates inspection is actively running
	InspectionPhaseInProgress InspectionPhase = "InProgress"
	// InspectionPhaseComplete indicates inspection finished successfully
	InspectionPhaseComplete InspectionPhase = "Complete"
	// InspectionPhaseFailed indicates inspection encountered an error
	InspectionPhaseFailed InspectionPhase = "Failed"
	// InspectionPhaseTimeout indicates inspection did not complete in time
	InspectionPhaseTimeout InspectionPhase = "Timeout"
)

// InspectionReport contains hardware information collected during inspection
type InspectionReport struct {
	// Timestamp when the inspection was performed
	Timestamp metav1.Time `json:"timestamp"`

	// CPUs contains CPU information
	CPUs CPUInfo `json:"cpus"`

	// Memory contains memory information
	Memory MemoryInfo `json:"memory"`

	// Disks contains information about storage devices
	// +optional
	Disks []DiskInfo `json:"disks,omitempty"`

	// NICs contains network interface information
	// +optional
	NICs []NICInfo `json:"nics,omitempty"`

	// System contains system/BIOS information
	System SystemInfo `json:"system"`

	// RawData contains the complete raw inspection output for debugging
	// +optional
	RawData string `json:"rawData,omitempty"`
}

// CPUInfo contains CPU-related information
type CPUInfo struct {
	// Count is the number of physical CPU sockets
	Count int `json:"count"`

	// Cores is the total number of CPU cores
	Cores int `json:"cores"`

	// Threads is the total number of CPU threads
	Threads int `json:"threads"`

	// Model is the CPU model name
	// +optional
	Model string `json:"model,omitempty"`

	// Architecture is the CPU architecture (e.g., x86_64, aarch64)
	// +optional
	Architecture string `json:"architecture,omitempty"`

	// MHz is the CPU frequency in MHz
	// +optional
	MHz float64 `json:"mhz,omitempty"`
}

// MemoryInfo contains memory information
type MemoryInfo struct {
	// TotalBytes is the total amount of physical memory in bytes
	TotalBytes int64 `json:"totalBytes"`

	// AvailableBytes is the available memory in bytes
	// +optional
	AvailableBytes int64 `json:"availableBytes,omitempty"`

	// TotalGB is the total memory in GB (derived from TotalBytes)
	TotalGB int `json:"totalGB"`
}

// DiskInfo contains information about a storage device
type DiskInfo struct {
	// Device is the device name (e.g., /dev/sda, /dev/nvme0n1)
	Device string `json:"device"`

	// SizeBytes is the size in bytes
	SizeBytes int64 `json:"sizeBytes"`

	// SizeGB is the size in GB
	SizeGB int `json:"sizeGB"`

	// Type is the disk type (SSD, HDD, NVMe)
	// +optional
	Type string `json:"type,omitempty"`

	// Model is the disk model
	// +optional
	Model string `json:"model,omitempty"`

	// Serial is the disk serial number
	// +optional
	Serial string `json:"serial,omitempty"`
}

// NICInfo contains network interface information
type NICInfo struct {
	// Interface is the interface name (e.g., eth0, ens3)
	Interface string `json:"interface"`

	// MACAddress is the MAC address
	MACAddress string `json:"macAddress"`

	// LinkStatus indicates if the link is up
	// +optional
	LinkStatus string `json:"linkStatus,omitempty"`

	// SpeedMbps is the link speed in Mbps
	// +optional
	SpeedMbps int `json:"speedMbps,omitempty"`

	// Driver is the network driver name
	// +optional
	Driver string `json:"driver,omitempty"`
}

// SystemInfo contains system/BIOS information
type SystemInfo struct {
	// Manufacturer is the system manufacturer
	// +optional
	Manufacturer string `json:"manufacturer,omitempty"`

	// Model is the system model
	// +optional
	Model string `json:"model,omitempty"`

	// SerialNumber is the system serial number
	// +optional
	SerialNumber string `json:"serialNumber,omitempty"`

	// BIOSVersion is the BIOS/UEFI version
	// +optional
	BIOSVersion string `json:"biosVersion,omitempty"`

	// BMCAddress is the BMC IP address (if detectable)
	// +optional
	BMCAddress string `json:"bmcAddress,omitempty"`
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

	// InspectionReport contains hardware details from the inspection phase
	// +optional
	InspectionReport *InspectionReport `json:"inspectionReport,omitempty"`

	// InspectionPhase tracks the current inspection progress
	// +optional
	InspectionPhase InspectionPhase `json:"inspectionPhase,omitempty"`

	// InspectionTimestamp is when inspection started
	// +optional
	InspectionTimestamp *metav1.Time `json:"inspectionTimestamp,omitempty"`

	// Conditions defines current service state of the PhysicalHost
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// Redfish conditions and reasons - simplified for power management only
const (
	RedfishConnectionReadyCondition clusterv1.ConditionType = "RedfishConnectionReady"
	HostAvailableCondition          clusterv1.ConditionType = "HostAvailable"
	HostInspectedCondition          clusterv1.ConditionType = "HostInspected"

	// Reasons
	MissingCredentialsReason      string = "MissingCredentials"
	SecretGetFailedReason         string = "SecretGetFailed"
	SecretNotFoundReason          string = "SecretNotFound"
	MissingSecretDataReason       string = "MissingSecretData"
	RedfishConnectionFailedReason string = "RedfishConnectionFailed"
	RedfishQueryFailedReason      string = "RedfishQueryFailed"
	PowerOnFailedReason           string = "PowerOnFailed"
	PowerOffFailedReason          string = "PowerOffFailed"
	SetBootPXEFailedReason        string = "SetBootPXEFailed"
	InspectionFailedReason        string = "InspectionFailed"
	InspectionTimeoutReason       string = "InspectionTimeout"
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
	if in.InspectionReport != nil {
		in, out := &in.InspectionReport, &out.InspectionReport
		*out = new(InspectionReport)
		(*in).DeepCopyInto(*out)
	}
	if in.InspectionTimestamp != nil {
		in, out := &in.InspectionTimestamp, &out.InspectionTimestamp
		*out = (*in).DeepCopy()
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(clusterv1.Conditions, len(*in))
		copy(*out, *in)
	}
}

// DeepCopyInto is an autogenerated deepcopy function for InspectionReport
func (in *InspectionReport) DeepCopyInto(out *InspectionReport) {
	*out = *in
	in.Timestamp.DeepCopyInto(&out.Timestamp)
	out.CPUs = in.CPUs
	out.Memory = in.Memory
	if in.Disks != nil {
		in, out := &in.Disks, &out.Disks
		*out = make([]DiskInfo, len(*in))
		copy(*out, *in)
	}
	if in.NICs != nil {
		in, out := &in.NICs, &out.NICs
		*out = make([]NICInfo, len(*in))
		copy(*out, *in)
	}
	out.System = in.System
}

// DeepCopy is an autogenerated deepcopy function for InspectionReport
func (in *InspectionReport) DeepCopy() *InspectionReport {
	if in == nil {
		return nil
	}
	out := new(InspectionReport)
	in.DeepCopyInto(out)
	return out
}

func init() {
	SchemeBuilder.Register(&PhysicalHost{}, &PhysicalHostList{})
}
