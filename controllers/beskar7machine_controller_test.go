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
	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
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
	SetBootSourceISOCalls   []string
	SetBootParametersCalls  [][]string
	SetBootSourceCalled     bool
	SetBootParametersCalled bool
	InsertedISO             string
	StoredBootParams        []string
	ShouldFail              map[string]error
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
	if m.SetBootSourceISOFunc != nil {
		return m.SetBootSourceISOFunc(ctx, isoURL)
	}
	m.SetBootSourceCalled = true
	m.InsertedISO = isoURL
	m.SetBootSourceISOCalls = append(m.SetBootSourceISOCalls, isoURL)
	return nil
}

func (m *MockRedfishClient) EjectVirtualMedia(ctx context.Context) error {
	if m.EjectVirtualMediaFunc != nil {
		return m.EjectVirtualMediaFunc(ctx)
	}
	return nil
}

func (m *MockRedfishClient) SetBootParameters(ctx context.Context, params []string) error {
	if m.SetBootParametersFunc != nil {
		return m.SetBootParametersFunc(ctx, params)
	}
	m.SetBootParametersCalled = true
	m.StoredBootParams = params
	m.SetBootParametersCalls = append(m.SetBootParametersCalls, params)
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
		ctx          context.Context
		testNs       *corev1.Namespace
		b7machine    *infrastructurev1alpha1.Beskar7Machine
		capiMachine  *clusterv1.Machine
		host         *infrastructurev1alpha1.PhysicalHost
		key          types.NamespacedName
		reconciler   *Beskar7MachineReconciler
		mockRfClient *MockRedfishClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create a unique namespace for the test
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
				ClusterName: "test-cluster",
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: pointer.String("test-bootstrap-secret"),
				},
			},
		}
		Expect(k8sClient.Create(ctx, capiMachine)).To(Succeed())

		// Basic Beskar7Machine object with valid OSFamily
		b7machine = &infrastructurev1alpha1.Beskar7Machine{
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
			Spec: infrastructurev1alpha1.Beskar7MachineSpec{
				ImageURL: "http://example.com/default.iso",
				OSFamily: "kairos", // Ensure valid OSFamily
			},
		}
		key = types.NamespacedName{Name: b7machine.Name, Namespace: b7machine.Namespace}

		// Create dummy secret for Redfish credentials with a unique name
		secretName := fmt.Sprintf("dummy-secret-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
		dummySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNs.Name,
			},
			Data: map[string][]byte{
				"username": []byte("user"),
				"password": []byte("pass"),
			},
		}
		Expect(k8sClient.Create(ctx, dummySecret)).To(Succeed())

		// Initialize mock Redfish client with proper defaults
		mockRfClient = &MockRedfishClient{
			GetSystemInfoFunc: func(ctx context.Context) (*internalredfish.SystemInfo, error) {
				return &internalredfish.SystemInfo{
					Status: common.Status{State: common.EnabledState},
				}, nil
			},
			GetPowerStateFunc: func(ctx context.Context) (redfish.PowerState, error) {
				return redfish.OffPowerState, nil
			},
			SetPowerStateFunc: func(ctx context.Context, state redfish.PowerState) error {
				return nil
			},
			SetBootSourceISOFunc: func(ctx context.Context, isoURL string) error {
				mockRfClient.SetBootSourceCalled = true
				mockRfClient.InsertedISO = isoURL
				mockRfClient.SetBootSourceISOCalls = append(mockRfClient.SetBootSourceISOCalls, isoURL)
				return nil
			},
			SetBootParametersFunc: func(ctx context.Context, params []string) error {
				mockRfClient.SetBootParametersCalled = true
				mockRfClient.StoredBootParams = params
				mockRfClient.SetBootParametersCalls = append(mockRfClient.SetBootParametersCalls, params)
				return nil
			},
		}

		// Initialize reconciler with mock client factory
		reconciler = &Beskar7MachineReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
			RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
				return mockRfClient, nil
			},
		}
	})

	AfterEach(func() {
		// Clean up the namespace and wait for deletion
		Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
		// Wait for namespace to be fully deleted
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: testNs.Name}, &corev1.Namespace{})
		}, "10s", "100ms").ShouldNot(Succeed())
	})

	Context("Reconcile Normal", func() {
		It("should requeue if no PhysicalHost is available", func() {
			By("Creating a Beskar7Machine without any available PhysicalHost")
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// First reconcile should add finalizer
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Second reconcile should requeue due to no available host
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			// Check that the machine status reflects waiting for host
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      b7machine.Name,
					Namespace: b7machine.Namespace,
				}, updatedMachine)).To(Succeed())
				g.Expect(conditions.IsFalse(updatedMachine, infrastructurev1alpha1.PhysicalHostAssociatedCondition)).To(BeTrue())
				g.Expect(conditions.GetReason(updatedMachine, infrastructurev1alpha1.PhysicalHostAssociatedCondition)).To(Equal(infrastructurev1alpha1.WaitingForPhysicalHostReason))
			}, "30s", "100ms").Should(Succeed())
		})

		It("should claim an available PhysicalHost", func() {
			By("Creating an available PhysicalHost")
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "redfish://dummy",
						CredentialsSecretRef: "dummy-secret",
					},
				},
				Status: infrastructurev1alpha1.PhysicalHostStatus{
					State: infrastructurev1alpha1.StateAvailable,
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			By("Creating a Beskar7Machine")
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// First reconcile should add finalizer
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Second reconcile should find and claim the host
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify the host was claimed
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      host.Name,
					Namespace: host.Namespace,
				}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
				g.Expect(updatedHost.Spec.ConsumerRef.Name).To(Equal(b7machine.Name))
				g.Expect(updatedHost.Spec.ConsumerRef.Namespace).To(Equal(b7machine.Namespace))
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1alpha1.StateClaimed))
			}, "30s", "100ms").Should(Succeed())

			// Verify the machine status
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      b7machine.Name,
					Namespace: b7machine.Namespace,
				}, updatedMachine)).To(Succeed())
				g.Expect(conditions.IsTrue(updatedMachine, infrastructurev1alpha1.PhysicalHostAssociatedCondition)).To(BeTrue())
				g.Expect(updatedMachine.Spec.ProviderID).NotTo(BeNil())
				expectedProviderID := providerID(host.Namespace, host.Name)
				g.Expect(*updatedMachine.Spec.ProviderID).To(Equal(expectedProviderID))
			}, "30s", "100ms").Should(Succeed())
		})

		It("should set Infra Ready=False when host is Provisioning", func() {
			Skip("Skipping due to envtest status/update reliability issues")
			// Create a PhysicalHost claimed by our machine and in Provisioning state (with status)
			hostName := "provisioning-host"
			imageUrl := "http://example.com/prov-test.iso" // Ensure unique URL if needed
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
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
				Status: infrastructurev1alpha1.PhysicalHostStatus{
					State: infrastructurev1alpha1.StateProvisioning,
					Ready: false,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			// Removed Status().Update()

			// Wait slightly for create to settle (optional, might help envtest)
			time.Sleep(100 * time.Millisecond)

			// Set ProviderID on Beskar7Machine to link it to the host
			providerID := providerID(host.Namespace, host.Name)
			b7machine.Spec.ProviderID = &providerID
			b7machine.Spec.ImageURL = imageUrl // Match host spec

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

			// Second reconcile (finds provisioning host)
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue || result.RequeueAfter > 0).To(BeTrue(), "Should requeue (with or without delay) when host is provisioning")

			// Fetch the updated Beskar7Machine
			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
				return conditions.Has(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition)
			}, time.Second*5, time.Millisecond*200).Should(BeTrue(), "InfrastructureReadyCondition should be set")

			// Check conditions and phase
			cond := conditions.Get(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition)
			Expect(cond.Status).To(Equal(corev1.ConditionFalse))
			Expect(cond.Reason).To(Equal(infrastructurev1alpha1.PhysicalHostNotReadyReason))
			Expect(cond.Message).To(ContainSubstring("is still provisioning"))
			Expect(b7machine.Status.Phase).NotTo(BeNil())
			Expect(*b7machine.Status.Phase).To(Equal("Provisioning"))
		})

		It("should set Infra Ready=True and ProviderID when host is Provisioned", func() {
			Skip("Skipping due to envtest status/update reliability issues")
			// Create a PhysicalHost claimed by our machine and in Provisioned state (with status)
			hostName := "provisioned-host"
			imageUrl := "http://example.com/ready-test.iso"
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
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
				Status: infrastructurev1alpha1.PhysicalHostStatus{
					State: infrastructurev1alpha1.StateProvisioned,
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			// Removed Status().Update()

			// Wait slightly for create to settle (optional, might help envtest)
			time.Sleep(100 * time.Millisecond)

			// Create the Beskar7Machine (ProviderID should be set by reconcile)
			b7machine.Spec.ImageURL = imageUrl // Match host spec
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// Initialize the reconciler
			reconciler := &Beskar7MachineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile (adds finalizer)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse()) // Should not requeue once provisioned
			Expect(result.RequeueAfter).To(BeZero())

			// Second reconcile (finds provisioned host)
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse()) // Should not requeue once provisioned
			Expect(result.RequeueAfter).To(BeZero())

			// Fetch the updated Beskar7Machine
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, key, b7machine)).To(Succeed())
				// Check ProviderID is set
				g.Expect(b7machine.Spec.ProviderID).NotTo(BeNil())
				expectedProviderID := providerID(host.Namespace, host.Name)
				g.Expect(*b7machine.Spec.ProviderID).To(Equal(expectedProviderID))
				// Check Condition
				g.Expect(conditions.IsTrue(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition)).To(BeTrue())
				// Check Phase
				g.Expect(b7machine.Status.Phase).NotTo(BeNil())
				g.Expect(*b7machine.Status.Phase).To(Equal("Provisioned"))
			}, time.Second*5, time.Millisecond*200).Should(Succeed(), "Beskar7Machine should be Ready with ProviderID")
		})

		It("should handle host errors correctly", func() {
			By("Creating a PhysicalHost in error state")
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "error-host",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "redfish://dummy",
						CredentialsSecretRef: "dummy-secret",
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       b7machine.Kind,
						APIVersion: b7machine.APIVersion,
						Name:       b7machine.Name,
						Namespace:  b7machine.Namespace,
					},
				},
				Status: infrastructurev1alpha1.PhysicalHostStatus{
					State:        infrastructurev1alpha1.StateError,
					ErrorMessage: "Redfish connection failed",
					Ready:        false,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			By("Creating a Beskar7Machine")
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// First reconcile should add finalizer
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Second reconcile should detect the error state
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify the machine status reflects the error
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      b7machine.Name,
					Namespace: b7machine.Namespace,
				}, updatedMachine)).To(Succeed())
				g.Expect(conditions.IsFalse(updatedMachine, infrastructurev1alpha1.PhysicalHostAssociatedCondition)).To(BeTrue())
				g.Expect(conditions.GetReason(updatedMachine, infrastructurev1alpha1.PhysicalHostAssociatedCondition)).To(Equal(infrastructurev1alpha1.PhysicalHostErrorReason))
				g.Expect(updatedMachine.Status.FailureMessage).NotTo(BeNil())
				g.Expect(*updatedMachine.Status.FailureMessage).To(ContainSubstring("Redfish connection failed"))
			}, "30s", "100ms").Should(Succeed())
		})

		It("should provision a claimed PhysicalHost", func() {
			By("Creating a claimed PhysicalHost")
			imageUrl := "http://example.com/test.iso"
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
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
				Status: infrastructurev1alpha1.PhysicalHostStatus{
					State: infrastructurev1alpha1.StateClaimed,
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			By("Creating a Beskar7Machine with the same image URL")
			b7machine.Spec.ImageURL = imageUrl
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// First reconcile should add finalizer
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Second reconcile should find the claimed host
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify the host is being provisioned
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      host.Name,
					Namespace: host.Namespace,
				}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1alpha1.StateProvisioning))
				g.Expect(conditions.IsFalse(updatedHost, infrastructurev1alpha1.HostProvisionedCondition)).To(BeTrue())
				g.Expect(conditions.GetReason(updatedHost, infrastructurev1alpha1.HostProvisionedCondition)).To(Equal(infrastructurev1alpha1.ProvisioningReason))
			}, "30s", "100ms").Should(Succeed())

			// Verify the machine status
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      b7machine.Name,
					Namespace: b7machine.Namespace,
				}, updatedMachine)).To(Succeed())
				g.Expect(conditions.IsTrue(updatedMachine, infrastructurev1alpha1.PhysicalHostAssociatedCondition)).To(BeTrue())
				g.Expect(updatedMachine.Spec.ProviderID).NotTo(BeNil())
				expectedProviderID := providerID(host.Namespace, host.Name)
				g.Expect(*updatedMachine.Spec.ProviderID).To(Equal(expectedProviderID))
			}, "30s", "100ms").Should(Succeed())
		})
	})

	Context("Reconcile Delete", func() {
		It("should release PhysicalHost on deletion", func() {
			By("Creating a claimed PhysicalHost")
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "redfish://dummy",
						CredentialsSecretRef: "dummy-secret",
					},
					ConsumerRef: &corev1.ObjectReference{
						Kind:       b7machine.Kind,
						APIVersion: b7machine.APIVersion,
						Name:       b7machine.Name,
						Namespace:  b7machine.Namespace,
					},
				},
				Status: infrastructurev1alpha1.PhysicalHostStatus{
					State: infrastructurev1alpha1.StateClaimed,
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			By("Creating a Beskar7Machine")
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// First reconcile should add finalizer
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Second reconcile should find the claimed host
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify the host is claimed
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      host.Name,
					Namespace: host.Namespace,
				}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
				g.Expect(updatedHost.Spec.ConsumerRef.Name).To(Equal(b7machine.Name))
			}, "30s", "100ms").Should(Succeed())

			By("Deleting the Beskar7Machine")
			Expect(k8sClient.Delete(ctx, b7machine)).To(Succeed())

			// Reconcile deletion
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      b7machine.Name,
				Namespace: b7machine.Namespace,
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify the host is released
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      host.Name,
					Namespace: host.Namespace,
				}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).To(BeNil())
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1alpha1.StateAvailable))
			}, "30s", "100ms").Should(Succeed())

			// Verify the machine is deleted
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      b7machine.Name,
					Namespace: b7machine.Namespace,
				}, &infrastructurev1alpha1.Beskar7Machine{})
				g.Expect(err).To(HaveOccurred())
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}, "30s", "100ms").Should(Succeed())
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
		It("should configure boot for PreBakedISO mode", func() {
			// Create a PhysicalHost
			host := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prebaked-host",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "https://redfish-mock.example.com",
						CredentialsSecretRef: "dummy-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			// Create a Beskar7Machine with PreBakedISO mode
			b7machine := &infrastructurev1alpha1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.Beskar7MachineSpec{
					BootMode: infrastructurev1alpha1.PreBakedISO,
					ImageURL: "https://example.com/prebaked.iso",
				},
			}
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// First reconcile to add finalizer
			key := types.NamespacedName{Name: b7machine.Name, Namespace: b7machine.Namespace}
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Second reconcile to claim host and configure boot
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify PhysicalHost is claimed
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed())

			// Assert Redfish calls
			Expect(mockRfClient.SetBootParametersCalled).To(BeTrue(), "SetBootParameters should be called once for PreBakedISO")
			Expect(mockRfClient.StoredBootParams).To(BeNil(), "SetBootParameters should be called with nil params for PreBakedISO")
			Expect(mockRfClient.SetBootSourceCalled).To(BeTrue(), "SetBootSourceISO should be called once for PreBakedISO")
			Expect(mockRfClient.InsertedISO).To(Equal("https://example.com/prebaked.iso"), "SetBootSourceISO called with incorrect ImageURL for PreBakedISO")
		})

		It("should configure boot for RemoteConfig mode with Kairos", func() {
			// Create a PhysicalHost
			host := &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "remote-config-host",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "https://redfish-mock.example.com",
						CredentialsSecretRef: "dummy-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			// Create a Beskar7Machine with RemoteConfig mode
			genericIsoURL := "https://example.com/generic.iso"
			remoteConfigURL := "https://example.com/config.yaml"
			b7machine := &infrastructurev1alpha1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.Beskar7MachineSpec{
					BootMode:        infrastructurev1alpha1.RemoteConfig,
					ImageURL:        genericIsoURL,
					RemoteConfigURL: remoteConfigURL,
				},
			}
			Expect(k8sClient.Create(ctx, b7machine)).To(Succeed())

			// First reconcile to add finalizer
			key := types.NamespacedName{Name: b7machine.Name, Namespace: b7machine.Namespace}
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Second reconcile to claim host and configure boot
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify PhysicalHost is claimed
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed())

			// Assert Redfish calls
			Expect(mockRfClient.SetBootParametersCalled).To(BeTrue(), "SetBootParameters should be called once for RemoteConfig")
			Expect(mockRfClient.StoredBootParams).To(Equal([]string{fmt.Sprintf("kairos.config=%s", remoteConfigURL)}), "SetBootParameters called with incorrect RemoteConfig params")
			Expect(mockRfClient.SetBootSourceCalled).To(BeTrue(), "SetBootSourceISO should be called once for RemoteConfig")
			Expect(mockRfClient.InsertedISO).To(Equal(genericIsoURL), "SetBootSourceISO called with incorrect ImageURL for RemoteConfig")
		})

		It("should configure boot for RemoteConfig mode with Flatcar", func() {
			remoteConfigURL := "https://example.com/flatcar-ignition.json"
			genericIsoURL := "http://example.com/flatcar-generic.iso"

			b7machine.Spec.OSFamily = "flatcar"
			b7machine.Spec.ImageURL = genericIsoURL
			b7machine.Spec.ProvisioningMode = "RemoteConfig"
			b7machine.Spec.ConfigURL = remoteConfigURL

			// Create a PhysicalHost that the reconciler will find and claim
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "available-host-flatcar", Namespace: testNs.Name},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "redfish://dummy-flatcar",
						CredentialsSecretRef: "dummy-secret-flatcar",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			Eventually(func(g Gomega) {
				createdHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				createdHost.Status = infrastructurev1alpha1.PhysicalHostStatus{State: infrastructurev1alpha1.StateAvailable, Ready: true}
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
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return mockRfClient, nil
				},
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
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed(), "PhysicalHost should be claimed after second reconcile for flatcar")

			// Assert Redfish calls
			Expect(mockRfClient.SetBootParametersCalled).To(BeTrue(), "SetBootParameters should be called once for Flatcar")
			Expect(mockRfClient.StoredBootParams).To(Equal([]string{fmt.Sprintf("flatcar.ignition.config.url=%s", remoteConfigURL)}), "SetBootParameters called with incorrect Flatcar params")
			Expect(mockRfClient.SetBootSourceCalled).To(BeTrue(), "SetBootSourceISO should be called once for Flatcar")
			Expect(mockRfClient.InsertedISO).To(Equal(genericIsoURL), "SetBootSourceISO called with incorrect ImageURL for Flatcar")
		})

		It("should configure boot for RemoteConfig mode with LeapMicro", func() {
			remoteConfigURL := "https://example.com/leap-micro-combustion.script"
			genericIsoURL := "http://example.com/leap-micro-generic.iso"

			b7machine.Spec.OSFamily = "LeapMicro"
			b7machine.Spec.ImageURL = genericIsoURL
			b7machine.Spec.ProvisioningMode = "RemoteConfig"
			b7machine.Spec.ConfigURL = remoteConfigURL

			// Create a PhysicalHost that the reconciler will find and claim
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "available-host-leapmicro", Namespace: testNs.Name},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "redfish://dummy-leapmicro",
						CredentialsSecretRef: "dummy-secret-leapmicro",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			Eventually(func(g Gomega) {
				createdHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				createdHost.Status = infrastructurev1alpha1.PhysicalHostStatus{State: infrastructurev1alpha1.StateAvailable, Ready: true}
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
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return mockRfClient, nil
				},
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
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed(), "PhysicalHost should be claimed after second reconcile for leapmicro")

			// Assert Redfish calls
			Expect(mockRfClient.SetBootParametersCalled).To(BeTrue(), "SetBootParameters should be called once for LeapMicro")
			Expect(mockRfClient.StoredBootParams).To(Equal([]string{fmt.Sprintf("combustion.path=%s", remoteConfigURL)}), "SetBootParameters called with incorrect LeapMicro params")
			Expect(mockRfClient.SetBootSourceCalled).To(BeTrue(), "SetBootSourceISO should be called once for LeapMicro")
			Expect(mockRfClient.InsertedISO).To(Equal(genericIsoURL), "SetBootSourceISO called with incorrect ImageURL for LeapMicro")
		})

		It("should configure boot for RemoteConfig mode with Talos", func() {
			remoteConfigURL := "https://example.com/talos-machineconfig.yaml"
			genericIsoURL := "http://example.com/talos-generic.iso"

			b7machine.Spec.OSFamily = "talos"
			b7machine.Spec.ImageURL = genericIsoURL
			b7machine.Spec.ProvisioningMode = "RemoteConfig"
			b7machine.Spec.ConfigURL = remoteConfigURL

			// Create a PhysicalHost that the reconciler will find and claim
			host = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "available-host-talos", Namespace: testNs.Name},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "redfish://dummy-talos",
						CredentialsSecretRef: "dummy-secret-talos",
					},
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())
			Eventually(func(g Gomega) {
				createdHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, createdHost)).To(Succeed())
				createdHost.Status = infrastructurev1alpha1.PhysicalHostStatus{State: infrastructurev1alpha1.StateAvailable, Ready: true}
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
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return mockRfClient, nil
				},
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
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: host.Name, Namespace: host.Namespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
			}, "15s", "200ms").Should(Succeed(), "PhysicalHost should be claimed after second reconcile for talos")

			// Assert Redfish calls
			Expect(mockRfClient.SetBootParametersCalled).To(BeTrue(), "SetBootParameters should be called once for Talos")
			Expect(mockRfClient.StoredBootParams).To(Equal([]string{fmt.Sprintf("talos.config=%s", remoteConfigURL)}), "SetBootParameters called with incorrect Talos params")
			Expect(mockRfClient.SetBootSourceCalled).To(BeTrue(), "SetBootSourceISO should be called once for Talos")
			Expect(mockRfClient.InsertedISO).To(Equal(genericIsoURL), "SetBootSourceISO called with incorrect ImageURL for Talos")
		})

		// TODO: Add test for "RemoteConfig" mode with missing ConfigURL (error expected)
		// TODO: Add test for "RemoteConfig" mode with unsupported OSFamily (error expected)

	})

	Context("Error Handling", func() {
		var (
			physicalHost     *infrastructurev1alpha1.PhysicalHost
			mockRfClient     *MockRedfishClient
			reconciler       *Beskar7MachineReconciler
			ownerMachine     *clusterv1.Machine
			credentialSecret *corev1.Secret
			testNs           *corev1.Namespace
			beskar7Machine   *infrastructurev1alpha1.Beskar7Machine
		)

		BeforeEach(func() {
			// Create a unique namespace for the test
			testNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "error-test-",
				},
			}
			Expect(k8sClient.Create(ctx, testNs)).To(Succeed())

			// Create owner CAPI Machine
			ownerMachine = &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-machine-%d", GinkgoParallelProcess()),
					Namespace: testNs.Name,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: "test-cluster",
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: pointer.String("test-bootstrap-secret"),
					},
				},
			}
			Expect(k8sClient.Create(ctx, ownerMachine)).To(Succeed())

			// Create Beskar7Machine
			beskar7Machine = &infrastructurev1alpha1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-beskar7-%d", GinkgoParallelProcess()),
					Namespace: testNs.Name,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: clusterv1.GroupVersion.String(),
							Kind:       "Machine",
							Name:       ownerMachine.Name,
							UID:        ownerMachine.UID,
						},
					},
				},
				Spec: infrastructurev1alpha1.Beskar7MachineSpec{
					OSFamily: "kairos", // Changed from "ubuntu" to "kairos"
				},
			}
			Expect(k8sClient.Create(ctx, beskar7Machine)).To(Succeed())

			// Create PhysicalHost
			physicalHost = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-host-%d", GinkgoParallelProcess()),
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "https://redfish-mock.example.com",
						CredentialsSecretRef: fmt.Sprintf("test-redfish-credentials-%d", GinkgoParallelProcess()),
					},
				},
			}
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			// Create credential secret
			credentialSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-redfish-credentials-%d", GinkgoParallelProcess()),
					Namespace: testNs.Name,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			}
			Expect(k8sClient.Create(ctx, credentialSecret)).To(Succeed())

			// Initialize mock client
			mockRfClient = &MockRedfishClient{
				GetSystemInfoFunc: func(ctx context.Context) (*internalredfish.SystemInfo, error) {
					return &internalredfish.SystemInfo{
						Status: common.Status{State: common.EnabledState},
					}, nil
				},
				GetPowerStateFunc: func(ctx context.Context) (redfish.PowerState, error) {
					return redfish.OffPowerState, nil
				},
				SetPowerStateFunc: func(ctx context.Context, state redfish.PowerState) error {
					return nil
				},
				SetBootSourceISOFunc: func(ctx context.Context, isoURL string) error {
					mockRfClient.SetBootSourceCalled = true
					mockRfClient.InsertedISO = isoURL
					mockRfClient.SetBootSourceISOCalls = append(mockRfClient.SetBootSourceISOCalls, isoURL)
					return nil
				},
				SetBootParametersFunc: func(ctx context.Context, params []string) error {
					mockRfClient.SetBootParametersCalled = true
					mockRfClient.StoredBootParams = params
					mockRfClient.SetBootParametersCalls = append(mockRfClient.SetBootParametersCalls, params)
					return nil
				},
				ShouldFail: make(map[string]error),
			}

			// Initialize reconciler
			reconciler = &Beskar7MachineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return mockRfClient, nil
				},
			}
		})

		AfterEach(func() {
			// Clean up resources
			Expect(k8sClient.Delete(ctx, beskar7Machine)).To(Succeed())
			Expect(k8sClient.Delete(ctx, physicalHost)).To(Succeed())
			Expect(k8sClient.Delete(ctx, credentialSecret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ownerMachine)).To(Succeed())
			Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
		})

		It("should handle connection errors", func() {
			// TODO: Future test implementation
			Skip("Skipping connection error test for now")
			By("Simulating a connection error")
			mockRfClient.ShouldFail["GetSystemInfo"] = fmt.Errorf("connection refused")

			// Reconcile should fail
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      beskar7Machine.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))

			// Check that the machine status reflects the error
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      beskar7Machine.Name,
					Namespace: testNs.Name,
				}, updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Status.FailureMessage).To(ContainSubstring("connection refused"))
			}, "10s", "100ms").Should(Succeed())
		})

		It("should handle power state errors", func() {
			// TODO: Future test implementation
			Skip("Skipping power state error test for now")
			By("Simulating a power state error")
			mockRfClient.ShouldFail["GetPowerState"] = fmt.Errorf("failed to get power state")

			// Reconcile should fail
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      beskar7Machine.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get power state"))

			// Check that the machine status reflects the error
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      beskar7Machine.Name,
					Namespace: testNs.Name,
				}, updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Status.FailureMessage).To(ContainSubstring("failed to get power state"))
			}, "10s", "100ms").Should(Succeed())
		})

		It("should handle boot configuration errors", func() {
			// TODO: Future test implementation
			Skip("Skipping boot configuration error test for now")
			By("Simulating a boot configuration error")
			mockRfClient.ShouldFail["SetBootSourceISO"] = fmt.Errorf("failed to set boot source")

			// Reconcile should fail
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      beskar7Machine.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to set boot source"))

			// Check that the machine status reflects the error
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      beskar7Machine.Name,
					Namespace: testNs.Name,
				}, updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Status.FailureMessage).To(ContainSubstring("failed to set boot source"))
			}, "10s", "100ms").Should(Succeed())
		})

		It("should handle boot parameter errors", func() {
			// TODO: Future test implementation
			Skip("Skipping boot parameter error test for now")
			By("Simulating a boot parameter error")
			mockRfClient.ShouldFail["SetBootParameters"] = fmt.Errorf("failed to set boot parameters")

			// Reconcile should fail
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      beskar7Machine.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to set boot parameters"))

			// Check that the machine status reflects the error
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      beskar7Machine.Name,
					Namespace: testNs.Name,
				}, updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Status.FailureMessage).To(ContainSubstring("failed to set boot parameters"))
			}, "10s", "100ms").Should(Succeed())
		})

		It("should handle missing physical host", func() {
			// TODO: Future test implementation
			Skip("Skipping missing physical host test for now")
			By("Deleting the physical host")
			Expect(k8sClient.Delete(ctx, physicalHost)).To(Succeed())

			// Reconcile should fail
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      beskar7Machine.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// Check that the machine status reflects the error
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      beskar7Machine.Name,
					Namespace: testNs.Name,
				}, updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Status.FailureMessage).To(ContainSubstring("not found"))
			}, "10s", "100ms").Should(Succeed())
		})

		It("should handle missing credential secret", func() {
			// TODO: Future test implementation
			Skip("Skipping missing credential secret test for now")
			By("Deleting the credential secret")
			Expect(k8sClient.Delete(ctx, credentialSecret)).To(Succeed())

			// Reconcile should fail
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      beskar7Machine.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret"))

			// Check that the machine status reflects the error
			Eventually(func(g Gomega) {
				updatedMachine := &infrastructurev1alpha1.Beskar7Machine{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      beskar7Machine.Name,
					Namespace: testNs.Name,
				}, updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Status.FailureMessage).To(ContainSubstring("secret"))
			}, "10s", "100ms").Should(Succeed())
		})
	})

})

func createOwnerMachine(ctx context.Context, namespace string) *clusterv1.Machine {
	ownerMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: namespace,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test-cluster",
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: pointer.String("test-bootstrap-secret"),
			},
		},
	}
	Expect(k8sClient.Create(ctx, ownerMachine)).To(Succeed())
	return ownerMachine
}
