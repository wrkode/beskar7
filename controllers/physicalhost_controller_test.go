package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/redfish"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	internalredfish "github.com/wrkode/beskar7/internal/redfish" // Import internal redfish
)

var _ = Describe("PhysicalHost Controller", func() {

	const ( // Define constants for test resources
		PhNamespace = "default"
		PhName      = "test-physicalhost"
		SecretName  = "test-redfish-credentials"
		Timeout     = time.Second * 10
		Interval    = time.Millisecond * 250
	)

	Context("When reconciling a PhysicalHost", func() {
		var physicalHost *infrastructurev1alpha1.PhysicalHost
		var credentialSecret *corev1.Secret
		var mockRfClient *internalredfish.MockClient // Added mock client variable
		var reconciler *PhysicalHostReconciler       // Added reconciler variable

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
						Address:              "redfish-mock.example.com", // Doesn't matter for mock
						CredentialsSecretRef: SecretName,
					},
				},
			}

			// Create Mock Redfish Client
			mockRfClient = internalredfish.NewMockClient()

			// Create the reconciler instance for the test
			reconciler = &PhysicalHostReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				// Define a factory that returns our mock client instance
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					// You could add assertions here on address/username/password if needed
					return mockRfClient, nil
				},
			}
		})

		AfterEach(func() {
			// Clean up resources
			Expect(k8sClient.Delete(ctx, physicalHost)).To(Succeed())
			Expect(k8sClient.Delete(ctx, credentialSecret)).To(Succeed())
			// Optionally delete namespace if created for test
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
			}, Timeout, Interval).Should(Succeed(), "Finalizer should be added")

			By("Reconciling again after finalizer addition")
			_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred(), "Second reconcile loop failed")

			// Now expect the state to become Available
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, phLookupKey, createdPh)).To(Succeed())
				g.Expect(createdPh.Status.State).To(Equal(infrastructurev1alpha1.StateAvailable))
				g.Expect(createdPh.Status.ObservedPowerState).To(Equal(redfish.OffPowerState)) // Mock default
				g.Expect(createdPh.Status.HardwareDetails).NotTo(BeNil())
				// TODO: Add condition checks using conditions.IsTrue, etc.
			}, Timeout, Interval).Should(Succeed(), "PhysicalHost should become Available")

			// Verify mock client methods were called (optional)
			Expect(mockRfClient.GetSystemInfoCalled).To(BeTrue())
			Expect(mockRfClient.GetPowerStateCalled).To(BeTrue())

		})

		// TODO: Add more tests:
		// - Test deletion/finalizer removal
		// - Test Redfish connection failure (using mock)
		// - Test secret not found / missing data
		// - Test provisioning flow (when claimed by a machine)
		//   - Check SetBootSourceISO called
		//   - Check SetPowerState called
		//   - Check status becomes Provisioned

	})
})
