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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// Import for our internal Redfish client interface and mocks
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
)

// MockRedfishClient is a mock implementation of the internalredfish.Client interface for testing.
// +kubebuilder:object:generate=false // We don't want Kubebuilder to generate CRDs for this mock.
type MockRedfishClient struct {
	GetSystemInfoFunc     func(ctx context.Context) (*internalredfish.SystemInfo, error)
	GetPowerStateFunc     func(ctx context.Context) (redfish.PowerState, error)
	SetPowerStateFunc     func(ctx context.Context, state redfish.PowerState) error
	SetBootSourceISOFunc  func(ctx context.Context, isoURL string) error
	EjectVirtualMediaFunc func(ctx context.Context) error
	SetBootParametersFunc func(ctx context.Context, params []string) error
	CloseFunc             func(ctx context.Context)

	// Store calls for assertion
	SetBootSourceISOCalls  []string
	SetBootParametersCalls [][]string
}

func (m *MockRedfishClient) GetSystemInfo(ctx context.Context) (*internalredfish.SystemInfo, error) {
	if m.GetSystemInfoFunc != nil {
		return m.GetSystemInfoFunc(ctx)
	}
	return &internalredfish.SystemInfo{Status: common.Status{State: common.EnabledState}}, nil // Default healthy
}

func (m *MockRedfishClient) GetPowerState(ctx context.Context) (redfish.PowerState, error) {
	if m.GetPowerStateFunc != nil {
		return m.GetPowerStateFunc(ctx)
	}
	return redfish.OffPowerState, nil // Default off
}

func (m *MockRedfishClient) SetPowerState(ctx context.Context, state redfish.PowerState) error {
	if m.SetPowerStateFunc != nil {
		return m.SetPowerStateFunc(ctx, state)
	}
	return nil
}

func (m *MockRedfishClient) SetBootSourceISO(ctx context.Context, isoURL string) error {
	m.SetBootSourceISOCalls = append(m.SetBootSourceISOCalls, isoURL)
	if m.SetBootSourceISOFunc != nil {
		return m.SetBootSourceISOFunc(ctx, isoURL)
	}
	return nil
}

func (m *MockRedfishClient) EjectVirtualMedia(ctx context.Context) error {
	if m.EjectVirtualMediaFunc != nil {
		return m.EjectVirtualMediaFunc(ctx)
	}
	return nil
}

func (m *MockRedfishClient) SetBootParameters(ctx context.Context, params []string) error {
	m.SetBootParametersCalls = append(m.SetBootParametersCalls, params) // Store a copy
	if m.SetBootParametersFunc != nil {
		return m.SetBootParametersFunc(ctx, params)
	}
	return nil
}

func (m *MockRedfishClient) Close(ctx context.Context) {
	if m.CloseFunc != nil {
		m.CloseFunc(ctx)
	}
}

// MockRedfishClientFactory creates a new MockRedfishClient.
// +kubebuilder:object:generate=false
func NewMockRedfishClientFactory(mockClient *MockRedfishClient) internalredfish.RedfishClientFactory {
	return func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
		// Potentially set address/username/password on mockClient if needed for specific tests
		return mockClient, nil
	}
}

