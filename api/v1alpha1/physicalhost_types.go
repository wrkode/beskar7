/*
Copyright 2024 The Beskar7 Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// PhysicalHostProvisioningState defines the states a PhysicalHost can be in.
type PhysicalHostProvisioningState string

const (
	// StateNone is the default state before reconciliation.
	StateNone PhysicalHostProvisioningState = ""
	// StateEnrolling means the controller is trying to establish connection and gather info.
	StateEnrolling PhysicalHostProvisioningState = "Enrolling"
	// StateAvailable means the host is ready to be claimed.
	StateAvailable PhysicalHostProvisioningState = "Available"
	// StateClaimed means the host is reserved by a consumer.
	StateClaimed PhysicalHostProvisioningState = "Claimed"
	// StateProvisioning means the host is being configured (ISO boot, power on).
	StateProvisioning PhysicalHostProvisioningState = "Provisioning"
	// StateProvisioned means the host has been successfully configured and powered on.
	StateProvisioned PhysicalHostProvisioningState = "Provisioned"
	// StateDeprovisioning means the host is being cleaned up.
	StateDeprovisioning PhysicalHostProvisioningState = "Deprovisioning"
	// StateError means the host is in an error state.
	StateError PhysicalHostProvisioningState = "Error"
	// StateUnknown means the host state could not be determined.
	StateUnknown PhysicalHostProvisioningState = "Unknown"
)

// PhysicalHostSpec defines the desired state of PhysicalHost
type PhysicalHostSpec struct {
	// RedfishConnection contains the details needed to connect to the Redfish API
	// +kubebuilder:validation:Required
	RedfishConnection RedfishConnectionInfo `json:"redfishConnection"`

	// ConsumerRef is a reference to the Beskar7Machine that is using this host.
	// +optional
	ConsumerRef *corev1.ObjectReference `json:"consumerRef,omitempty"`

	// BootISOSource is the URL of the ISO image to use for provisioning.
	// Set by the consumer (Beskar7Machine controller) to trigger provisioning.
	// +optional
	BootISOSource *string `json:"bootIsoSource,omitempty"`

	// UserDataSecretRef is a reference to a secret containing cloud-init user data.
	// Passed via annotation by CAPI? Or set directly by consumer?
	// +optional
	UserDataSecretRef *corev1.LocalObjectReference `json:"userDataSecretRef,omitempty"`

	// TODO: Add DesiredPowerState? (On/Off)
}

// RedfishConnectionInfo contains Redfish endpoint details
type RedfishConnectionInfo struct {
	// Address is the URL of the Redfish API endpoint (e.g., https://192.168.1.100)
	// +kubebuilder:validation:Required
	Address string `json:"address"`

	// CredentialsSecretRef is a reference to a Secret containing the username and password
	// for Redfish authentication.
	// +kubebuilder:validation:Required
	CredentialsSecretRef string `json:"credentialsSecretRef"`

	// InsecureSkipVerify specifies whether to skip TLS certificate verification.
	// Use with caution.
	// +optional
	InsecureSkipVerify *bool `json:"insecureSkipVerify,omitempty"`
}

// PhysicalHostStatus defines the observed state of PhysicalHost
type PhysicalHostStatus struct {
	// Ready indicates if the host is ready and enrolled.
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// State represents the current provisioning state of the host.
	// +optional
	State PhysicalHostProvisioningState `json:"state,omitempty"`

	// ObservedPowerState reflects the power state last observed from the Redfish endpoint.
	// +optional
	ObservedPowerState redfish.PowerState `json:"observedPowerState,omitempty"`

	// ErrorMessage provides details on the last error encountered.
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`

	// HardwareDetails contains discovered information about the host's hardware.
	// +optional
	HardwareDetails *HardwareDetails `json:"hardwareDetails,omitempty"`

	// Conditions represent the latest available observations of an object's state.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"` // Use clusterv1.Conditions type
}

// HardwareDetails stores hardware information discovered via Redfish.
type HardwareDetails struct {
	Manufacturer string        `json:"manufacturer,omitempty"`
	Model        string        `json:"model,omitempty"`
	SerialNumber string        `json:"serialNumber,omitempty"`
	Status       common.Status `json:"status,omitempty"`
}

// PhysicalHost specific conditions
const (
	// RedfishConnectionReadyCondition documents the connectivity status to the Redfish endpoint.
	RedfishConnectionReadyCondition clusterv1.ConditionType = "RedfishConnectionReady"

	// HostAvailableCondition documents whether the host is available for claiming (unclaimed and enrolled).
	HostAvailableCondition clusterv1.ConditionType = "HostAvailable"

	// HostProvisionedCondition documents whether the host has been provisioned according to its spec.
	HostProvisionedCondition clusterv1.ConditionType = "HostProvisioned"
)

// PhysicalHost condition reasons
const (
	// WaitingForCredentialsReason indicates the controller is waiting for the user to provide credentials.
	WaitingForCredentialsReason = "WaitingForCredentials"
	// MissingCredentialsReason indicates the credential secret reference is missing in the spec.
	MissingCredentialsReason = "MissingCredentialsSecretRef"
	// SecretGetFailedReason indicates the referenced credential secret could not be retrieved.
	SecretGetFailedReason = "SecretGetFailed"
	// SecretNotFoundReason indicates the referenced credential secret was not found.
	SecretNotFoundReason = "SecretNotFound"
	// MissingSecretDataReason indicates the credential secret is missing required data (username/password).
	MissingSecretDataReason = "MissingSecretData"
	// RedfishConnectionFailedReason indicates the controller could not establish a connection to the Redfish endpoint.
	RedfishConnectionFailedReason = "RedfishConnectionFailed"
	// RedfishQueryFailedReason indicates a query to the Redfish endpoint failed after connection.
	RedfishQueryFailedReason = "RedfishQueryFailed"
	// WaitingForBootInfoReason indicates the host is claimed but waiting for BootISOSource.
	WaitingForBootInfoReason = "WaitingForBootInfo"
	// ProvisioningReason indicates the host is currently being provisioned (setting boot source, powering on).
	ProvisioningReason = "Provisioning"
	// SetBootISOFailedReason indicates setting the boot ISO via Redfish failed.
	SetBootISOFailedReason = "SetBootISOFailed"
	// PowerOnFailedReason indicates powering on the host via Redfish failed.
	PowerOnFailedReason = "PowerOnFailed"
	// PowerOffFailedReason indicates powering off the host via Redfish failed.
	PowerOffFailedReason = "PowerOffFailed"
	// EjectMediaFailedReason indicates ejecting virtual media via Redfish failed.
	EjectMediaFailedReason = "EjectMediaFailed"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=physicalhosts,scope=Namespaced,shortName=ph
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state",description="Current state of the Physical Host"
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready",description="Indicates if the host is ready"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PhysicalHost is the Schema for the physicalhosts API
type PhysicalHost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PhysicalHostSpec   `json:"spec,omitempty"`
	Status PhysicalHostStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PhysicalHostList contains a list of PhysicalHost
type PhysicalHostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PhysicalHost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PhysicalHost{}, &PhysicalHostList{})
}

// GetConditions returns the list of conditions for a PhysicalHost.
func (ph *PhysicalHost) GetConditions() clusterv1.Conditions {
	return ph.Status.Conditions
}

// SetConditions sets the conditions on a PhysicalHost.
func (ph *PhysicalHost) SetConditions(conditions clusterv1.Conditions) {
	ph.Status.Conditions = conditions
}
