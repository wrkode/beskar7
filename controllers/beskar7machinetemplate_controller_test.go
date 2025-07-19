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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

var _ = Describe("Beskar7MachineTemplate Controller", func() {
	var (
		ctx        context.Context
		testNs     *corev1.Namespace
		template   *infrastructurev1beta1.Beskar7MachineTemplate
		key        types.NamespacedName
		reconciler *Beskar7MachineTemplateReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create a namespace for the test
		testNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "b7template-reconciler-",
			},
		}
		Expect(k8sClient.Create(ctx, testNs)).To(Succeed())

		// Basic Beskar7MachineTemplate object
		template = &infrastructurev1beta1.Beskar7MachineTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-template",
				Namespace: testNs.Name,
			},
			Spec: infrastructurev1beta1.Beskar7MachineTemplateSpec{
				Template: infrastructurev1beta1.Beskar7MachineTemplateResource{
					Spec: infrastructurev1beta1.Beskar7MachineSpec{
						ImageURL:         "http://example.com/test.iso",
						OSFamily:         "kairos",
						ProvisioningMode: "RemoteConfig",
						ConfigURL:        "http://example.com/config.yaml",
					},
				},
			},
		}
		key = types.NamespacedName{Name: template.Name, Namespace: template.Namespace}

		// Initialize reconciler
		reconciler = &Beskar7MachineTemplateReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
			Log:    ctrl.Log.WithName("controllers").WithName("Beskar7MachineTemplate"),
		}
	})

	AfterEach(func() {
		// Clean up the namespace
		Expect(k8sClient.Delete(ctx, testNs)).To(Succeed())
	})

	Context("Reconcile Normal", func() {
		It("should add finalizer and validate template", func() {
			By("Creating the template")
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("First reconcile - should add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Checking finalizer is added")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, template)).To(Succeed())
				g.Expect(template.Finalizers).To(ContainElement(Beskar7MachineTemplateFinalizer))
			}, "5s", "100ms").Should(Succeed())

			By("Second reconcile - should complete successfully")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue after successful reconcile")
			Expect(result.RequeueAfter).To(BeZero(), "Should not requeue after successful reconcile")
		})

		It("should fail validation for invalid template", func() {
			By("Creating template with missing ImageURL")
			invalidTemplate := template.DeepCopy()
			invalidTemplate.Spec.Template.Spec.ImageURL = ""
			Expect(k8sClient.Create(ctx, invalidTemplate)).To(Succeed())

			By("First reconcile - should add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Second reconcile - should fail validation")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Should return invalid error for missing ImageURL")
		})

		It("should successfully reconcile a valid template", func() {
			By("Creating template with valid OSFamily")
			validTemplate := template.DeepCopy()
			validTemplate.Spec.Template.Spec.OSFamily = "kairos" // Valid OSFamily
			validTemplate.Name = "test-template-valid-os"
			validKey := types.NamespacedName{Name: validTemplate.Name, Namespace: validTemplate.Namespace}
			Expect(k8sClient.Create(ctx, validTemplate)).To(Succeed())

			By("First reconcile - should add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: validKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Second reconcile - should complete successfully")
			result, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: validKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue after successful reconciliation")
		})
	})

	Context("Reconcile Delete", func() {
		It("should remove finalizer when no machines reference the template", func() {
			By("Creating template with finalizer")
			template.Finalizers = []string{Beskar7MachineTemplateFinalizer}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("Deleting the template")
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())

			By("Reconciling after deletion")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue after finalizer removal")

			By("Checking template is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, template)
				return apierrors.IsNotFound(err)
			}, "10s", "200ms").Should(BeTrue(), "Template should be deleted")
		})

		It("should wait for machines to be cleaned up before allowing deletion", func() {
			By("Creating template with finalizer")
			template.Finalizers = []string{Beskar7MachineTemplateFinalizer}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("Creating a machine that references the template")
			machine := &infrastructurev1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: testNs.Name,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "Beskar7MachineTemplate",
							Name:       template.Name,
							UID:        template.UID,
						},
					},
				},
				Spec: infrastructurev1beta1.Beskar7MachineSpec{
					ImageURL: "http://example.com/test.iso",
					OSFamily: "kairos",
				},
			}
			Expect(k8sClient.Create(ctx, machine)).To(Succeed())

			By("Deleting the template")
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())

			By("Reconciling after deletion - should fail due to referencing machine")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsConflict(err)).To(BeTrue(), "Should return conflict error when machines still reference template")

			By("Deleting the referencing machine")
			Expect(k8sClient.Delete(ctx, machine)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: machine.Name, Namespace: machine.Namespace}, machine)
				return apierrors.IsNotFound(err)
			}, "10s", "200ms").Should(BeTrue(), "Machine should be deleted")

			By("Reconciling after machine deletion - should succeed")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue after successful cleanup")

			By("Checking template is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, template)
				return apierrors.IsNotFound(err)
			}, "10s", "200ms").Should(BeTrue(), "Template should be deleted after machine cleanup")
		})
	})

	Context("Machine to Template Mapping", func() {
		It("should map machine events to template reconcile requests", func() {
			By("Creating a machine with template owner reference")
			machine := &infrastructurev1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: testNs.Name,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "Beskar7MachineTemplate",
							Name:       "test-template",
							UID:        "test-uid",
						},
					},
				},
				Spec: infrastructurev1beta1.Beskar7MachineSpec{
					ImageURL: "http://example.com/test.iso",
					OSFamily: "kairos",
				},
			}

			By("Calling the mapping function")
			requests := reconciler.Beskar7MachineToTemplate(ctx, machine)

			By("Verifying the mapping result")
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].NamespacedName).To(Equal(types.NamespacedName{
				Namespace: testNs.Name,
				Name:      "test-template",
			}))
		})

		It("should not map machines without template owner references", func() {
			By("Creating a machine without template owner reference")
			machine := &infrastructurev1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: testNs.Name,
				},
				Spec: infrastructurev1beta1.Beskar7MachineSpec{
					ImageURL: "http://example.com/test.iso",
					OSFamily: "kairos",
				},
			}

			By("Calling the mapping function")
			requests := reconciler.Beskar7MachineToTemplate(ctx, machine)

			By("Verifying no mapping occurs")
			Expect(requests).To(BeEmpty())
		})
	})

	Context("Pause Functionality", func() {
		It("should skip reconciliation when template is paused", func() {
			By("Creating template with pause annotation")
			template.Annotations = map[string]string{
				"cluster.x-k8s.io/paused": "true",
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("Reconciling paused template")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue paused template")

			By("Checking finalizer is not added")
			Consistently(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, template)).To(Succeed())
				g.Expect(template.Finalizers).NotTo(ContainElement(Beskar7MachineTemplateFinalizer))
			}, "2s", "100ms").Should(Succeed())
		})
	})

	Context("Resource Not Found", func() {
		It("should handle missing template gracefully", func() {
			By("Reconciling non-existent template")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue for non-existent template")
		})
	})

	Context("Finalizer Management", func() {
		It("should add finalizer during normal reconciliation", func() {
			By("Creating template without finalizer")
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("Reconciling to add finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "Should requeue after adding finalizer")

			By("Checking finalizer is added")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, template)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(template, Beskar7MachineTemplateFinalizer)).To(BeTrue())
			}, "5s", "100ms").Should(Succeed())
		})

		It("should not add finalizer if already present", func() {
			By("Creating template with finalizer already present")
			template.Finalizers = []string{Beskar7MachineTemplateFinalizer}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("Reconciling template with existing finalizer")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue when finalizer already exists")

			By("Checking finalizer count remains the same")
			Eventually(func(g Gomega) {
				Expect(k8sClient.Get(ctx, key, template)).To(Succeed())
				g.Expect(template.Finalizers).To(HaveLen(1))
				g.Expect(template.Finalizers[0]).To(Equal(Beskar7MachineTemplateFinalizer))
			}, "5s", "100ms").Should(Succeed())
		})
	})
})
