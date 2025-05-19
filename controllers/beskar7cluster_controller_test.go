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

package controllers

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	conditions "sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Beskar7Cluster Reconciler", func() {
	var (
		ctx         context.Context
		testNs      *corev1.Namespace
		b7cluster   *infrastructurev1alpha1.Beskar7Cluster
		capiCluster *clusterv1.Cluster
		key         types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create a namespace for the test
		testNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "b7cluster-reconciler-",
			},
		}
		Expect(k8sClient.Create(ctx, testNs)).To(Succeed())

		// Create owner CAPI Cluster
		capiCluster = &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: testNs.Name,
			},
			Spec: clusterv1.ClusterSpec{
				// InfrastructureRef is needed for GetOwnerCluster
				InfrastructureRef: &corev1.ObjectReference{
					APIVersion: infrastructurev1alpha1.GroupVersion.String(),
					Kind:       "Beskar7Cluster",
					Name:       "test-b7cluster",
					Namespace:  testNs.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, capiCluster)).To(Succeed())

		// Basic Beskar7Cluster object
		b7cluster = &infrastructurev1alpha1.Beskar7Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-b7cluster",
				Namespace: testNs.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       capiCluster.Name,
						UID:        capiCluster.UID,
					},
				},
			},
			Spec: infrastructurev1alpha1.Beskar7ClusterSpec{
				// ControlPlaneEndpoint will be derived by the controller
			},
		}
		key = types.NamespacedName{Name: b7cluster.Name, Namespace: b7cluster.Namespace}
	})

	AfterEach(func() {
		// Clean up the namespace and resources
		Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
	})

	Context("Reconcile Normal", func() {
		It("should add finalizer and wait for control plane machines", func() {
			Expect(k8sClient.Create(ctx, b7cluster)).To(Succeed())

			reconciler := &Beskar7ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			// Check finalizer is added
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, b7cluster)).To(Succeed())
				g.Expect(b7cluster.Finalizers).To(ContainElement(Beskar7ClusterFinalizer))
			}, "5s", "100ms").Should(Succeed())

			// Second reconcile tries to find endpoint, fails, sets condition, and requeues
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue when no machines are found")

			// Check condition and status
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, b7cluster)).To(Succeed())
				cond := conditions.Get(b7cluster, infrastructurev1alpha1.ControlPlaneEndpointReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(infrastructurev1alpha1.ControlPlaneEndpointNotSetReason))
				g.Expect(b7cluster.Status.Ready).To(BeFalse())
				g.Expect(b7cluster.Status.ControlPlaneEndpoint.IsZero()).To(BeTrue())
			}, "5s", "100ms").Should(Succeed(), "ControlPlaneEndpointReady should be False")
		})

		It("should derive endpoint when a ready control plane machine has an IP", func() {
			Expect(k8sClient.Create(ctx, b7cluster)).To(Succeed())

			// Create the CAPI Machine object (spec only first)
			cpMachine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "controlplane-0",
					Namespace: testNs.Name,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:       capiCluster.Name,
						"cluster.x-k8s.io/control-plane": "", // Mark as control plane
					},
				},
				Spec: clusterv1.MachineSpec{ClusterName: capiCluster.Name},
				// Status will be updated below
			}
			Expect(k8sClient.Create(ctx, cpMachine)).To(Succeed())

			By("Setting the Machine's Status to Ready with an IP")
			cpMachineKey := client.ObjectKeyFromObject(cpMachine)
			Eventually(func(g Gomega) error {
				// Fetch the machine first to get the latest ResourceVersion for update
				machineToUpdate := &clusterv1.Machine{}
				if err := k8sClient.Get(ctx, cpMachineKey, machineToUpdate); err != nil {
					return err
				}
				// Set the desired status fields
				machineToUpdate.Status.InfrastructureReady = true
				machineToUpdate.Status.Addresses = []clusterv1.MachineAddress{
					{Type: clusterv1.MachineExternalIP, Address: "1.1.1.1"},
					{Type: clusterv1.MachineInternalIP, Address: "192.168.1.10"},
				}
				conditions.MarkTrue(machineToUpdate, clusterv1.InfrastructureReadyCondition)
				// Attempt the status update
				return k8sClient.Status().Update(ctx, machineToUpdate)
			}, "10s", "100ms").Should(Succeed(), "Failed to update Machine status")

			By("Ensuring the Machine status conditions are readable")
			Eventually(func(g Gomega) {
				updatedMachine := &clusterv1.Machine{}
				g.Expect(k8sClient.Get(ctx, cpMachineKey, updatedMachine)).To(Succeed())
				g.Expect(conditions.IsTrue(updatedMachine, clusterv1.InfrastructureReadyCondition)).To(BeTrue())
			}, "10s", "100ms").Should(Succeed(), "Machine condition InfrastructureReady should be True after update")

			reconciler := &Beskar7ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile should find the machine and set the endpoint
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue once endpoint is derived")
			Expect(result.RequeueAfter).To(BeZero())

			// Check condition and status
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, b7cluster)).To(Succeed())
				cond := conditions.Get(b7cluster, infrastructurev1alpha1.ControlPlaneEndpointReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
				g.Expect(b7cluster.Status.Ready).To(BeTrue())
				g.Expect(b7cluster.Status.ControlPlaneEndpoint.Host).To(Equal("192.168.1.10"))
				g.Expect(b7cluster.Status.ControlPlaneEndpoint.Port).To(Equal(int32(6443)))
			}, "5s", "100ms").Should(Succeed(), "ControlPlaneEndpoint should be derived correctly")
		})

		// TODO: Add test for machine ready but no address
		// TODO: Add test for machine ready but only external address
		// TODO: Add test for machine not ready

		// TODO: Add tests for failure domain discovery
		It("should discover FailureDomains from PhysicalHost labels", func() {
			// Create the Beskar7Cluster first (will have finalizer added on first reconcile)
			Expect(k8sClient.Create(ctx, b7cluster)).To(Succeed())
			reconciler := &Beskar7ClusterReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Create PhysicalHosts with different zone labels
			zoneLabel := "topology.kubernetes.io/zone"
			ph1 := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "fd-host-1", Namespace: testNs.Name, Labels: map[string]string{zoneLabel: "zone-a"}},
				Spec:       infrastructurev1alpha1.PhysicalHostSpec{RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{Address: "dummy1", CredentialsSecretRef: "dummy"}},
			}
			ph2 := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "fd-host-2", Namespace: testNs.Name, Labels: map[string]string{zoneLabel: "zone-b"}},
				Spec:       infrastructurev1alpha1.PhysicalHostSpec{RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{Address: "dummy2", CredentialsSecretRef: "dummy"}},
			}
			ph3 := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "fd-host-3", Namespace: testNs.Name, Labels: map[string]string{zoneLabel: "zone-a"}}, // Duplicate zone
				Spec:       infrastructurev1alpha1.PhysicalHostSpec{RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{Address: "dummy3", CredentialsSecretRef: "dummy"}},
			}
			ph4 := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "fd-host-4", Namespace: testNs.Name}, // No zone label
				Spec:       infrastructurev1alpha1.PhysicalHostSpec{RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{Address: "dummy4", CredentialsSecretRef: "dummy"}},
			}
			Expect(k8sClient.Create(ctx, ph1)).To(Succeed())
			Expect(k8sClient.Create(ctx, ph2)).To(Succeed())
			Expect(k8sClient.Create(ctx, ph3)).To(Succeed())
			Expect(k8sClient.Create(ctx, ph4)).To(Succeed())

			// Reconcile again to trigger FailureDomain discovery
			_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Check FailureDomains in status
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, b7cluster)).To(Succeed())
				g.Expect(b7cluster.Status.FailureDomains).To(HaveLen(2), "Should discover 2 unique zones")
				g.Expect(b7cluster.Status.FailureDomains).To(HaveKey("zone-a"))
				g.Expect(b7cluster.Status.FailureDomains["zone-a"]).To(Equal(clusterv1.FailureDomainSpec{ControlPlane: true}))
				g.Expect(b7cluster.Status.FailureDomains).To(HaveKey("zone-b"))
				g.Expect(b7cluster.Status.FailureDomains["zone-b"]).To(Equal(clusterv1.FailureDomainSpec{ControlPlane: true}))
			}, "5s", "100ms").Should(Succeed(), "FailureDomains should be discovered correctly")
		})

		It("should support custom failure domain labels", func() {
			// Create the Beskar7Cluster with custom failure domain label
			customLabel := "custom.zone"
			b7cluster.Spec.FailureDomainLabel = customLabel
			Expect(k8sClient.Create(ctx, b7cluster)).To(Succeed())
			reconciler := &Beskar7ClusterReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Create PhysicalHosts with custom zone labels
			ph1 := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-fd-host-1", Namespace: testNs.Name, Labels: map[string]string{customLabel: "custom-zone-1"}},
				Spec:       infrastructurev1alpha1.PhysicalHostSpec{RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{Address: "dummy1", CredentialsSecretRef: "dummy"}},
			}
			ph2 := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-fd-host-2", Namespace: testNs.Name, Labels: map[string]string{customLabel: "custom-zone-2"}},
				Spec:       infrastructurev1alpha1.PhysicalHostSpec{RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{Address: "dummy2", CredentialsSecretRef: "dummy"}},
			}
			// Create a host with standard label (should be ignored)
			ph3 := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "standard-fd-host", Namespace: testNs.Name, Labels: map[string]string{"topology.kubernetes.io/zone": "standard-zone"}},
				Spec:       infrastructurev1alpha1.PhysicalHostSpec{RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{Address: "dummy3", CredentialsSecretRef: "dummy"}},
			}
			Expect(k8sClient.Create(ctx, ph1)).To(Succeed())
			Expect(k8sClient.Create(ctx, ph2)).To(Succeed())
			Expect(k8sClient.Create(ctx, ph3)).To(Succeed())

			// Reconcile again to trigger FailureDomain discovery
			_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Check FailureDomains in status
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, b7cluster)).To(Succeed())
				g.Expect(b7cluster.Status.FailureDomains).To(HaveLen(2), "Should discover 2 unique custom zones")
				g.Expect(b7cluster.Status.FailureDomains).To(HaveKey("custom-zone-1"))
				g.Expect(b7cluster.Status.FailureDomains["custom-zone-1"]).To(Equal(clusterv1.FailureDomainSpec{ControlPlane: true}))
				g.Expect(b7cluster.Status.FailureDomains).To(HaveKey("custom-zone-2"))
				g.Expect(b7cluster.Status.FailureDomains["custom-zone-2"]).To(Equal(clusterv1.FailureDomainSpec{ControlPlane: true}))
				g.Expect(b7cluster.Status.FailureDomains).NotTo(HaveKey("standard-zone"), "Should not discover zones with standard label")
			}, "5s", "100ms").Should(Succeed(), "Custom failure domains should be discovered correctly")
		})
	})

	Context("Reconcile Delete", func() {
		It("should remove the finalizer upon deletion", func() {
			By("Creating Beskar7Cluster with finalizer")
			b7cluster.Finalizers = []string{Beskar7ClusterFinalizer}
			Expect(k8sClient.Create(ctx, b7cluster)).To(Succeed())

			By("Deleting the Beskar7Cluster")
			Expect(k8sClient.Delete(ctx, b7cluster)).To(Succeed())

			By("Reconciling after deletion")
			reconciler := &Beskar7ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue after finalizer removal")

			By("Checking if Beskar7Cluster is deleted")
			Eventually(func() bool {
				lookupCluster := &infrastructurev1alpha1.Beskar7Cluster{}
				err := k8sClient.Get(ctx, key, lookupCluster)
				return client.IgnoreNotFound(err) == nil
			}, "10s", "200ms").Should(BeTrue(), "Beskar7Cluster should be deleted")
		})
	})

})
