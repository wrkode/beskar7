//go:build integration

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

package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	"github.com/wrkode/beskar7/internal/coordination"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	cfg              *rest.Config
	k8sClient        client.Client
	kubernetesClient kubernetes.Interface
	testEnv          *envtest.Environment
	ctx              context.Context
	cancel           context.CancelFunc
)

func TestConcurrentProvisioningIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Concurrent Provisioning Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	// Setup scheme
	Expect(infrastructurev1beta1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	kubernetesClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(kubernetesClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = Describe("Concurrent Provisioning Integration", func() {
	var (
		testNamespace string
		coordinator   coordination.ClaimCoordinator
	)

	BeforeEach(func() {
		// Create test namespace
		testNamespace = fmt.Sprintf("test-concurrent-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Setup coordinator
		coordinator = coordination.NewHostClaimCoordinator(k8sClient)
	})

	AfterEach(func() {
		// Clean up namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	Context("Basic Host Claiming", func() {
		It("should successfully claim an available host", func() {
			By("Creating an available host")
			host := &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: testNamespace,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish.example.com",
						CredentialsSecretRef: "test-secret",
					},
				},
				Status: infrastructurev1beta1.PhysicalHostStatus{
					State: infrastructurev1beta1.StateAvailable,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			By("Creating a machine")
			machine := &infrastructurev1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: testNamespace,
					UID:       "test-machine-uid",
				},
				Spec: infrastructurev1beta1.Beskar7MachineSpec{
					ImageURL: "http://example.com/image.iso",
					OSFamily: "kairos",
				},
			}
			Expect(k8sClient.Create(ctx, machine)).To(Succeed())

			By("Claiming the host")
			request := coordination.ClaimRequest{
				Machine: machine,
				RequiredSpecs: coordination.HostRequirements{
					RequiredTags: []string{},
				},
			}

			result, err := coordinator.ClaimHost(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ClaimSuccess).To(BeTrue())
			Expect(result.Host.Name).To(Equal("test-host"))

			By("Verifying host is claimed in Kubernetes")
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-host", Namespace: testNamespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
				g.Expect(updatedHost.Spec.ConsumerRef.Name).To(Equal("test-machine"))
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1beta1.StateClaimed))
			}, "5s", "100ms").Should(Succeed())
		})

		It("should handle no available hosts gracefully", func() {
			By("Creating a machine without any available hosts")
			machine := &infrastructurev1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: testNamespace,
					UID:       "test-machine-uid",
				},
				Spec: infrastructurev1beta1.Beskar7MachineSpec{
					ImageURL: "http://example.com/image.iso",
					OSFamily: "kairos",
				},
			}
			Expect(k8sClient.Create(ctx, machine)).To(Succeed())

			By("Attempting to claim a host")
			request := coordination.ClaimRequest{
				Machine: machine,
				RequiredSpecs: coordination.HostRequirements{
					RequiredTags: []string{},
				},
			}

			result, err := coordinator.ClaimHost(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ClaimSuccess).To(BeFalse())
			Expect(result.Retry).To(BeTrue())
			Expect(result.RetryAfter).To(BeNumerically(">", 0))
		})
	})

	Context("Concurrent Host Claims", func() {
		It("should handle multiple concurrent claims without conflicts", func() {
			numHosts := 5
			numMachines := 10

			By("Creating multiple available hosts")
			hosts := make([]*infrastructurev1beta1.PhysicalHost, numHosts)
			for i := 0; i < numHosts; i++ {
				hosts[i] = &infrastructurev1beta1.PhysicalHost{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("host-%d", i),
						Namespace: testNamespace,
					},
					Spec: infrastructurev1beta1.PhysicalHostSpec{
						RedfishConnection: infrastructurev1beta1.RedfishConnection{
							Address:              fmt.Sprintf("redfish%d.example.com", i),
							CredentialsSecretRef: "test-secret",
						},
					},
					Status: infrastructurev1beta1.PhysicalHostStatus{
						State: infrastructurev1beta1.StateAvailable,
					},
				}
				Expect(k8sClient.Create(ctx, hosts[i])).To(Succeed())
			}

			By("Creating multiple machines")
			machines := make([]*infrastructurev1beta1.Beskar7Machine, numMachines)
			for i := 0; i < numMachines; i++ {
				machines[i] = &infrastructurev1beta1.Beskar7Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("machine-%d", i),
						Namespace: testNamespace,
						UID:       types.UID(fmt.Sprintf("machine-uid-%d", i)),
					},
					Spec: infrastructurev1beta1.Beskar7MachineSpec{
						ImageURL: "http://example.com/image.iso",
						OSFamily: "kairos",
					},
				}
				Expect(k8sClient.Create(ctx, machines[i])).To(Succeed())
			}

			By("Running concurrent claims")
			var wg sync.WaitGroup
			results := make([]*coordination.ClaimResult, numMachines)
			errors := make([]error, numMachines)

            for i := 0; i < numMachines; i++ {
                idx := i
                wg.Add(1)
                go func(index int) {
					defer GinkgoRecover()
					defer wg.Done()

					request := coordination.ClaimRequest{
						Machine: machines[index],
						RequiredSpecs: coordination.HostRequirements{
							RequiredTags: []string{},
						},
					}

					results[index], errors[index] = coordinator.ClaimHost(ctx, request)
				}(i)
			}

			wg.Wait()

			By("Verifying claim results")
			successfulClaims := 0
			failedClaims := 0
			claimedHosts := make(map[string]bool)

			for i := 0; i < numMachines; i++ {
				if errors[i] == nil && results[i] != nil && results[i].ClaimSuccess {
					successfulClaims++
					hostName := results[i].Host.Name
					Expect(claimedHosts[hostName]).To(BeFalse(), fmt.Sprintf("Host %s was claimed multiple times", hostName))
					claimedHosts[hostName] = true
				} else {
					failedClaims++
				}
			}

			Expect(successfulClaims).To(Equal(numHosts), "Exactly 5 claims should succeed")
			Expect(failedClaims).To(Equal(numMachines-numHosts), "Exactly 5 claims should fail")

			By("Verifying no double claims in Kubernetes")
			for hostName := range claimedHosts {
				host := &infrastructurev1beta1.PhysicalHost{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: hostName, Namespace: testNamespace}, host)).To(Succeed())
				Expect(host.Spec.ConsumerRef).NotTo(BeNil())
				Expect(host.Status.State).To(Equal(infrastructurev1beta1.StateClaimed))
			}
		})
	})

	Context("Host Release", func() {
		It("should successfully release a claimed host", func() {
			By("Creating and claiming a host")
			host := &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "release-test-host",
					Namespace: testNamespace,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish.example.com",
						CredentialsSecretRef: "test-secret",
					},
					ConsumerRef: &corev1.ObjectReference{
						Name:      "release-test-machine",
						Namespace: testNamespace,
					},
				},
				Status: infrastructurev1beta1.PhysicalHostStatus{
					State: infrastructurev1beta1.StateClaimed,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			machine := &infrastructurev1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "release-test-machine",
					Namespace: testNamespace,
					UID:       "release-test-machine-uid",
				},
				Spec: infrastructurev1beta1.Beskar7MachineSpec{
					ImageURL: "http://example.com/image.iso",
					OSFamily: "kairos",
				},
			}
			Expect(k8sClient.Create(ctx, machine)).To(Succeed())

			By("Releasing the host")
			err := coordinator.ReleaseHost(ctx, machine)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying host is released in Kubernetes")
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "release-test-host", Namespace: testNamespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).To(BeNil())
				g.Expect(updatedHost.Status.State).To(Equal(infrastructurev1beta1.StateAvailable))
			}, "5s", "100ms").Should(Succeed())
		})
	})

	Context("Leader Election Coordination", func() {
		It("should initialize leader election coordinator successfully", func() {
			By("Creating leader election coordinator")
			config := coordination.LeaderElectionConfig{
				Namespace:          testNamespace,
				Identity:           "test-leader",
				LeaseDuration:      5 * time.Second,
				RenewDeadline:      3 * time.Second,
				RetryPeriod:        1 * time.Second,
				ProcessingInterval: 500 * time.Millisecond,
			}

			leaderCoordinator := coordination.NewLeaderElectionClaimCoordinator(
				k8sClient,
				kubernetesClient,
				config,
			)

			Expect(leaderCoordinator).NotTo(BeNil())

			By("Testing interface compliance")
			var claimCoordinator coordination.ClaimCoordinator = leaderCoordinator
			Expect(claimCoordinator).NotTo(BeNil())

			var leaderCapable coordination.LeaderElectionCapable = leaderCoordinator
			Expect(leaderCapable).NotTo(BeNil())

			By("Testing leadership status")
			isLeader, identity := leaderCoordinator.GetLeadershipStatus()
			Expect(identity).To(Equal("test-leader"))
			// Initially should not be leader since election hasn't started
			Expect(isLeader).To(BeFalse())

			By("Testing queue status")
			queueLength, pendingClaims := leaderCoordinator.GetClaimQueueStatus()
			Expect(queueLength).To(Equal(0))
			Expect(pendingClaims).To(Equal(0))
		})

		It("should fall back to optimistic locking when not leader", func() {
			By("Creating resources")
			host := &infrastructurev1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fallback-test-host",
					Namespace: testNamespace,
				},
				Spec: infrastructurev1beta1.PhysicalHostSpec{
					RedfishConnection: infrastructurev1beta1.RedfishConnection{
						Address:              "redfish.example.com",
						CredentialsSecretRef: "test-secret",
					},
				},
				Status: infrastructurev1beta1.PhysicalHostStatus{
					State: infrastructurev1beta1.StateAvailable,
				},
			}
			Expect(k8sClient.Create(ctx, host)).To(Succeed())

			machine := &infrastructurev1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fallback-test-machine",
					Namespace: testNamespace,
					UID:       "fallback-test-machine-uid",
				},
				Spec: infrastructurev1beta1.Beskar7MachineSpec{
					ImageURL: "http://example.com/image.iso",
					OSFamily: "kairos",
				},
			}
			Expect(k8sClient.Create(ctx, machine)).To(Succeed())

			By("Creating leader election coordinator")
			config := coordination.LeaderElectionConfig{
				Namespace: testNamespace,
				Identity:  "fallback-test-coordinator",
			}

			leaderCoordinator := coordination.NewLeaderElectionClaimCoordinator(
				k8sClient,
				kubernetesClient,
				config,
			)

			By("Claiming host when not leader (should fall back)")
			request := coordination.ClaimRequest{
				Machine: machine,
				RequiredSpecs: coordination.HostRequirements{
					RequiredTags: []string{},
				},
			}

			result, err := leaderCoordinator.ClaimHost(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ClaimSuccess).To(BeTrue())
			Expect(result.Host.Name).To(Equal("fallback-test-host"))

			By("Verifying host is claimed")
			Eventually(func(g Gomega) {
				updatedHost := &infrastructurev1beta1.PhysicalHost{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "fallback-test-host", Namespace: testNamespace}, updatedHost)).To(Succeed())
				g.Expect(updatedHost.Spec.ConsumerRef).NotTo(BeNil())
				g.Expect(updatedHost.Spec.ConsumerRef.Name).To(Equal("fallback-test-machine"))
			}, "5s", "100ms").Should(Succeed())
		})
	})

	Context("Stress Testing", func() {
		It("should handle high-concurrency scenarios efficiently", func() {
			numHosts := 20
			numMachines := 40

			By("Creating many hosts")
			for i := 0; i < numHosts; i++ {
				host := &infrastructurev1beta1.PhysicalHost{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("stress-host-%d", i),
						Namespace: testNamespace,
					},
					Spec: infrastructurev1beta1.PhysicalHostSpec{
						RedfishConnection: infrastructurev1beta1.RedfishConnection{
							Address:              fmt.Sprintf("redfish%d.example.com", i),
							CredentialsSecretRef: "test-secret",
						},
					},
					Status: infrastructurev1beta1.PhysicalHostStatus{
						State: infrastructurev1beta1.StateAvailable,
					},
				}
				Expect(k8sClient.Create(ctx, host)).To(Succeed())
			}

			By("Creating many machines")
			machines := make([]*infrastructurev1beta1.Beskar7Machine, numMachines)
			for i := 0; i < numMachines; i++ {
				machines[i] = &infrastructurev1beta1.Beskar7Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("stress-machine-%d", i),
						Namespace: testNamespace,
						UID:       types.UID(fmt.Sprintf("stress-machine-uid-%d", i)),
					},
					Spec: infrastructurev1beta1.Beskar7MachineSpec{
						ImageURL: "http://example.com/stress.iso",
						OSFamily: "kairos",
					},
				}
				Expect(k8sClient.Create(ctx, machines[i])).To(Succeed())
			}

			By("Running high-concurrency claims")
			var wg sync.WaitGroup
			results := make([]*coordination.ClaimResult, numMachines)
			errors := make([]error, numMachines)
			startTime := time.Now()

            for i := 0; i < numMachines; i++ {
                idx := i
                wg.Add(1)
                go func(index int) {
					defer GinkgoRecover()
					defer wg.Done()

					request := coordination.ClaimRequest{
						Machine: machines[index],
						RequiredSpecs: coordination.HostRequirements{
							RequiredTags: []string{},
						},
					}

                    results[index], errors[index] = coordinator.ClaimHost(ctx, request)
                }(idx)
			}

			wg.Wait()
			duration := time.Since(startTime)

			By("Analyzing stress test results")
			successfulClaims := 0
			failedClaims := 0
			claimedHosts := make(map[string]bool)

			for i := 0; i < numMachines; i++ {
				if errors[i] == nil && results[i] != nil && results[i].ClaimSuccess {
					successfulClaims++
					hostName := results[i].Host.Name
					Expect(claimedHosts[hostName]).To(BeFalse(), fmt.Sprintf("Host %s was claimed multiple times", hostName))
					claimedHosts[hostName] = true
				} else {
					failedClaims++
				}
			}

			Expect(successfulClaims).To(Equal(numHosts), fmt.Sprintf("Expected %d successful claims", numHosts))
			Expect(failedClaims).To(Equal(numMachines-numHosts), fmt.Sprintf("Expected %d failed claims", numMachines-numHosts))
			Expect(duration).To(BeNumerically("<", 30*time.Second), "Stress test should complete within 30 seconds")

			By("Verifying data integrity in Kubernetes")
			for hostName := range claimedHosts {
				host := &infrastructurev1beta1.PhysicalHost{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: hostName, Namespace: testNamespace}, host)).To(Succeed())
				Expect(host.Spec.ConsumerRef).NotTo(BeNil())
				Expect(host.Status.State).To(Equal(infrastructurev1beta1.StateClaimed))
			}
		})
	})
})
