package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/util/conditions"

	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
)

var _ = Describe("Security Tests", func() {
	var (
		ctx    context.Context
		testNs *corev1.Namespace
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create a unique namespace for the test
		testNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "security-test-",
			},
		}
		Expect(k8sClient.Create(ctx, testNs)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up the namespace and wait for deletion
		Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: testNs.Name}, &corev1.Namespace{})
		}, "30s", "100ms").ShouldNot(Succeed())
	})

	Context("TLS and Certificate Validation", func() {
		var (
			physicalHost     *infrastructurev1alpha1.PhysicalHost
			credentialSecret *corev1.Secret
			mockRfClient     *internalredfish.MockClient
			reconciler       *PhysicalHostReconciler
			secretName       string
			shouldFailTLS    bool
		)

		BeforeEach(func() {
			// Create credential secret with unique name
			secretName = fmt.Sprintf("test-redfish-credentials-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
			credentialSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNs.Name,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			}
			Expect(k8sClient.Create(ctx, credentialSecret)).To(Succeed())

			// Create physical host
			physicalHost = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-host-%d", GinkgoParallelProcess()),
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "https://redfish-mock.example.com",
						CredentialsSecretRef: secretName,
						InsecureSkipVerify:   ptr.To(false), // Default to secure
					},
				},
			}

			shouldFailTLS = false

			mockRfClient = internalredfish.NewMockClient()
			reconciler = &PhysicalHostReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					if shouldFailTLS && !insecure {
						return nil, fmt.Errorf("TLS certificate validation failed: x509: certificate signed by unknown authority")
					}
					mockRfClient.Insecure = insecure
					return mockRfClient, nil
				},
			}
		})

		AfterEach(func() {
			// Delete physical host and wait for deletion
			if physicalHost != nil {
				Expect(k8sClient.Delete(ctx, physicalHost)).To(Succeed())
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      physicalHost.Name,
						Namespace: testNs.Name,
					}, &infrastructurev1alpha1.PhysicalHost{})
				}, "30s", "100ms").Should(HaveOccurred())
			}

			// Delete credential secret and wait for deletion
			if credentialSecret != nil {
				Expect(k8sClient.Delete(ctx, credentialSecret)).To(Succeed())
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      secretName,
						Namespace: testNs.Name,
					}, &corev1.Secret{})
				}, "30s", "100ms").Should(HaveOccurred())
			}
		})

		It("should reject invalid TLS certificates by default", func() {
			By("Creating a PhysicalHost with an invalid certificate")
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			// Simulate TLS error in mock client
			shouldFailTLS = true

			// Reconcile should not return an error, but update status
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      physicalHost.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch the PhysicalHost before assertions
			updatedHost := &infrastructurev1alpha1.PhysicalHost{}
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      physicalHost.Name,
					Namespace: testNs.Name,
				}, updatedHost)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updatedHost.Status.ErrorMessage).To(ContainSubstring("certificate"))
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1alpha1.StateError))
				g.Expect(conditions.IsFalse(updatedHost, infrastructurev1alpha1.RedfishConnectionReadyCondition)).To(BeTrue())
				g.Expect(conditions.GetReason(updatedHost, infrastructurev1alpha1.RedfishConnectionReadyCondition)).To(Equal(TLSErrorReason))
			}, "10s", "100ms").Should(Succeed())
		})

		It("should allow insecure connections when explicitly configured", func() {
			By("Creating a PhysicalHost with InsecureSkipVerify enabled")
			physicalHost.Spec.RedfishConnection.InsecureSkipVerify = ptr.To(true)
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			// Simulate TLS error in mock client, but insecure is true so it should not fail
			shouldFailTLS = true

			// Reconcile should succeed
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      physicalHost.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch the PhysicalHost before assertions
			updatedHost := &infrastructurev1alpha1.PhysicalHost{}
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      physicalHost.Name,
					Namespace: testNs.Name,
				}, updatedHost)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updatedHost.Status.ErrorMessage).To(BeEmpty())
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1alpha1.StateAvailable))
				g.Expect(conditions.IsTrue(updatedHost, infrastructurev1alpha1.RedfishConnectionReadyCondition)).To(BeTrue())
			}, "10s", "100ms").Should(Succeed())
		})
	})

	Context("Credential Handling", func() {
		var (
			physicalHost     *infrastructurev1alpha1.PhysicalHost
			credentialSecret *corev1.Secret
			mockRfClient     *internalredfish.MockClient
			reconciler       *PhysicalHostReconciler
			secretName       string
		)

		BeforeEach(func() {
			// Create credential secret with unique name
			secretName = fmt.Sprintf("test-redfish-credentials-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
			credentialSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNs.Name,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			}
			Expect(k8sClient.Create(ctx, credentialSecret)).To(Succeed())

			// Create physical host
			physicalHost = &infrastructurev1alpha1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-host-%d", GinkgoParallelProcess()),
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1alpha1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1alpha1.RedfishConnectionInfo{
						Address:              "https://redfish-mock.example.com",
						CredentialsSecretRef: secretName,
					},
				},
			}

			mockRfClient = internalredfish.NewMockClient()
			reconciler = &PhysicalHostReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RedfishClientFactory: func(ctx context.Context, address, username, password string, insecure bool) (internalredfish.Client, error) {
					if username == "invalid" || password == "invalid" {
						return nil, fmt.Errorf("authentication failed: invalid credentials")
					}
					return mockRfClient, nil
				},
			}
		})

		AfterEach(func() {
			// Delete physical host and wait for deletion
			if physicalHost != nil {
				Expect(k8sClient.Delete(ctx, physicalHost)).To(Succeed())
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      physicalHost.Name,
						Namespace: testNs.Name,
					}, &infrastructurev1alpha1.PhysicalHost{})
				}, "30s", "100ms").Should(HaveOccurred())
			}

			// Delete credential secret and wait for deletion
			if credentialSecret != nil {
				Expect(k8sClient.Delete(ctx, credentialSecret)).To(Succeed())
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      secretName,
						Namespace: testNs.Name,
					}, &corev1.Secret{})
				}, "30s", "100ms").Should(HaveOccurred())
			}
		})

		It("should handle missing credentials secret", func() {
			By("Creating a PhysicalHost without creating the secret")
			Expect(k8sClient.Delete(ctx, credentialSecret)).To(Succeed())
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			// Reconcile should not return an error, but update status
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      physicalHost.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).NotTo(HaveOccurred())

			// Check that the host status reflects the error
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      physicalHost.Name,
					Namespace: testNs.Name,
				}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1alpha1.StateError))
				g.Expect(updatedHost.Status.ErrorMessage).To(ContainSubstring("secret"))
				g.Expect(conditions.IsFalse(updatedHost, infrastructurev1alpha1.RedfishConnectionReadyCondition)).To(BeTrue())
				g.Expect(conditions.GetReason(updatedHost, infrastructurev1alpha1.RedfishConnectionReadyCondition)).To(Equal(infrastructurev1alpha1.SecretNotFoundReason))
			}, "30s", "100ms").Should(Succeed())
		})

		It("should handle invalid credentials", func() {
			By("Creating a PhysicalHost with invalid credentials")
			credentialSecret.Data["username"] = []byte("invalid")
			credentialSecret.Data["password"] = []byte("invalid")
			Expect(k8sClient.Update(ctx, credentialSecret)).To(Succeed())
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			// Reconcile should not return an error, but update status
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      physicalHost.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).NotTo(HaveOccurred())

			// Check that the host status reflects the error
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      physicalHost.Name,
					Namespace: testNs.Name,
				}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1alpha1.StateError))
				g.Expect(updatedHost.Status.ErrorMessage).To(ContainSubstring("authentication"))
				g.Expect(conditions.IsFalse(updatedHost, infrastructurev1alpha1.RedfishConnectionReadyCondition)).To(BeTrue())
				g.Expect(conditions.GetReason(updatedHost, infrastructurev1alpha1.RedfishConnectionReadyCondition)).To(Equal(InvalidCredentialsReason))
			}, "30s", "100ms").Should(Succeed())
		})

		It("should handle credential rotation", func() {
			// TODO: Future test implementation
			Skip("Skipping credential rotation test for now")
			By("Creating a PhysicalHost with initial credentials")
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())

			// First reconcile should succeed
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      physicalHost.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).NotTo(HaveOccurred())

			// Update credentials
			credentialSecret.Data["username"] = []byte("rotated")
			credentialSecret.Data["password"] = []byte("rotated")
			Expect(k8sClient.Update(ctx, credentialSecret)).To(Succeed())

			// Reconcile should succeed with new credentials
			_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      physicalHost.Name,
				Namespace: testNs.Name,
			}})
			Expect(err).NotTo(HaveOccurred())

			// Check that the host remains available
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1alpha1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      physicalHost.Name,
					Namespace: testNs.Name,
				}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1alpha1.StateAvailable))
			}, "30s", "100ms").Should(Succeed())
		})
	})
})
