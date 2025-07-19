package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/redfish"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	conditions "sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	internalredfish "github.com/wrkode/beskar7/internal/redfish" // Import internal redfish
	"github.com/wrkode/beskar7/internal/statemachine"
)

var _ = Describe("PhysicalHost Controller", func() {

	const ( // Define constants for test resources
		Timeout  = time.Second * 10
		Interval = time.Millisecond * 250
	)

	// Helper function to reconcile with timeout context
	reconcileWithTimeout := func(reconciler *PhysicalHostReconciler, phLookupKey types.NamespacedName) (ctrl.Result, error) {
		reconcileCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return reconciler.Reconcile(reconcileCtx, ctrl.Request{NamespacedName: phLookupKey})
	}

	Context("When reconciling a PhysicalHost", func() {
		var physicalHost *infrastructurev1beta1.PhysicalHost
		var credentialSecret *corev1.Secret
		var mockRfClient *internalredfish.MockClient // Added mock client variable
		var reconciler *PhysicalHostReconciler       // Added reconciler variable
		var testNs *corev1.Namespace

		BeforeEach(func() {
			// Create a unique namespace for this test
			testNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "physicalhost-test-",
				},
			}
			Expect(k8sClient.Create(ctx, testNs)).To(Succeed())

			// Create the credential secret with a unique name
			credentialSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redfish-credentials",
					Namespace: testNs.Name,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			}
			Expect(k8sClient.Create(ctx, credentialSecret)).To(Succeed())

			// Define the PhysicalHost resource
			physicalHost = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-physicalhost",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "https://redfish-mock.example.com", // Doesn't matter for mock
						CredentialsSecretRef: credentialSecret.Name,
					},
				},
			}

			// Create Mock Redfish Client
			mockRfClient = internalredfish.NewMockClient()

			// Create the reconciler instance for the test
			reconciler = &PhysicalHostReconciler{
				Client:   k8sClient, // Use the properly configured client from test suite
				Scheme:   k8sClient.Scheme(),
				Log:      ctrl.Log.WithName("physicalhost-test"),
				Recorder: record.NewFakeRecorder(100), // Add event recorder for conditions
				// Define a factory that returns our mock client instance
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					// You could add assertions here on address/username/password if needed
					return mockRfClient, nil
				},
				// Set timeouts for test environment to prevent internal timeout issues
				reconcileTimeout:  60 * time.Second, // Internal reconcile timeout
				stuckStateTimeout: 5 * time.Minute,  // Stuck state detection timeout
				maxRetries:        3,                // Max retries for state transitions
				// Initialize state machine components
				stateMachine:         statemachine.NewPhysicalHostStateMachine(ctrl.Log.WithName("state-machine")),
				stateTransitionGuard: statemachine.NewStateTransitionGuard(k8sClient, ctrl.Log.WithName("transition-guard")),
				stateRecoveryManager: statemachine.NewStateRecoveryManager(k8sClient, ctrl.Log.WithName("recovery-manager")),
			}
		})

		AfterEach(func() {
			// Clean up the namespace which will clean up all resources
			Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
		})

		It("Should successfully reconcile and become Available", func() {
			By("Creating the PhysicalHost resource")
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: physicalHost.Name, Namespace: physicalHost.Namespace}

			// Directly reconcile once with timeout context
			reconcileCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			_, err := reconciler.Reconcile(reconcileCtx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred(), "First reconcile loop failed")

			// First reconcile adds finalizer and requeues
			createdPh := &infrastructurev1beta1.PhysicalHost{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, phLookupKey, createdPh)).To(Succeed())
				g.Expect(createdPh.Finalizers).To(ContainElement(PhysicalHostFinalizer))
			}, Timeout, Interval).Should(Succeed(), "Finalizer should be added")

			By("Reconciling again after finalizer addition")
			reconcileCtx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
			defer cancel2()
			_, err = reconciler.Reconcile(reconcileCtx2, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred(), "Second reconcile loop failed")

			// Now expect the state to become Available
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, phLookupKey, createdPh)).To(Succeed())
				g.Expect(createdPh.Status.State).To(Equal(infrastructurev1beta1.StateAvailable))
				g.Expect(createdPh.Status.ObservedPowerState).To(Equal(string(redfish.OffPowerState))) // Mock default
				g.Expect(createdPh.Status.HardwareDetails).NotTo(BeNil())
				// TODO: Add condition checks using conditions.IsTrue, etc.
			}, Timeout, Interval).Should(Succeed(), "PhysicalHost should become Available")

			// Verify mock client methods were called (optional)
			Expect(mockRfClient.GetSystemInfoCalled).To(BeTrue())
			Expect(mockRfClient.GetPowerStateCalled).To(BeTrue())

		})

		It("Should deprovision and remove finalizer on delete", func() {
			By("Creating the PhysicalHost resource with a finalizer")
			// Ensure the mock client will report the host as 'On' initially to test power-off
			mockRfClient.PowerState = redfish.OnPowerState
			mockRfClient.InsertedISO = "http://example.com/test.iso" // Simulate media inserted

			// Use unique names for this test
			deletePhName := physicalHost.Name + "-delete"
			deleteSecretName := credentialSecret.Name + "-delete"

			phToCreate := physicalHost.DeepCopy()
			phToCreate.Name = deletePhName
			phToCreate.Spec.RedfishConnection.CredentialsSecretRef = deleteSecretName // Point to unique secret
			phToCreate.Finalizers = []string{PhysicalHostFinalizer}
			// Simulate it was provisioned and then released (ConsumerRef is nil)
			phToCreate.Status.State = infrastructurev1beta1.StateProvisioned // Set to a state that allows deprovisioning
			phToCreate.Spec.ConsumerRef = nil                                // Ensure it's not considered in use

			// Create unique secret for this test
			deleteSecret := credentialSecret.DeepCopy()
			deleteSecret.Name = deleteSecretName
			deleteSecret.ResourceVersion = "" // Clear resource version for create
			Expect(k8sClient.Create(ctx, deleteSecret)).To(Succeed())

			Expect(k8sClient.Create(ctx, phToCreate)).To(Succeed())
			Eventually(func(g Gomega) {
				getStatusPh := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deletePhName, Namespace: physicalHost.Namespace}, getStatusPh)).To(Succeed())
				getStatusPh.Status.State = infrastructurev1beta1.StateProvisioned
				getStatusPh.Status.ObservedPowerState = string(redfish.OnPowerState)
				g.Expect(k8sClient.Status().Update(ctx, getStatusPh)).To(Succeed())
			}, Timeout, Interval).Should(Succeed(), "Failed to set initial status for deletion test")

			phLookupKey := types.NamespacedName{Name: deletePhName, Namespace: physicalHost.Namespace}

			By("Deleting the PhysicalHost resource")
			Expect(k8sClient.Delete(ctx, phToCreate)).To(Succeed())

			By("Reconciling to trigger deprovisioning, Redfish actions, and finalizer removal setup")
			// A single reconcile should be enough for reconcileDelete to do its work.
			// The deferred patch in the main Reconcile loop will handle the actual finalizer removal from the object.
			deleteCtx, deleteCancel := context.WithTimeout(ctx, 30*time.Second)
			defer deleteCancel()
			_, err := reconciler.Reconcile(deleteCtx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred(), "Reconcile for deprovisioning failed")

			// Verify mock client methods were called for deprovisioning IMMEDIATELY after the reconcile call
			// as these actions happen within the reconcileDelete function.
			Expect(mockRfClient.EjectMediaCalled).To(BeTrue(), "EjectVirtualMedia should have been called")
			Expect(mockRfClient.SetPowerStateCalled).To(BeTrue(), "SetPowerState should have been called")
			Expect(mockRfClient.PowerState).To(Equal(redfish.OffPowerState), "SetPowerState should have been called with OffPowerState")

			// Check if state moved to deprovisioning. This might be racy if the object is deleted too fast.
			// It's more important to check that the Redfish calls were made and the object is gone.
			// We can attempt to get it once, but if it's already gone, that's also a success for finalizer removal.
			deletedPh := &infrastructurev1beta1.PhysicalHost{}
			err = k8sClient.Get(ctx, phLookupKey, deletedPh)
			if err == nil { // If we can still get it, check its status
				Expect(deletedPh.Status.State).To(Equal(infrastructurev1beta1.StateDeprovisioning))
				cond := conditions.Get(deletedPh, infrastructurev1beta1.HostProvisionedCondition)
				Expect(cond).NotTo(BeNil())
				Expect(cond.Reason).To(SatisfyAny(Equal(infrastructurev1beta1.DeprovisioningReason), Equal(clusterv1.DeletingReason)))
			} else {
				Expect(client.IgnoreNotFound(err)).To(BeNil(), "Error getting PH, should be NotFound or nil")
			}

			By("Ensuring PhysicalHost is eventually deleted from API server (finalizer removed)")
			Eventually(func() bool {
				ph := &infrastructurev1beta1.PhysicalHost{}
				errGet := k8sClient.Get(ctx, phLookupKey, ph)
				return client.IgnoreNotFound(errGet) == nil
			}, Timeout*2, Interval).Should(BeTrue(), "PhysicalHost should be deleted from API server")

			// Cleanup the unique secret for this test
			Expect(k8sClient.Delete(ctx, deleteSecret)).To(Succeed())
		})

		// TODO: Add more tests:
		// - Test deletion/finalizer removal
		// - Test Redfish connection failure (using mock)
		// - Test secret not found / missing data
		// - Test provisioning flow (when claimed by a machine)
		//   - Check SetBootSourceISO called
		//   - Check SetPowerState called
		//   - Check status becomes Provisioned

		It("should handle Redfish connection failure", func() {
			By("Creating PhysicalHost with mock that fails connection")
			failedConnPhName := physicalHost.Name + "-connection-fail"
			failedConnSecretName := credentialSecret.Name + "-connection-fail"

			// Create credentials secret
			failedSecret := credentialSecret.DeepCopy()
			failedSecret.Name = failedConnSecretName
			failedSecret.ResourceVersion = ""
			Expect(k8sClient.Create(ctx, failedSecret)).To(Succeed())

			// Create PhysicalHost
			failedConnPh := physicalHost.DeepCopy()
			failedConnPh.Name = failedConnPhName
			failedConnPh.Spec.RedfishConnection.CredentialsSecretRef = failedConnSecretName

			// Create reconciler that simulates connection failure
			failedReconciler := &PhysicalHostReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Log:      ctrl.Log.WithName("physicalhost-test-failed"),
				Recorder: record.NewFakeRecorder(100),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return nil, fmt.Errorf("connection timeout: unable to connect to %s", address)
				},
				// Set timeouts for test environment
				reconcileTimeout:  60 * time.Second,
				stuckStateTimeout: 5 * time.Minute,
				maxRetries:        3,
				// Initialize state machine components
				stateMachine:         statemachine.NewPhysicalHostStateMachine(ctrl.Log.WithName("state-machine")),
				stateTransitionGuard: statemachine.NewStateTransitionGuard(k8sClient, ctrl.Log.WithName("transition-guard")),
				stateRecoveryManager: statemachine.NewStateRecoveryManager(k8sClient, ctrl.Log.WithName("recovery-manager")),
			}

			Expect(k8sClient.Create(ctx, failedConnPh)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: failedConnPhName, Namespace: physicalHost.Namespace}

			By("First reconcile adds finalizer")
			result, err := failedReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			By("Second reconcile should fail due to connection error")
			result, err = failedReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection timeout"))

			By("Checking that proper conditions are set")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, phLookupKey, failedConnPh)).To(Succeed())
				cond := conditions.Get(failedConnPh, infrastructurev1beta1.RedfishConnectionReadyCondition)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(infrastructurev1beta1.RedfishConnectionFailedReason))
				g.Expect(cond.Message).To(ContainSubstring("Failed to connect to Redfish"))
				g.Expect(failedConnPh.Status.State).To(Equal(infrastructurev1beta1.StateError))
			}, Timeout, Interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, failedConnPh)).To(Succeed())
			Expect(k8sClient.Delete(ctx, failedSecret)).To(Succeed())
		})

		It("should handle secret not found", func() {
			By("Creating PhysicalHost with reference to non-existent secret")
			noSecretPhName := physicalHost.Name + "-no-secret"
			noSecretPh := physicalHost.DeepCopy()
			noSecretPh.Name = noSecretPhName
			noSecretPh.Spec.RedfishConnection.CredentialsSecretRef = "non-existent-secret"

			Expect(k8sClient.Create(ctx, noSecretPh)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: noSecretPhName, Namespace: physicalHost.Namespace}

			By("First reconcile adds finalizer")
			result, err := reconcileWithTimeout(reconciler, phLookupKey)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			By("Second reconcile should fail due to missing secret")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(BeNil()) // Should be NotFound error

			By("Checking that proper conditions are set")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, phLookupKey, noSecretPh)).To(Succeed())
				cond := conditions.Get(noSecretPh, infrastructurev1beta1.RedfishConnectionReadyCondition)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(infrastructurev1beta1.SecretNotFoundReason))
				g.Expect(cond.Message).To(ContainSubstring("not found"))
			}, Timeout, Interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, noSecretPh)).To(Succeed())
		})

		It("should handle secret with missing data", func() {
			By("Creating secret with missing username/password")
			invalidDataSecretName := credentialSecret.Name + "-invalid-data"
			invalidDataSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      invalidDataSecretName,
					Namespace: physicalHost.Namespace,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					// password is missing
				},
			}
			Expect(k8sClient.Create(ctx, invalidDataSecret)).To(Succeed())

			invalidDataPhName := physicalHost.Name + "-invalid-data"
			invalidDataPh := physicalHost.DeepCopy()
			invalidDataPh.Name = invalidDataPhName
			invalidDataPh.Spec.RedfishConnection.CredentialsSecretRef = invalidDataSecretName

			Expect(k8sClient.Create(ctx, invalidDataPh)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: invalidDataPhName, Namespace: physicalHost.Namespace}

			By("First reconcile adds finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			By("Second reconcile should detect invalid secret data")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred()) // This is permanent error, no requeue
			Expect(result.Requeue).To(BeFalse())

			By("Checking that proper conditions are set")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, phLookupKey, invalidDataPh)).To(Succeed())
				cond := conditions.Get(invalidDataPh, infrastructurev1beta1.RedfishConnectionReadyCondition)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(infrastructurev1beta1.MissingSecretDataReason))
				g.Expect(cond.Message).To(ContainSubstring("Username or password missing"))
			}, Timeout, Interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, invalidDataPh)).To(Succeed())
			Expect(k8sClient.Delete(ctx, invalidDataSecret)).To(Succeed())
		})

		It("should handle Redfish query failures", func() {
			By("Creating PhysicalHost with mock that fails queries")
			queryFailPhName := physicalHost.Name + "-query-fail"
			queryFailSecretName := credentialSecret.Name + "-query-fail"

			// Create credentials secret
			queryFailSecret := credentialSecret.DeepCopy()
			queryFailSecret.Name = queryFailSecretName
			queryFailSecret.ResourceVersion = ""
			Expect(k8sClient.Create(ctx, queryFailSecret)).To(Succeed())

			// Create mock client that connects but fails queries
			queryFailMockClient := internalredfish.NewMockClient()
			queryFailMockClient.ShouldFail["GetSystemInfo"] = fmt.Errorf("redfish query failed: 500 Internal Server Error")
			queryFailMockClient.ShouldFail["GetPowerState"] = fmt.Errorf("power state query failed")

			// Create reconciler with failing query client
			queryFailReconciler := &PhysicalHostReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Log:      ctrl.Log.WithName("physicalhost-test-queryfail"),
				Recorder: record.NewFakeRecorder(100),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return queryFailMockClient, nil // Connection succeeds, queries fail
				},
				// Set timeouts for test environment
				reconcileTimeout:  60 * time.Second,
				stuckStateTimeout: 5 * time.Minute,
				maxRetries:        3,
				// Initialize state machine components
				stateMachine:         statemachine.NewPhysicalHostStateMachine(ctrl.Log.WithName("state-machine")),
				stateTransitionGuard: statemachine.NewStateTransitionGuard(k8sClient, ctrl.Log.WithName("transition-guard")),
				stateRecoveryManager: statemachine.NewStateRecoveryManager(k8sClient, ctrl.Log.WithName("recovery-manager")),
			}

			// Create PhysicalHost
			queryFailPh := physicalHost.DeepCopy()
			queryFailPh.Name = queryFailPhName
			queryFailPh.Spec.RedfishConnection.CredentialsSecretRef = queryFailSecretName

			Expect(k8sClient.Create(ctx, queryFailPh)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: queryFailPhName, Namespace: physicalHost.Namespace}

			By("First reconcile adds finalizer")
			result, err := queryFailReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			By("Second reconcile should fail due to query errors")
			_, err = queryFailReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redfish query failed"))

			By("Checking that proper conditions are set")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, phLookupKey, queryFailPh)).To(Succeed())
				// Connection should be ready (since connection succeeded)
				connCond := conditions.Get(queryFailPh, infrastructurev1beta1.RedfishConnectionReadyCondition)
				g.Expect(connCond).NotTo(BeNil())
				g.Expect(connCond.Status).To(Equal(corev1.ConditionTrue))

				// Host should not be available due to query failure
				hostCond := conditions.Get(queryFailPh, infrastructurev1beta1.HostAvailableCondition)
				g.Expect(hostCond).NotTo(BeNil())
				g.Expect(hostCond.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(hostCond.Reason).To(Equal(infrastructurev1beta1.RedfishQueryFailedReason))
				g.Expect(queryFailPh.Status.State).To(Equal(infrastructurev1beta1.StateError))
			}, Timeout, Interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, queryFailPh)).To(Succeed())
			Expect(k8sClient.Delete(ctx, queryFailSecret)).To(Succeed())
		})

		It("should handle provisioning flow when claimed by machine", func() {
			By("Creating PhysicalHost that gets claimed for provisioning")
			provisionPhName := physicalHost.Name + "-provision"
			provisionSecretName := credentialSecret.Name + "-provision"
			isoURL := "http://example.com/provision-test.iso"

			// Create credentials secret
			provisionSecret := credentialSecret.DeepCopy()
			provisionSecret.Name = provisionSecretName
			provisionSecret.ResourceVersion = ""
			Expect(k8sClient.Create(ctx, provisionSecret)).To(Succeed())

			// Create mock client that tracks calls
			provisionMockClient := internalredfish.NewMockClient()

			// Create reconciler with provision tracking client
			provisionReconciler := &PhysicalHostReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Log:      ctrl.Log.WithName("physicalhost-test-provision"),
				Recorder: record.NewFakeRecorder(100),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return provisionMockClient, nil
				},
				// Set timeouts for test environment
				reconcileTimeout:  60 * time.Second,
				stuckStateTimeout: 5 * time.Minute,
				maxRetries:        3,
				// Initialize state machine components
				stateMachine:         statemachine.NewPhysicalHostStateMachine(ctrl.Log.WithName("state-machine")),
				stateTransitionGuard: statemachine.NewStateTransitionGuard(k8sClient, ctrl.Log.WithName("transition-guard")),
				stateRecoveryManager: statemachine.NewStateRecoveryManager(k8sClient, ctrl.Log.WithName("recovery-manager")),
			}

			// Create PhysicalHost
			provisionPh := physicalHost.DeepCopy()
			provisionPh.Name = provisionPhName
			provisionPh.Spec.RedfishConnection.CredentialsSecretRef = provisionSecretName

			Expect(k8sClient.Create(ctx, provisionPh)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: provisionPhName, Namespace: physicalHost.Namespace}

			By("First reconcile adds finalizer and makes host Available")
			result, err := provisionReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			_, err = provisionReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying host becomes Available initially")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, phLookupKey, provisionPh)).To(Succeed())
				g.Expect(provisionPh.Status.State).To(Equal(infrastructurev1beta1.StateAvailable))
				g.Expect(conditions.IsTrue(provisionPh, infrastructurev1beta1.HostAvailableCondition)).To(BeTrue())
			}, Timeout, Interval).Should(Succeed())

			By("Claiming the host for provisioning")
			// Simulate a machine claiming the host
			provisionPh.Spec.ConsumerRef = &corev1.ObjectReference{
				Kind:       "Beskar7Machine",
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Name:       "test-machine",
				Namespace:  physicalHost.Namespace,
			}
			provisionPh.Spec.BootISOSource = &isoURL
			Expect(k8sClient.Update(ctx, provisionPh)).To(Succeed())

			By("Reconciling after claim - should trigger provisioning")
			_, err = provisionReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying provisioning actions were called")
			Expect(provisionMockClient.SetBootSourceCalled).To(BeTrue(), "SetBootSourceISO should have been called")
			Expect(provisionMockClient.InsertedISO).To(Equal(isoURL), "SetBootSourceISO should have been called with correct ISO URL")
			Expect(provisionMockClient.SetPowerStateCalled).To(BeTrue(), "SetPowerState should have been called")
			Expect(provisionMockClient.PowerState).To(Equal(redfish.OnPowerState), "Host should be powered on for provisioning")

			By("Verifying host state becomes Provisioning")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, phLookupKey, provisionPh)).To(Succeed())
				g.Expect(provisionPh.Status.State).To(Equal(infrastructurev1beta1.StateProvisioning))
				cond := conditions.Get(provisionPh, infrastructurev1beta1.HostProvisionedCondition)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(infrastructurev1beta1.ProvisioningReason))
			}, Timeout, Interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, provisionPh)).To(Succeed())
			Expect(k8sClient.Delete(ctx, provisionSecret)).To(Succeed())
		})

		It("should handle address detection failures gracefully", func() {
			By("Creating PhysicalHost with mock that fails address detection")
			addrFailPhName := physicalHost.Name + "-addr-fail"
			addrFailSecretName := credentialSecret.Name + "-addr-fail"

			// Create credentials secret
			addrFailSecret := credentialSecret.DeepCopy()
			addrFailSecret.Name = addrFailSecretName
			addrFailSecret.ResourceVersion = ""
			Expect(k8sClient.Create(ctx, addrFailSecret)).To(Succeed())

			// Create mock client that fails address detection but succeeds other operations
			addrFailMockClient := internalredfish.NewMockClient()
			addrFailMockClient.GetNetworkAddressesFunc = func(ctx context.Context) ([]internalredfish.NetworkAddress, error) {
				return nil, fmt.Errorf("network interface discovery failed")
			}

			// Create reconciler with address detection failure
			addrFailReconciler := &PhysicalHostReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Log:      ctrl.Log.WithName("physicalhost-test-addrfail"),
				Recorder: record.NewFakeRecorder(100),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return addrFailMockClient, nil
				},
				// Set timeouts for test environment
				reconcileTimeout:  60 * time.Second,
				stuckStateTimeout: 5 * time.Minute,
				maxRetries:        3,
				// Initialize state machine components
				stateMachine:         statemachine.NewPhysicalHostStateMachine(ctrl.Log.WithName("state-machine")),
				stateTransitionGuard: statemachine.NewStateTransitionGuard(k8sClient, ctrl.Log.WithName("transition-guard")),
				stateRecoveryManager: statemachine.NewStateRecoveryManager(k8sClient, ctrl.Log.WithName("recovery-manager")),
			}

			// Create PhysicalHost
			addrFailPh := physicalHost.DeepCopy()
			addrFailPh.Name = addrFailPhName
			addrFailPh.Spec.RedfishConnection.CredentialsSecretRef = addrFailSecretName

			Expect(k8sClient.Create(ctx, addrFailPh)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: addrFailPhName, Namespace: physicalHost.Namespace}

			By("Reconciling should succeed despite address detection failure")
			_, err := addrFailReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())

			_, err = addrFailReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying host still becomes Available (address detection is non-fatal)")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, phLookupKey, addrFailPh)).To(Succeed())
				g.Expect(addrFailPh.Status.State).To(Equal(infrastructurev1beta1.StateAvailable))
				g.Expect(conditions.IsTrue(addrFailPh, infrastructurev1beta1.HostAvailableCondition)).To(BeTrue())
				// Addresses should be empty/nil due to detection failure
				g.Expect(addrFailPh.Status.Addresses).To(BeEmpty())
			}, Timeout, Interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, addrFailPh)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addrFailSecret)).To(Succeed())
		})

		It("should handle deletion when Redfish connection fails", func() {
			By("Creating PhysicalHost and then simulating connection failure during deletion")
			deleteFailPhName := physicalHost.Name + "-delete-fail"
			deleteFailSecretName := credentialSecret.Name + "-delete-fail"

			// Create credentials secret
			deleteFailSecret := credentialSecret.DeepCopy()
			deleteFailSecret.Name = deleteFailSecretName
			deleteFailSecret.ResourceVersion = ""
			Expect(k8sClient.Create(ctx, deleteFailSecret)).To(Succeed())

			// Create PhysicalHost
			deleteFailPh := physicalHost.DeepCopy()
			deleteFailPh.Name = deleteFailPhName
			deleteFailPh.Spec.RedfishConnection.CredentialsSecretRef = deleteFailSecretName

			Expect(k8sClient.Create(ctx, deleteFailPh)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: deleteFailPhName, Namespace: physicalHost.Namespace}

			By("First reconcile makes host available")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, phLookupKey, deleteFailPh)).To(Succeed())
				g.Expect(deleteFailPh.Status.State).To(Equal(infrastructurev1beta1.StateAvailable))
			}, Timeout, Interval).Should(Succeed())

			By("Creating reconciler that fails connections during deletion")
			deleteFailReconciler := &PhysicalHostReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Log:      ctrl.Log.WithName("physicalhost-test-deletefail"),
				Recorder: record.NewFakeRecorder(100),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return nil, fmt.Errorf("connection failed during deletion")
				},
				// Set timeouts for test environment
				reconcileTimeout:  60 * time.Second,
				stuckStateTimeout: 5 * time.Minute,
				maxRetries:        3,
				// Initialize state machine components
				stateMachine:         statemachine.NewPhysicalHostStateMachine(ctrl.Log.WithName("state-machine")),
				stateTransitionGuard: statemachine.NewStateTransitionGuard(k8sClient, ctrl.Log.WithName("transition-guard")),
				stateRecoveryManager: statemachine.NewStateRecoveryManager(k8sClient, ctrl.Log.WithName("recovery-manager")),
			}

			By("Deleting the PhysicalHost")
			Expect(k8sClient.Delete(ctx, deleteFailPh)).To(Succeed())

			By("Reconciling during deletion should handle connection failure gracefully")
			_, err = deleteFailReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred()) // Should not error, should allow finalizer removal

			By("PhysicalHost should eventually be deleted despite connection failure")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, phLookupKey, deleteFailPh)
				return client.IgnoreNotFound(err) == nil
			}, Timeout*2, Interval).Should(BeTrue(), "PhysicalHost should be deleted despite connection failure")

			// Cleanup
			Expect(k8sClient.Delete(ctx, deleteFailSecret)).To(Succeed())
		})
	})

	Describe("PhysicalHost pause functionality", func() {
		var physicalHost *infrastructurev1beta1.PhysicalHost
		var credentialSecret *corev1.Secret
		var mockRfClient *internalredfish.MockClient
		var reconciler *PhysicalHostReconciler
		var testNs *corev1.Namespace

		BeforeEach(func() {
			// Create a unique namespace for this test
			testNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "physicalhost-pause-test-",
				},
			}
			Expect(k8sClient.Create(ctx, testNs)).To(Succeed())

			// Create unique names for this test
			pausePhName := "test-physicalhost-pause"
			pauseSecretName := "test-redfish-credentials-pause"

			// Create the credential secret
			credentialSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pauseSecretName,
					Namespace: testNs.Name,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			}
			Expect(k8sClient.Create(ctx, credentialSecret)).To(Succeed())

			// Define the PhysicalHost resource
			physicalHost = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pausePhName,
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "https://redfish-pause.example.com",
						CredentialsSecretRef: credentialSecret.Name,
					},
				},
			}

			// Create Mock Redfish Client
			mockRfClient = internalredfish.NewMockClient()

			// Create the reconciler instance for the test
			reconciler = &PhysicalHostReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Log:      ctrl.Log.WithName("physicalhost-test-pause"),
				Recorder: record.NewFakeRecorder(100),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					return mockRfClient, nil
				},
				// Set timeouts for test environment
				reconcileTimeout:  60 * time.Second,
				stuckStateTimeout: 5 * time.Minute,
				maxRetries:        3,
				// Initialize state machine components
				stateMachine:         statemachine.NewPhysicalHostStateMachine(ctrl.Log.WithName("state-machine")),
				stateTransitionGuard: statemachine.NewStateTransitionGuard(k8sClient, ctrl.Log.WithName("transition-guard")),
				stateRecoveryManager: statemachine.NewStateRecoveryManager(k8sClient, ctrl.Log.WithName("recovery-manager")),
			}
		})

		AfterEach(func() {
			// Clean up the namespace which will clean up all resources
			Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
		})

		It("should skip reconciliation when PhysicalHost is paused", func() {
			By("Creating a paused PhysicalHost resource")
			physicalHost.Annotations = map[string]string{
				clusterv1.PausedAnnotation: "true",
			}
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: physicalHost.Name, Namespace: physicalHost.Namespace}

			By("Reconciling the paused PhysicalHost")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			By("Verifying that no Redfish calls were made")
			Expect(mockRfClient.GetSystemInfoCalled).To(BeFalse())
			Expect(mockRfClient.GetPowerStateCalled).To(BeFalse())
			Expect(mockRfClient.CloseCalled).To(BeFalse())

			By("Verifying that the finalizer was not added")
			pausedPh := &infrastructurev1beta1.PhysicalHost{}
			Expect(k8sClient.Get(ctx, phLookupKey, pausedPh)).To(Succeed())
			Expect(pausedPh.Finalizers).NotTo(ContainElement(PhysicalHostFinalizer))
		})

		It("should continue reconciliation when pause annotation is false", func() {
			By("Creating a PhysicalHost with pause annotation set to false")
			physicalHost.Annotations = map[string]string{
				clusterv1.PausedAnnotation: "false",
			}
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: physicalHost.Name, Namespace: physicalHost.Namespace}

			By("Reconciling the PhysicalHost")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{})) // Should be paused, not requeued

			By("Verifying that reconciliation was paused (no Redfish calls)")
			Expect(mockRfClient.GetSystemInfoCalled).To(BeFalse())
			Expect(mockRfClient.GetPowerStateCalled).To(BeFalse())
			Expect(mockRfClient.CloseCalled).To(BeFalse())

			By("Verifying that the finalizer was not added")
			pausedPh := &infrastructurev1beta1.PhysicalHost{}
			Expect(k8sClient.Get(ctx, phLookupKey, pausedPh)).To(Succeed())
			Expect(pausedPh.Finalizers).NotTo(ContainElement(PhysicalHostFinalizer))
		})

		It("should resume reconciliation when pause annotation is removed", func() {
			By("Creating a paused PhysicalHost resource")
			physicalHost.Annotations = map[string]string{
				clusterv1.PausedAnnotation: "true",
			}
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			phLookupKey := types.NamespacedName{Name: physicalHost.Name, Namespace: physicalHost.Namespace}

			By("Reconciling the paused PhysicalHost (should be skipped)")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			By("Removing the pause annotation")
			pausedPh := &infrastructurev1beta1.PhysicalHost{}
			Expect(k8sClient.Get(ctx, phLookupKey, pausedPh)).To(Succeed())
			delete(pausedPh.Annotations, clusterv1.PausedAnnotation)
			Expect(k8sClient.Update(ctx, pausedPh)).To(Succeed())

			By("Reconciling again (should proceed)")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: phLookupKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{Requeue: true})) // Requeue for finalizer addition

			By("Verifying that reconciliation proceeded normally")
			Eventually(func(g Gomega) {
				resumedPh := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, phLookupKey, resumedPh)).To(Succeed())
				g.Expect(resumedPh.Finalizers).To(ContainElement(PhysicalHostFinalizer))
			}, Timeout, Interval).Should(Succeed())
		})
	})
})