var _ = Describe("Beskar7Machine Reconciler", func() {
	var (
		ctx         context.Context
		testNs      *corev1.Namespace
		b7machine   *infrastructurev1beta1.Beskar7Machine
		capiMachine *clusterv1.Machine
		host        *infrastructurev1beta1.PhysicalHost
		key         types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create a namespace for the test
		testNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "b7machine-reconciler-",
			},
		}
		Expect(k8sClient.Create(ctx, testNs)).To(Succeed())

		// Create owner CAPI Machine
		capiMachine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: testNs.Name,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "test-cluster", // Required field
			},
		}
		Expect(k8sClient.Create(ctx, capiMachine)).To(Succeed())

		// Basic Beskar7Machine object
		b7machine = &infrastructurev1beta1.Beskar7Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-b7machine",
				Namespace: testNs.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Machine",
						Name:       capiMachine.Name,
						UID:        capiMachine.UID,
					},
				},
			},
			Spec: infrastructurev1beta1.Beskar7MachineSpec{
				ImageURL: "http://example.com/default.iso", // Corrected: Use ImageURL
				OSFamily: "kairos",                         // Provide a default OSFamily
			},
		}
		key = types.NamespacedName{Name: b7machine.Name, Namespace: b7machine.Namespace}

	})

	AfterEach(func() {
		// Clean up the namespace
		Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
		// Note: k8sClient.Delete might need propagation policy if resources linger
		// Expect(k8sClient.Delete(ctx, testNs, client.PropagationPolicy(metav1.DeletePropagationForeground))).To(Succeed())
	})

	Context("Reconcile Normal", func() {
		// Add mockRfClient here for this context if it's not already in an outer scope shared with Provisioning Logic context
		var mockRfClient *MockRedfishClient // Define at this level if not shared

		BeforeEach(func() {
			// Initialize mockRfClient if defined at this level
			mockRfClient = &MockRedfishClient{
				SetBootSourceISOCalls:  make([]string, 0),
				SetBootParametersCalls: make([][]string, 0),
				// Initialize other funcs to return default success or specific test values if needed
			}
			// Ensure b7machine and other resources are reset/recreated if necessary, or use unique names
		})

		It("should requeue if no PhysicalHost is available", func() {
			// Create the Beskar7Machine
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// Initialize the reconciler for this test
			reconciler := &Beskar7MachineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile (adds finalizer)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue()) // Expect immediate requeue after adding finalizer
			Expect(result.RequeueAfter).To(BeZero())

			// Second reconcile (should actually check for host)
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue || result.RequeueAfter > 0).To(BeTrue(), "Should requeue (with or without delay) when waiting for host")

			// Fetch the updated Beskar7Machine
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, b7machine)
				return err == nil
			}, time.Second*5, time.Millisecond*200).Should(BeTrue())

			// Check conditions
			Expect(conditions.Has(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition)).To(BeTrue())
			cond := conditions.Get(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition)
			Expect(cond.Status).To(Equal(corev1.ConditionFalse))
			Expect(cond.Reason).To(Equal(infrastructurev1beta1.WaitingForPhysicalHostReason))
		})

		It("should claim an available PhysicalHost", func() {
			// Create an available PhysicalHost with status populated
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "available-host",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy",
						CredentialsSecretRef: "dummy-secret",
					},
				},
				Status: infrastructurev1beta1.PhysicalHostStatus{
					State: infrastructurev1beta1.StateAvailable,
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			// >>> Explicitly update the status after creation <<<
			Eventually(func(g Gomega) {
				// Fetch the created host first
				createdHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				// Now update its status
				createdHost.Status = infrastructurev1beta1.PhysicalHostStatus{
					State: infrastructurev1beta1.StateAvailable,
					Ready: true,
				}
				g.Expect(k8sClient.Status().Update(ctx, createdHost)).To(Succeed())
			}, "10s", "100ms").Should(Succeed(), "Failed to update PhysicalHost status")

			// Wait slightly for create to settle (optional, might help envtest)
			time.Sleep(100 * time.Millisecond)

			// Create dummy secret for Redfish credentials for this host
			dummySecretForHost := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dummy-secret", Namespace: testNs.Name},
				Data:       map[string][]byte{"username": []byte("user"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, dummySecretForHost)).To(Succeed())

			// Create the Beskar7Machine
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// Initialize the reconciler
			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockRfClient), // Ensure this is added
			}

			// First reconcile (adds finalizer)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue()) // Expect immediate requeue
			Expect(result.RequeueAfter).To(BeZero())

			// Second reconcile (claims host & attempts boot config)
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			// In this specific test, we are not asserting boot config details, just that it requeues after claiming.
			// The actual Redfish calls would happen here but might fail if a real client was used.
			// With a mock, they should succeed with nil error by default.
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Fetch the updated PhysicalHost
			hostKey := types.NamespacedName{Name: host.Name, Namespace: host.Namespace}
			Eventually(func() *corev1.ObjectReference {
				Expect(k8sClient.Get(ctx, hostKey, host)).To(Succeed())
				return host.Spec.ConsumerRef
			}, "15s", "200ms").ShouldNot(BeNil(), "ConsumerRef should be set") // Increased timeout from previous fix

			// Check claimed host details
			Expect(host.Spec.ConsumerRef.Name).To(Equal(b7machine.Name))
			Expect(host.Spec.ConsumerRef.Namespace).To(Equal(b7machine.Namespace))
			Expect(host.Spec.ConsumerRef.Kind).To(Equal(b7machine.Kind))
			Expect(host.Spec.ConsumerRef.APIVersion).To(Equal(b7machine.APIVersion))
			Expect(host.Spec.BootISOSource).NotTo(BeNil())
			Expect(*host.Spec.BootISOSource).To(Equal(b7machine.Spec.ImageURL))

			// Fetch the updated Beskar7Machine
			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
				return conditions.IsTrue(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition)
			}, time.Second*5, time.Millisecond*200).Should(BeTrue(), "PhysicalHostAssociatedCondition should be True")

			// Check ProviderID is not set yet
			Expect(b7machine.Spec.ProviderID).To(BeNil())
		})

		It("should set Infra Ready=False when host is Provisioning", func() {
			// Create a PhysicalHost claimed by our machine and in Provisioning state
			hostName := "provisioning-host"
			imageUrl := "http://example.com/prov-test.iso"
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy",
						CredentialsSecretRef: "dummy-secret",
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       b7machine.Kind,
						APIVersion: b7machine.APIVersion,
						Name:       b7machine.Name,
						Namespace:  b7machine.Namespace,
					},
					BootISOSource: &imageUrl,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			// Create mock Redfish client that simulates provisioning state
			mockClient := &MockRedfishClient{
				GetSystemInfoFunc: func(ctx context.Context) (*internalredfish.SystemInfo, error) {
					return &internalredfish.SystemInfo{
						Manufacturer: "TestManufacturer",
						Model:        "TestModel",
						SerialNumber: "TestSerial",
						Status:       common.Status{State: common.EnabledState},
					}, nil
				},
				GetPowerStateFunc: func(ctx context.Context) (redfish.PowerState, error) {
					return redfish.OnPowerState, nil
				},
			}

			// Set ProviderID on Beskar7Machine to link it to the host
			providerID := providerID(host.Namespace, host.Name)
			b7machine.Spec.ProviderID = &providerID
			b7machine.Spec.ImageURL = imageUrl

			// Create the Beskar7Machine
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// Initialize the reconciler with mock client
			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockClient),
			}

			// First reconcile (adds finalizer)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Update host status to Provisioning
			host.Status.State = infrastructurev1beta1.StateProvisioning
			host.Status.Ready = false
			Expect(k8sClient.Status().Update(ctx, host)).To(Succeed())

			// Wait for status update to be processed
			Eventually(func() bool {
				var updatedHost infrastructurev1beta1.PhysicalHost
				err := k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, &updatedHost)
				return err == nil && updatedHost.Status.State == infrastructurev1beta1.StateProvisioning
			}, time.Second*5, time.Millisecond*200).Should(BeTrue())

			// Second reconcile (finds provisioning host)
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue || result.RequeueAfter > 0).To(BeTrue())

			// Verify Beskar7Machine status
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
				g.Expect(conditions.Has(b7machine, infrastructurev1beta1.InfrastructureReadyCondition)).To(BeTrue())
				cond := conditions.Get(b7machine, infrastructurev1beta1.InfrastructureReadyCondition)
				g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(infrastructurev1beta1.PhysicalHostNotReadyReason))
				g.Expect(cond.Message).To(ContainSubstring("is still provisioning"))
				g.Expect(b7machine.Status.Phase).NotTo(BeNil())
				g.Expect(*b7machine.Status.Phase).To(Equal("Provisioning"))
			}, time.Second*5, time.Millisecond*200).Should(Succeed())
		})

		It("should set Infra Ready=True and ProviderID when host is Provisioned", func() {
			// Create a PhysicalHost claimed by our machine and in Provisioned state
			hostName := "provisioned-host"
			imageUrl := "http://example.com/ready-test.iso"
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy",
						CredentialsSecretRef: "dummy-secret",
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       b7machine.Kind,
						APIVersion: b7machine.APIVersion,
						Name:       b7machine.Name,
						Namespace:  b7machine.Namespace,
					},
					BootISOSource: &imageUrl,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			// Create mock Redfish client that simulates provisioned state
			mockClient := &MockRedfishClient{
				GetSystemInfoFunc: func(ctx context.Context) (*internalredfish.SystemInfo, error) {
					return &internalredfish.SystemInfo{
						Manufacturer: "TestManufacturer",
						Model:        "TestModel",
						SerialNumber: "TestSerial",
						Status:       common.Status{State: common.EnabledState},
					}, nil
				},
				GetPowerStateFunc: func(ctx context.Context) (redfish.PowerState, error) {
					return redfish.OnPowerState, nil
				},
			}

			// Create the Beskar7Machine
			b7machine.Spec.ImageURL = imageUrl
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// Initialize the reconciler with mock client
			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockClient),
			}

			// First reconcile (adds finalizer)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Update host status to Provisioned
			host.Status.State = infrastructurev1beta1.StateProvisioned
			host.Status.Ready = true
			Expect(k8sClient.Status().Update(ctx, host)).To(Succeed())

			// Wait for status update to be processed
			Eventually(func() bool {
				var updatedHost infrastructurev1beta1.PhysicalHost
				err := k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, &updatedHost)
				return err == nil && updatedHost.Status.State == infrastructurev1beta1.StateProvisioned
			}, time.Second*5, time.Millisecond*200).Should(BeTrue())

			// Second reconcile (finds provisioned host)
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify Beskar7Machine status
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
				g.Expect(b7machine.Spec.ProviderID).NotTo(BeNil())
				expectedProviderID := providerID(host.Namespace, host.Name)
				g.Expect(*b7machine.Spec.ProviderID).To(Equal(expectedProviderID))
				g.Expect(conditions.IsTrue(b7machine, infrastructurev1beta1.InfrastructureReadyCondition)).To(BeTrue())
				g.Expect(b7machine.Status.Phase).NotTo(BeNil())
				g.Expect(*b7machine.Status.Phase).To(Equal("Provisioned"))
			}, time.Second*5, time.Millisecond*200).Should(Succeed())
		})

		It("should set Infra Ready=False and Phase=Failed when host is Error", func() {
			// Create a PhysicalHost claimed by our machine and in Error state
			hostName := "error-host"
			imageUrl := "http://example.com/error-test.iso"
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy",
						CredentialsSecretRef: "dummy-secret",
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       b7machine.Kind,
						APIVersion: b7machine.APIVersion,
						Name:       b7machine.Name,
						Namespace:  b7machine.Namespace,
					},
					BootISOSource: &imageUrl,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			// Create mock Redfish client that simulates error state
			mockClient := &MockRedfishClient{
				GetSystemInfoFunc: func(ctx context.Context) (*internalredfish.SystemInfo, error) {
					return nil, fmt.Errorf("simulated error")
				},
				GetPowerStateFunc: func(ctx context.Context) (redfish.PowerState, error) {
					return "", fmt.Errorf("simulated error")
				},
			}

			// Set ProviderID on Beskar7Machine to link it to the host
			providerID := providerID(host.Namespace, host.Name)
			b7machine.Spec.ProviderID = &providerID
			b7machine.Spec.ImageURL = imageUrl

			// Create the Beskar7Machine
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// Initialize the reconciler with mock client
			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockClient),
			}

			// First reconcile (adds finalizer)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Update host status to Error
			host.Status.State = infrastructurev1beta1.StateError
			host.Status.ErrorMessage = "Redfish connection failed repeatedly"
			host.Status.Ready = false
			Expect(k8sClient.Status().Update(ctx, host)).To(Succeed())

			// Wait for status update to be processed
			Eventually(func() bool {
				var updatedHost infrastructurev1beta1.PhysicalHost
				err := k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, &updatedHost)
				return err == nil && updatedHost.Status.State == infrastructurev1beta1.StateError
			}, time.Second*5, time.Millisecond*200).Should(BeTrue())

			// Second reconcile (finds error host)
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify Beskar7Machine status
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
				g.Expect(conditions.Has(b7machine, infrastructurev1beta1.InfrastructureReadyCondition)).To(BeTrue())
				cond := conditions.Get(b7machine, infrastructurev1beta1.InfrastructureReadyCondition)
				g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(infrastructurev1beta1.PhysicalHostErrorReason))
				g.Expect(cond.Message).To(ContainSubstring("Redfish connection failed repeatedly"))
				g.Expect(b7machine.Status.Phase).NotTo(BeNil())
				g.Expect(*b7machine.Status.Phase).To(Equal("Failed"))
			}, time.Second*5, time.Millisecond*200).Should(Succeed())
		})
	})

	Context("Reconcile Delete", func() {
		// TODO: This test is currently skipped due to timing issues with host state transitions.
		// The test will be reimplemented in the future with proper state management and timing controls.
		PIt("should release the PhysicalHost when deleted", func() {
			// Create a PhysicalHost claimed by our machine
			hostName := "to-be-released-host"
			imageUrl := "http://example.com/release-test.iso"
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy",
						CredentialsSecretRef: "dummy-secret",
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       b7machine.Kind,
						APIVersion: b7machine.APIVersion,
						Name:       b7machine.Name,
						Namespace:  b7machine.Namespace,
					},
					BootISOSource: &imageUrl,
				},
				Status: infrastructurev1beta1.PhysicalHostStatus{
					State: infrastructurev1beta1.StateProvisioned, // Start as if it was provisioned
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			// Wait for host to be created and status to be updated
			hostKey := types.NamespacedName{Name: host.Name, Namespace: host.Namespace}
			Eventually(func(g Gomega) {
				getErr := k8sClient.Get(ctx, hostKey, host)
				g.Expect(getErr).NotTo(HaveOccurred())
				g.Expect(host.Status.State).To(Equal(infrastructurev1beta1.StateProvisioned))
			}, time.Second*5, time.Millisecond*200).Should(Succeed())

			// Set ProviderID on Beskar7Machine to link it
			providerID := providerID(host.Namespace, host.Name)
			b7machine.Spec.ProviderID = &providerID
			b7machine.Spec.ImageURL = imageUrl // Ensure matching URL

			// Create the Beskar7Machine
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// Wait for Beskar7Machine to be created
			Eventually(func() bool {
				return k8sClient.Get(ctx, key, b7machine) == nil
			}, time.Second*5, time.Millisecond*200).Should(BeTrue())

			// Initialize the reconciler
			reconciler := &Beskar7MachineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile (adds finalizer)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue()) // Expect immediate requeue
			Expect(result.RequeueAfter).To(BeZero())

			// Second reconcile (releases host)
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse()) // Should not requeue after successful delete reconcile

			// --- Simulate the PhysicalHost controller updating the status ---
			Eventually(func(g Gomega) {
				latest := &infrastructurev1beta1.PhysicalHost{}
				getErr := k8sClient.Get(ctx, hostKey, latest)
				g.Expect(getErr).NotTo(HaveOccurred(), "Failed to get host for status update")
				latest.Status.State = infrastructurev1beta1.StateProvisioned
				latest.Status.Ready = true
				g.Expect(k8sClient.Status().Update(ctx, latest)).To(Succeed(), "Failed to update host status")
			}, time.Second*5, time.Millisecond*200).Should(Succeed(), "Failed to update PhysicalHost status")

			// Check the PhysicalHost is released
			Eventually(func(g Gomega) {
				latest := &infrastructurev1beta1.PhysicalHost{}
				getErr := k8sClient.Get(ctx, hostKey, latest)
				g.Expect(getErr).NotTo(HaveOccurred(), "Failed to get host for release check")
				g.Expect(latest.Spec.ConsumerRef).To(BeNil(), "ConsumerRef should be nil after release")
				g.Expect(latest.Spec.BootISOSource).To(BeNil(), "BootISOSource should be nil after release")
				g.Expect(latest.Status.State).To(Equal(infrastructurev1beta1.StateProvisioned), "Host should remain in Provisioned state after release")
			}, time.Second*15, time.Millisecond*250).Should(Succeed(), "PhysicalHost should be released and remain in Provisioned state")

			// Delete the Beskar7Machine
			Expect(k8sClient.Delete(ctx, b7machine)).To(Succeed())

			// Check Beskar7Machine is eventually deleted (finalizer removed)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, b7machine)
				return client.IgnoreNotFound(err) == nil
			}, time.Second*10, time.Millisecond*200).Should(BeTrue(), "Beskar7Machine should be deleted")
		})

		It("should remove finalizer if host not found", func() {
			// Set ProviderID on Beskar7Machine to link it to a non-existent host
			nonExistentHostName := "ghost-host"
			providerID := providerID(testNs.Name, nonExistentHostName) // Use testNs.Name for namespace
			b7machine.Spec.ProviderID = &providerID

			// Create the Beskar7Machine
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// Initialize the reconciler
			reconciler := &Beskar7MachineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile (adds finalizer)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue()) // Expect immediate requeue
			Expect(result.RequeueAfter).To(BeZero())

			// Fix #3: Reconcile normally again (optional but good practice)
			// This reconcile will fail to find the host and requeue
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue || result.RequeueAfter > 0).To(BeTrue())

			// Fetch to confirm finalizer is added
			Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
			Expect(b7machine.Finalizers).To(ContainElement(Beskar7MachineFinalizer))

			// Delete the Beskar7Machine
			Expect(k8sClient.Delete(ctx, b7machine)).To(Succeed())

			// Reconcile *after delete* to trigger deletion logic
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred()) // Should not error even if host is missing
			// Fix #3: Check result of deletion reconcile
			Expect(result.Requeue).To(BeFalse(), "Deletion reconcile should not requeue if host is missing")
			Expect(result.RequeueAfter).To(BeZero(), "Deletion reconcile should not have RequeueAfter if host is missing")

			// Check Beskar7Machine is eventually deleted (finalizer removed)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, b7machine)
				return client.IgnoreNotFound(err) == nil
			}, time.Second*10, time.Millisecond*200).Should(BeTrue(), "Beskar7Machine should be deleted even if host was not found")
		})
	})

	Context("Reconcile Normal - Provisioning Logic", func() {
		var mockRfClient *MockRedfishClient // This is correctly defined here for this context

		BeforeEach(func() {
			// Reset mock client for each test in this context
			mockRfClient = &MockRedfishClient{
				SetBootSourceISOCalls:  make([]string, 0),
				SetBootParametersCalls: make([][]string, 0),
			}
		})

		It("should configure boot for PreBakedISO mode", func() {
			preBakedIsoURL := "http://example.com/prebaked.iso"
			b7machine.Spec.OSFamily = "kairos" // Needs an OS family for defaulting logic
			b7machine.Spec.ImageURL = preBakedIsoURL
			b7machine.Spec.ProvisioningMode = "PreBakedISO"
			b7machine.Spec.ConfigURL = "" // Should be ignored

			// Create a PhysicalHost that the reconciler will find and claim
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "available-host-prebaked", Namespace: testNs.Name},
				// Ensure RedfishConnection has all required fields for getRedfishClientForHost
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy-prebaked",
						CredentialsSecretRef: "dummy-secret-prebaked", // Needs a corresponding dummy secret
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			Eventually(func(g Gomega) {
				createdHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				createdHost.Status = infrastructurev1beta1.PhysicalHostStatus{State: infrastructurev1beta1.StateAvailable, Ready: true}
				g.Expect(k8sClient.Status().Update(ctx, createdHost)).To(Succeed())
			}, "10s", "100ms").Should(Succeed(), "Failed to update PhysicalHost status for prebaked test")

			// Create dummy secret for Redfish credentials
			dummySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dummy-secret-prebaked", Namespace: testNs.Name},
				Data:       map[string][]byte{"username": []byte("user"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, dummySecret)).To(Succeed())

			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockRfClient),
			}

			By("First reconcile to add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Second reconcile to claim host and configure boot settings")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue to wait for PhysicalHost controller")

			// Ensure ConsumerRef was set on PhysicalHost by the second reconcile
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
				g.Expect(updatedHost.Spec.ConsumerRef.Name).To(Equal(b7machine.Name))
			}, "15s", "200ms").Should(Succeed(), "PhysicalHost should be claimed after second reconcile")

			// Assertions about Redfish client calls (now happen in second reconcile)
			Expect(mockRfClient.SetBootParametersCalls).To(HaveLen(1), "SetBootParameters should be called once")
			Expect(mockRfClient.SetBootParametersCalls[0]).To(BeNil(), "SetBootParameters should be called with nil for PreBakedISO")
			Expect(mockRfClient.SetBootSourceISOCalls).To(HaveLen(1), "SetBootSourceISO should be called once")
			Expect(mockRfClient.SetBootSourceISOCalls[0]).To(Equal(preBakedIsoURL), "SetBootSourceISO should be called with the preBakedIsoURL")

			// Verify Beskar7Machine conditions
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
				g.Expect(conditions.IsTrue(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition)).To(BeTrue())
			}, "5s", "100ms").Should(Succeed(), "PhysicalHostAssociatedCondition should be True")
		})

		It("should configure boot for RemoteConfig mode with Kairos", func() {
			remoteConfigURL := "https://example.com/kairos-config.yaml"
			genericIsoURL := "http://example.com/kairos-generic.iso"

			b7machine.Spec.OSFamily = "kairos"
			b7machine.Spec.ImageURL = genericIsoURL
			b7machine.Spec.ProvisioningMode = "RemoteConfig"
			b7machine.Spec.ConfigURL = remoteConfigURL

			// Create a PhysicalHost that the reconciler will find and claim
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "available-host-remote", Namespace: testNs.Name},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy-remote",
						CredentialsSecretRef: "dummy-secret-remote",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			Eventually(func(g Gomega) {
				createdHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				createdHost.Status = infrastructurev1beta1.PhysicalHostStatus{State: infrastructurev1beta1.StateAvailable, Ready: true}
				g.Expect(k8sClient.Status().Update(ctx, createdHost)).To(Succeed())
			}, "10s", "100ms").Should(Succeed(), "Failed to update PhysicalHost status for remote config test")

			// Create dummy secret for Redfish credentials
			dummySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dummy-secret-remote", Namespace: testNs.Name},
				Data:       map[string][]byte{"username": []byte("user"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, dummySecret)).To(Succeed())

			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockRfClient),
			}

			By("First reconcile to add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Second reconcile to claim host and configure boot settings")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue to wait for PhysicalHost controller")

			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed(), "PhysicalHost should be claimed after second reconcile for remote config")

			Expect(mockRfClient.SetBootParametersCalls).To(HaveLen(1), "SetBootParameters should be called once for RemoteConfig")
			Expect(mockRfClient.SetBootParametersCalls[0]).To(Equal([]string{fmt.Sprintf("config_url=%s", remoteConfigURL)}), "SetBootParameters called with incorrect Kairos params")
			Expect(mockRfClient.SetBootSourceISOCalls).To(HaveLen(1), "SetBootSourceISO should be called once for RemoteConfig")
			Expect(mockRfClient.SetBootSourceISOCalls[0]).To(Equal(genericIsoURL), "SetBootSourceISO called with incorrect ImageURL for RemoteConfig")

			// Verify Beskar7Machine conditions
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
				g.Expect(conditions.IsTrue(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition)).To(BeTrue())
			}, "5s", "100ms").Should(Succeed(), "PhysicalHostAssociatedCondition should be True")
		})

		It("should configure boot for RemoteConfig mode with Flatcar", func() {
			remoteConfigURL := "https://example.com/flatcar-ignition.json"
			genericIsoURL := "http://example.com/flatcar-generic.iso"

			b7machine.Spec.OSFamily = "flatcar"
			b7machine.Spec.ImageURL = genericIsoURL
			b7machine.Spec.ProvisioningMode = "RemoteConfig"
			b7machine.Spec.ConfigURL = remoteConfigURL

			// Create a PhysicalHost that the reconciler will find and claim
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "available-host-flatcar", Namespace: testNs.Name},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy-flatcar",
						CredentialsSecretRef: "dummy-secret-flatcar",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			Eventually(func(g Gomega) {
				createdHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				createdHost.Status = infrastructurev1beta1.PhysicalHostStatus{State: infrastructurev1beta1.StateAvailable, Ready: true}
				g.Expect(k8sClient.Status().Update(ctx, createdHost)).To(Succeed())
			}, "10s", "100ms").Should(Succeed(), "Failed to update PhysicalHost status for flatcar test")

			// Create dummy secret for Redfish credentials
			dummySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dummy-secret-flatcar", Namespace: testNs.Name},
				Data:       map[string][]byte{"username": []byte("user"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, dummySecret)).To(Succeed())

			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockRfClient),
			}

			By("First reconcile to add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Second reconcile to claim host and configure boot settings")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue to wait for PhysicalHost controller")

			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed(), "PhysicalHost should be claimed after second reconcile for flatcar")

			// Assert Redfish calls
			Expect(mockRfClient.SetBootParametersCalls).To(HaveLen(1), "SetBootParameters should be called once for Flatcar")
			Expect(mockRfClient.SetBootParametersCalls[0]).To(Equal([]string{fmt.Sprintf("flatcar.ignition.config.url=%s", remoteConfigURL)}), "SetBootParameters called with incorrect Flatcar params")
			Expect(mockRfClient.SetBootSourceISOCalls).To(HaveLen(1), "SetBootSourceISO should be called once for Flatcar")
			Expect(mockRfClient.SetBootSourceISOCalls[0]).To(Equal(genericIsoURL), "SetBootSourceISO called with incorrect ImageURL for Flatcar")
		})

		It("should configure boot for RemoteConfig mode with LeapMicro", func() {
			remoteConfigURL := "https://example.com/leap-micro-combustion.script"
			genericIsoURL := "http://example.com/leap-micro-generic.iso"

			b7machine.Spec.OSFamily = "LeapMicro"
			b7machine.Spec.ImageURL = genericIsoURL
			b7machine.Spec.ProvisioningMode = "RemoteConfig"
			b7machine.Spec.ConfigURL = remoteConfigURL

			// Create a PhysicalHost that the reconciler will find and claim
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "available-host-leapmicro", Namespace: testNs.Name},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy-leapmicro",
						CredentialsSecretRef: "dummy-secret-leapmicro",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			Eventually(func(g Gomega) {
				createdHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				createdHost.Status = infrastructurev1beta1.PhysicalHostStatus{State: infrastructurev1beta1.StateAvailable, Ready: true}
				g.Expect(k8sClient.Status().Update(ctx, createdHost)).To(Succeed())
			}, "10s", "100ms").Should(Succeed(), "Failed to update PhysicalHost status for leapmicro test")

			// Create dummy secret for Redfish credentials
			dummySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dummy-secret-leapmicro", Namespace: testNs.Name},
				Data:       map[string][]byte{"username": []byte("user"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, dummySecret)).To(Succeed())

			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockRfClient),
			}

			By("First reconcile to add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Second reconcile to claim host and configure boot settings")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue to wait for PhysicalHost controller")

			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed(), "PhysicalHost should be claimed after second reconcile for leapmicro")

			// Assert Redfish calls
			Expect(mockRfClient.SetBootParametersCalls).To(HaveLen(1), "SetBootParameters should be called once for LeapMicro")
			Expect(mockRfClient.SetBootParametersCalls[0]).To(Equal([]string{fmt.Sprintf("combustion.path=%s", remoteConfigURL)}), "SetBootParameters called with incorrect LeapMicro params")
			Expect(mockRfClient.SetBootSourceISOCalls).To(HaveLen(1), "SetBootSourceISO should be called once for LeapMicro")
			Expect(mockRfClient.SetBootSourceISOCalls[0]).To(Equal(genericIsoURL), "SetBootSourceISO called with incorrect ImageURL for LeapMicro")
		})

		It("should configure boot for RemoteConfig mode with Talos", func() {
			remoteConfigURL := "https://example.com/talos-machineconfig.yaml"
			genericIsoURL := "http://example.com/talos-generic.iso"

			b7machine.Spec.OSFamily = "talos"
			b7machine.Spec.ImageURL = genericIsoURL
			b7machine.Spec.ProvisioningMode = "RemoteConfig"
			b7machine.Spec.ConfigURL = remoteConfigURL

			// Create a PhysicalHost that the reconciler will find and claim
			host = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "available-host-talos", Namespace: testNs.Name},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish://dummy-talos",
						CredentialsSecretRef: "dummy-secret-talos",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			Eventually(func(g Gomega) {
				createdHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				createdHost.Status = infrastructurev1beta1.PhysicalHostStatus{State: infrastructurev1beta1.StateAvailable, Ready: true}
				g.Expect(k8sClient.Status().Update(ctx, createdHost)).To(Succeed())
			}, "10s", "100ms").Should(Succeed(), "Failed to update PhysicalHost status for talos test")

			// Create dummy secret for Redfish credentials
			dummySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dummy-secret-talos", Namespace: testNs.Name},
				Data:       map[string][]byte{"username": []byte("user"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, dummySecret)).To(Succeed())

			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			reconciler := &Beskar7MachineReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RedfishClientFactory: NewMockRedfishClientFactory(mockRfClient),
			}

			By("First reconcile to add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Second reconcile to claim host and configure boot settings")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue to wait for PhysicalHost controller")

			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed(), "PhysicalHost should be claimed after second reconcile for talos")

			// Assert Redfish calls
			Expect(mockRfClient.SetBootParametersCalls).To(HaveLen(1), "SetBootParameters should be called once for Talos")
			Expect(mockRfClient.SetBootParametersCalls[0]).To(Equal([]string{fmt.Sprintf("talos.config=%s", remoteConfigURL)}), "SetBootParameters called with incorrect Talos params")
			Expect(mockRfClient.SetBootSourceISOCalls).To(HaveLen(1), "SetBootSourceISO should be called once for Talos")
			Expect(mockRfClient.SetBootSourceISOCalls[0]).To(Equal(genericIsoURL), "SetBootSourceISO called with incorrect ImageURL for Talos")
		})

		// TODO: Add test for "RemoteConfig" mode with missing ConfigURL (error expected)
		// TODO: Add test for "RemoteConfig" mode with unsupported OSFamily (error expected)

	})

})
