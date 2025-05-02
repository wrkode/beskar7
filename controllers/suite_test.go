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
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	// === Setup Scheme FIRST ===
	// Add Beskar7 types to scheme
	Expect(infrastructurev1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	// Add CAPI types to scheme
	Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	//+kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"), // Local CRDs
			// Absolute path to CAPI CRDs from go list command
			"/Users/wrizzo/go/pkg/mod/sigs.k8s.io/cluster-api@v1.10.1/config/crd/bases",
		},
		// Remove CRDInstallOptions as we use CRDDirectoryPaths explicitly
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// === Scheme setup can remain here or move back, order doesn't matter as much now ===

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Setup manager and reconcilers for integration tests (optional, usually done per-spec)
	// k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
	// 	Scheme: scheme.Scheme,
	// })
	// Expect(err).ToNot(HaveOccurred())

	// err = (&PhysicalHostReconciler{
	// 	Client: k8sManager.GetClient(),
	// 	Scheme: k8sManager.GetScheme(),
	// }).SetupWithManager(k8sManager)
	// Expect(err).ToNot(HaveOccurred())
	// ... add other reconcilers

	// go func() {
	// 	defer GinkgoRecover()
	// 	err = k8sManager.Start(ctx)
	// 	Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	// }()

})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	Expect(testEnv.Stop()).To(Succeed())
})
