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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// Beskar7ClusterSpec defines the desired state of Beskar7Cluster
type Beskar7ClusterSpec struct {
	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// FailureDomainLabels specifies the label keys to use for failure domain discovery.
	// If not specified, defaults to ["topology.kubernetes.io/zone"].
	// Labels are checked in order, and the first matching label is used.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	FailureDomainLabels []string `json:"failureDomainLabels,omitempty"`

	// TODO: Add any Beskar7 specific cluster configuration here
	// Example: Redfish discovery settings, global power management policies?
}

// Beskar7ClusterStatus defines the observed state of Beskar7Cluster
type Beskar7ClusterStatus struct {
	// Ready denotes that the cluster infrastructure is ready.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// Populated by the Beskar7Cluster controller.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// Conditions defines current service state of the Beskar7Cluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// FailureDomains specifies failure domain information for the cluster.
	// +optional
	FailureDomains clusterv1.FailureDomains `json:"failureDomains,omitempty"`

	// TODO: Add any observed status fields specific to Beskar7
}

// Beskar7Cluster specific conditions
const (
	// ControlPlaneEndpointReady documents the availability of the Control Plane Endpoint.
	ControlPlaneEndpointReady clusterv1.ConditionType = "ControlPlaneEndpointReady"
	// FailureDomainsReady documents the availability of failure domains.
	FailureDomainsReady clusterv1.ConditionType = "FailureDomainsReady"
)

// Beskar7Cluster condition reasons
const (
	// ControlPlaneEndpointNotSetReason indicates the ControlPlaneEndpoint is not defined in the spec.
	ControlPlaneEndpointNotSetReason = "ControlPlaneEndpointNotSet"
	// InvalidFailureDomainLabelReason indicates that a failure domain label has an invalid format.
	InvalidFailureDomainLabelReason = "InvalidFailureDomainLabel"
	// ListFailedReason indicates that listing PhysicalHosts failed.
	ListFailedReason = "ListFailed"
	// NoFailureDomainsReason indicates that no failure domains were found.
	NoFailureDomainsReason = "NoFailureDomains"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=beskar7clusters,scope=Namespaced,categories=cluster-api,shortName=b7c
//+kubebuilder:storageversion
//+kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster.x-k8s.io/cluster-name']",description="Cluster to which this Beskar7Cluster belongs"
//+kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.controlPlaneEndpoint.host",description="Control Plane Endpoint Host"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Cluster infrastructure is ready for bootstrapping"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Beskar7Cluster is the Schema for the beskar7clusters API
type Beskar7Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Beskar7ClusterSpec   `json:"spec,omitempty"`
	Status Beskar7ClusterStatus `json:"status,omitempty"`
}

// GetConditions returns the conditions for the Beskar7Cluster.
func (c *Beskar7Cluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions for the Beskar7Cluster.
func (c *Beskar7Cluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

//+kubebuilder:object:root=true

// Beskar7ClusterList contains a list of Beskar7Cluster
type Beskar7ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Beskar7Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Beskar7Cluster{}, &Beskar7ClusterList{})
}
