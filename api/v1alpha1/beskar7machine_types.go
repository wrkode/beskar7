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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// Beskar7MachineSpec defines the desired state of Beskar7Machine
type Beskar7MachineSpec struct {
	// ProviderID will be the reference to the PhysicalHost used for this machine.
	// Format: b7:////<namespace>/<physicalhost-name>
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// Image is the details of the OS image to boot the machine with.
	// +kubebuilder:validation:Required
	Image Image `json:"image"`

	// UserDataSecret contains a reference to a Secret containing the cloud-init user data.
	// +optional
	UserDataSecret *corev1.LocalObjectReference `json:"userDataSecret,omitempty"`

	// TODO: Add fields like HardwareSelector (to match PhysicalHost labels?)
	// TODO: Add power management options?
}

// Image defines the details of the OS image.
type Image struct {
	// URL is the location of the ISO image.
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Checksum is the SHA256 checksum of the ISO image.
	// +optional
	Checksum *string `json:"checksum,omitempty"`
	// TODO: Add ChecksumType if supporting other algorithms?
}

// Beskar7Machine specific conditions
const (
	// PhysicalHostAssociatedCondition documents the successful association of a PhysicalHost with this Beskar7Machine.
	PhysicalHostAssociatedCondition clusterv1.ConditionType = "PhysicalHostAssociated"
	// InfrastructureReadyCondition indicates that the underlying infrastructure (PhysicalHost) is provisioned and ready.
	// This becomes true once the PhysicalHostAssociatedCondition is true AND the associated PhysicalHost is ready.
	InfrastructureReadyCondition clusterv1.ConditionType = "InfrastructureReady"

	// TODO: Add other conditions like UserDataSecretReady?
)

// Beskar7Machine condition reasons
const (
	// WaitingForPhysicalHostReason indicates the controller is waiting for an available PhysicalHost to claim.
	WaitingForPhysicalHostReason = "WaitingForPhysicalHost"
	// PhysicalHostAssociatedReason indicates a PhysicalHost has been successfully claimed.
	PhysicalHostAssociatedReason = "PhysicalHostAssociated"
	// PhysicalHostAssociationFailedReason indicates an error occurred trying to claim a PhysicalHost.
	PhysicalHostAssociationFailedReason = "PhysicalHostAssociationFailed"
	// PhysicalHostNotFoundReason indicates the associated PhysicalHost (by ProviderID or label) could not be found.
	PhysicalHostNotFoundReason = "PhysicalHostNotFound"
	// PhysicalHostNotReadyReason indicates the associated PhysicalHost is not yet ready/provisioned.
	PhysicalHostNotReadyReason = "PhysicalHostNotReady"
	// PhysicalHostReleasedReason indicates the associated PhysicalHost has been released during deletion.
	PhysicalHostReleasedReason = "PhysicalHostReleased"
	// ReleasePhysicalHostFailedReason indicates an error occurred trying to release the PhysicalHost.
	ReleasePhysicalHostFailedReason = "ReleasePhysicalHostFailed"
	// PhysicalHostErrorReason indicates the associated PhysicalHost is in an Error state.
	PhysicalHostErrorReason = "PhysicalHostError"
)

// Beskar7MachineStatus defines the observed state of Beskar7Machine
type Beskar7MachineStatus struct {
	// Ready denotes that the machine is ready.
	// +optional
	Ready bool `json:"ready"`

	// Addresses contains the associated addresses for the machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Phase represents the current phase of machine actuation.
	// E.g. Pending, Provisioning, Provisioned, Running, Deleting, Failed.
	// +optional
	Phase *string `json:"phase,omitempty"`

	// FailureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"` // Consider using CAPI errors/conditions

	// FailureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Conditions defines current service state of the Beskar7Machine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// GetConditions returns the conditions for the Beskar7Machine.
func (m *Beskar7Machine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions for the Beskar7Machine.
func (m *Beskar7Machine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=beskar7machines,scope=Namespaced,categories=cluster-api,shortName=b7m
//+kubebuilder:storageversion
//+kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster.x-k8s.io/cluster-name']",description="Cluster to which this Beskar7Machine belongs"
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.phase",description="Machine status such as Terminating/Pending/Running/Failed etc"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
//+kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID",description="PhysicalHost instance ID"
//+kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind=='Machine')].name",description="Machine object which owns this Beskar7Machine"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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

func init() {
	SchemeBuilder.Register(&Beskar7Machine{}, &Beskar7MachineList{})
}
