package controllers

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

var _ = Describe("Beskar7Machine Controller", func() {

	const (
		Timeout  = time.Second * 10
		Interval = time.Millisecond * 250
	)

	Context("When reconciling a Beskar7Machine", func() {
		var beskar7Machine *infrastructurev1beta1.Beskar7Machine
		var physicalHost *infrastructurev1beta1.PhysicalHost
		var credentialSecret *corev1.Secret
		var reconciler *Beskar7MachineReconciler
		var testNs *corev1.Namespace
		var capiCluster *clusterv1.Cluster
		var capiMachine *clusterv1.Machine

		BeforeEach(func() {
			// Create unique namespace
			testNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "beskar7machine-test-",
				},
			}
			Expect(k8sClient.Create(ctx, testNs)).To(Succeed())

			// Create CAPI Cluster
			capiCluster = &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: testNs.Name,
				},
				Spec: clusterv1.ClusterSpec{},
			}
			Expect(k8sClient.Create(ctx, capiCluster)).To(Succeed())

			// Create credential secret
			credentialSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bmc-creds",
					Namespace: testNs.Name,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("password"),
				},
			}
			Expect(k8sClient.Create(ctx, credentialSecret)).To(Succeed())

			// Create available PhysicalHost
			physicalHost = &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "https://192.168.1.100",
						CredentialsSecretRef: credentialSecret.Name,
					},
				},
				Status: infrastructurev1beta1.PhysicalHostStatus{
					State: infrastructurev1beta1.StateAvailable,
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, physicalHost)).To(Succeed())
			Expect(k8sClient.Status().Update(ctx, physicalHost)).To(Succeed())

			// Create CAPI Machine first
			capiMachine = &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: testNs.Name,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: capiCluster.Name,
					},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: capiCluster.Name,
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: infrastructurev1beta1.GroupVersion.String(),
						Kind:       "Beskar7Machine",
						Name:       "test-machine",
						Namespace:  testNs.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, capiMachine)).To(Succeed())

			// Create Beskar7Machine with owner reference
			beskar7Machine = &infrastructurev1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
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
					InspectionImageURL: "http://boot-server/ipxe/inspect.ipxe",
					TargetImageURL:     "http://boot-server/images/kairos.tar.gz",
				},
			}

			// Create reconciler
			reconciler = &Beskar7MachineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Log:    ctrl.Log.WithName("beskar7machine-test"),
			}
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
		})

		It("Should successfully claim an available PhysicalHost [with CAPI setup]", func() {
			By("Creating the Beskar7Machine")
			Expect(k8sClient.Create(ctx, beskar7Machine)).To(Succeed())

			machineLookupKey := types.NamespacedName{Name: beskar7Machine.Name, Namespace: beskar7Machine.Namespace}

			By("Reconciling adds finalizer first")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: machineLookupKey})
			Expect(err).NotTo(HaveOccurred())

			By("Second reconciliation claims host")
			_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: machineLookupKey})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying PhysicalHost was claimed")
			Eventually(func(g Gomega) {
				claimedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: physicalHost.Name, Namespace: physicalHost.Namespace}, claimedHost)).To(Succeed())
				g.Expect(claimedHost.Spec.ConsumerRef).NotTo(BeNil())
				g.Expect(claimedHost.Spec.ConsumerRef.Name).To(Equal(beskar7Machine.Name))
				g.Expect(claimedHost.Status.State).To(Equal(infrastructurev1beta1.StateInUse))
			}, Timeout, Interval).Should(Succeed())
		})

		It("Should handle pause annotation", func() {
			By("Creating paused machine")
			beskar7Machine.Annotations = map[string]string{
				clusterv1.PausedAnnotation: "true",
			}
			Expect(k8sClient.Create(ctx, beskar7Machine)).To(Succeed())

			machineLookupKey := types.NamespacedName{Name: beskar7Machine.Name, Namespace: beskar7Machine.Namespace}

			By("Reconciling paused machine")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: machineLookupKey})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying no host was claimed")
			Consistently(func(g Gomega) {
				host := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: physicalHost.Name, Namespace: physicalHost.Namespace}, host)).To(Succeed())
				g.Expect(host.Spec.ConsumerRef).To(BeNil())
				g.Expect(host.Status.State).To(Equal(infrastructurev1beta1.StateAvailable))
			}, "2s", "100ms").Should(Succeed())
		})
	})
})

