package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	conditions "sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
	"github.com/wrkode/beskar7/internal/statemachine"
)

var _ = Describe("PhysicalHost Controller", func() {

	const (
		PhNamespace = "default"
		PhName      = "test-physicalhost"
		SecretName  = "test-redfish-credentials"
		Timeout     = time.Second * 10
		Interval    = time.Millisecond * 250
	)

	Context("When reconciling a PhysicalHost", func() {
		var physicalHost *infrastructurev1alpha1.PhysicalHost
		var credentialSecret *corev1.Secret
		var mockRfClient *internalredfish.MockClient
		var reconciler *PhysicalHostReconciler

		BeforeEach(func() {
			// Create namespace if needed (usually default exists)
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: PhNamespace}}
			Expect(k8sClient.Create(ctx, ns)).To(SatisfyAny(Succeed(), MatchError(ContainSubstring("already exists"))))

			// Create the credential secret
			credentialSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: PhNamespace,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			}
			Expect(k8sClient.Create(ctx, credentialSecret)).To(Succeed())

			// Define the PhysicalHost resource
			physicalHost = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PhName,
					Namespace: PhNamespace,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "redfish-mock.example.com",
						CredentialsSecretRef: SecretName,
					},
				},
				Status: infrastructurev1alpha1.PhysicalHostStatus{
					State: infrastructurev1alpha1.StateNone,
				},
			}

			// Create Mock Redfish Client
			mockRfClient = internalredfish.NewMockClient()
			mockRfClient.SystemInfo = &internalredfish.SystemInfo{
				Manufacturer: "TestManufacturer",
				Model:        "TestModel",
				SerialNumber: "TestSerial",
				Status:       common.Status{State: common.EnabledState},
			}
			mockRfClient.PowerState = redfish.OffPowerState
			mockRfClient.ShouldFail = make(map[string]error)

			// Create the reconciler instance for the test
			reconciler = &PhysicalHostReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return mockRfClient, nil
				},
				StateMachine: statemachine.NewPhysicalHostStateMachine(),
			}
		})

		AfterEach(func() {
			// Clean up resources
			Expect(k8sClient.Delete(ctx, physicalHost)).To(Succeed())
			Expect(k8sClient.Delete(ctx, credentialSecret)).To(Succeed())
		})

		It("Should successfully reconcile and become Available", func() {
			By("Creating the PhysicalHost resource")
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: PhName, Namespace: PhNamespace}

			// Directly reconcile once
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred(), "First reconcile loop failed")

			// First reconcile adds finalizer and requeues
			createdPh := &infrastructurev1alpha1.PhysicalHost{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, phLookupKey, createdPh)).To(Succeed())
				g.Expect(createdPh.Finalizers).To(ContainElement(PhysicalHostFinalizer))
				g.Expect(createdPh.Status.State).To(Equal(infrastructurev1alpha1.StateEnrolling))
			}, Timeout, Interval).Should(Succeed(), "Finalizer should be added and state should be Enrolling")

			By("Reconciling again after finalizer addition")
			_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred(), "Second reconcile loop failed")

			// Now expect the state to become Available
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, phLookupKey, createdPh)).To(Succeed())
				g.Expect(createdPh.Status.State).To(Equal(infrastructurev1alpha1.StateAvailable))
				g.Expect(createdPh.Status.ObservedPowerState).To(Equal(redfish.OffPowerState))
				g.Expect(createdPh.Status.HardwareDetails).NotTo(BeNil())
				g.Expect(createdPh.Status.HardwareDetails.Manufacturer).To(Equal("TestManufacturer"))
				g.Expect(createdPh.Status.HardwareDetails.Model).To(Equal("TestModel"))
				g.Expect(createdPh.Status.HardwareDetails.SerialNumber).To(Equal("TestSerial"))
				g.Expect(conditions.IsTrue(createdPh, infrastructurev1alpha1.HostAvailableCondition)).To(BeTrue())
			}, Timeout, Interval).Should(Succeed(), "PhysicalHost should become Available")

			// Verify mock client methods were called
			Expect(mockRfClient.GetSystemInfoCalled).To(BeTrue(), "GetSystemInfo should have been called")
			Expect(mockRfClient.GetPowerStateCalled).To(BeTrue(), "GetPowerState should have been called")
		})

		// TODO: Fix deprovisioning test - EjectVirtualMedia is not being called during deprovisioning
		PIt("Should deprovision and remove finalizer on delete", func() {
			By("Creating the PhysicalHost resource with a finalizer")
			// Ensure the mock client will report the host as 'On' initially to test power-off
			mockRfClient.PowerState = redfish.OnPowerState
			mockRfClient.InsertedISO = "http://example.com/test.iso" // Simulate media inserted

			// Use unique names for this test
			deletePhName := PhName + "-delete"
			deleteSecretName := SecretName + "-delete"

			phToCreate := physicalHost.DeepCopy()
			phToCreate.Name = deletePhName
			phToCreate.Spec.RedfishConnection.CredentialsSecretRef = deleteSecretName
			phToCreate.Finalizers = []string{PhysicalHostFinalizer}
			phToCreate.Status.State = infrastructurev1alpha1.StateProvisioned
			phToCreate.Spec.ConsumerRef = nil

			// Create unique secret for this test
			deleteSecret := credentialSecret.DeepCopy()
			deleteSecret.Name = deleteSecretName
			deleteSecret.ResourceVersion = ""
			Expect(k8sClient.Create(ctx, deleteSecret)).To(Succeed())

			Expect(k8sClient.Create(ctx, phToCreate)).To(Succeed())
			Eventually(func(g Gomega) {
				getStatusPh := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deletePhName, Namespace: PhNamespace}, getStatusPh)).To(Succeed())
				getStatusPh.Status.State = infrastructurev1alpha1.StateProvisioned
				getStatusPh.Status.ObservedPowerState = redfish.OnPowerState
				g.Expect(k8sClient.Status().Update(ctx, getStatusPh)).To(Succeed())
			}, Timeout, Interval).Should(Succeed(), "Failed to set initial status for deletion test")

			phLookupKey := types.NamespacedName{Name: deletePhName, Namespace: PhNamespace}

			By("Deleting the PhysicalHost resource")
			Expect(k8sClient.Delete(ctx, phToCreate)).To(Succeed())

			By("Reconciling to trigger deprovisioning, Redfish actions, and finalizer removal setup")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred(), "Reconcile for deprovisioning failed")

			// Verify mock client methods were called for deprovisioning
			Expect(mockRfClient.EjectMediaCalled).To(BeTrue(), "EjectVirtualMedia should have been called")
			Expect(mockRfClient.SetPowerStateCalled).To(BeTrue(), "SetPowerState should have been called")
			Expect(mockRfClient.PowerState).To(Equal(redfish.OffPowerState), "SetPowerState should have been called with OffPowerState")

			// Check if state moved to deprovisioning
			deletedPh := &infrastructurev1alpha1.PhysicalHost{}
			err = k8sClient.Get(ctx, phLookupKey, deletedPh)
			if err == nil {
				Expect(deletedPh.Status.State).To(Equal(infrastructurev1alpha1.StateDeprovisioning))
				cond := conditions.Get(deletedPh, infrastructurev1alpha1.HostProvisionedCondition)
				Expect(cond).NotTo(BeNil())
				Expect(cond.Reason).To(SatisfyAny(Equal(infrastructurev1alpha1.DeprovisioningReason), Equal(clusterv1.DeletingReason)))
			} else {
				Expect(client.IgnoreNotFound(err)).To(BeNil(), "Error getting PH, should be NotFound or nil")
			}

			By("Ensuring PhysicalHost is eventually deleted from API server (finalizer removed)")
			Eventually(func() bool {
				ph := &infrastructurev1alpha1.PhysicalHost{}
				errGet := k8sClient.Get(ctx, phLookupKey, ph)
				return client.IgnoreNotFound(errGet) == nil
			}, Timeout*2, Interval).Should(BeTrue(), "PhysicalHost should be deleted from API server")

			// Cleanup the unique secret for this test
			Expect(k8sClient.Delete(ctx, deleteSecret)).To(Succeed())
		})
	})
})
