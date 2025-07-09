package webhooks

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

func TestBeskar7MachineTemplateWebhook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Beskar7MachineTemplate Webhook Suite")
}

var _ = Describe("Beskar7MachineTemplate Webhook", func() {
	var webhook *Beskar7MachineTemplateWebhook
	var ctx context.Context

	BeforeEach(func() {
		webhook = &Beskar7MachineTemplateWebhook{}
		ctx = context.Background()
	})

	Describe("ValidateCreate", func() {
		It("should accept valid Beskar7MachineTemplate", func() {
			template := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineTemplateSpec{
					Template: infrav1beta1.Beskar7MachineTemplateResource{
						Spec: infrav1beta1.Beskar7MachineSpec{
							ImageURL:         "https://example.com/image.iso",
							OSFamily:         "kairos",
							ProvisioningMode: "RemoteConfig",
							ConfigURL:        "https://example.com/config.yaml",
						},
					},
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, template)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject template with providerID set", func() {
			providerID := "test-provider-id"
			template := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineTemplateSpec{
					Template: infrav1beta1.Beskar7MachineTemplateResource{
						Spec: infrav1beta1.Beskar7MachineSpec{
							ImageURL:         "https://example.com/image.iso",
							OSFamily:         "kairos",
							ProvisioningMode: "RemoteConfig",
							ConfigURL:        "https://example.com/config.yaml",
							ProviderID:       &providerID,
						},
					},
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, template)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("providerID should not be set in machine templates"))
			Expect(warnings).To(BeEmpty())
		})

		It("should reject template with invalid imageURL", func() {
			template := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineTemplateSpec{
					Template: infrav1beta1.Beskar7MachineTemplateResource{
						Spec: infrav1beta1.Beskar7MachineSpec{
							ImageURL:         "invalid-url",
							OSFamily:         "kairos",
							ProvisioningMode: "RemoteConfig",
							ConfigURL:        "https://example.com/config.yaml",
						},
					},
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, template)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject template with invalid OSFamily", func() {
			template := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineTemplateSpec{
					Template: infrav1beta1.Beskar7MachineTemplateResource{
						Spec: infrav1beta1.Beskar7MachineSpec{
							ImageURL:         "https://example.com/image.iso",
							OSFamily:         "invalid-os",
							ProvisioningMode: "RemoteConfig",
							ConfigURL:        "https://example.com/config.yaml",
						},
					},
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, template)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Describe("ValidateUpdate", func() {
		It("should reject update with changed imageURL", func() {
			oldTemplate := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineTemplateSpec{
					Template: infrav1beta1.Beskar7MachineTemplateResource{
						Spec: infrav1beta1.Beskar7MachineSpec{
							ImageURL:         "https://example.com/old-image.iso",
							OSFamily:         "kairos",
							ProvisioningMode: "RemoteConfig",
							ConfigURL:        "https://example.com/config.yaml",
						},
					},
				},
			}

			newTemplate := oldTemplate.DeepCopy()
			newTemplate.Spec.Template.Spec.ImageURL = "https://example.com/new-image.iso"

			warnings, err := webhook.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("imageURL is immutable in machine templates"))
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update with changed OSFamily", func() {
			oldTemplate := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineTemplateSpec{
					Template: infrav1beta1.Beskar7MachineTemplateResource{
						Spec: infrav1beta1.Beskar7MachineSpec{
							ImageURL:         "https://example.com/image.iso",
							OSFamily:         "kairos",
							ProvisioningMode: "RemoteConfig",
							ConfigURL:        "https://example.com/config.yaml",
						},
					},
				},
			}

			newTemplate := oldTemplate.DeepCopy()
			newTemplate.Spec.Template.Spec.OSFamily = "ubuntu"

			warnings, err := webhook.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("osFamily is immutable in machine templates"))
			Expect(warnings).To(BeEmpty())
		})

		It("should accept update with unchanged spec", func() {
			template := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineTemplateSpec{
					Template: infrav1beta1.Beskar7MachineTemplateResource{
						Spec: infrav1beta1.Beskar7MachineSpec{
							ImageURL:         "https://example.com/image.iso",
							OSFamily:         "kairos",
							ProvisioningMode: "RemoteConfig",
							ConfigURL:        "https://example.com/config.yaml",
						},
					},
				},
			}

			newTemplate := template.DeepCopy()
			// Only change metadata, not spec
			newTemplate.Labels = map[string]string{"new": "label"}

			warnings, err := webhook.ValidateUpdate(ctx, template, newTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Describe("Default", func() {
		It("should apply defaults from Beskar7Machine webhook", func() {
			template := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineTemplateSpec{
					Template: infrav1beta1.Beskar7MachineTemplateResource{
						Spec: infrav1beta1.Beskar7MachineSpec{
							ImageURL: "https://example.com/image.iso",
							OSFamily: "kairos",
							// ProvisioningMode not set - should be defaulted
							ConfigURL: "https://example.com/config.yaml",
						},
					},
				},
			}

			err := webhook.Default(ctx, template)
			Expect(err).NotTo(HaveOccurred())

			// The defaults should be applied by the machine webhook
			Expect(template.Spec.Template.Spec.ProvisioningMode).NotTo(BeEmpty())
		})
	})

	Describe("ValidateDelete", func() {
		It("should allow deletion without validation", func() {
			template := &infrav1beta1.Beskar7MachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
			}

			warnings, err := webhook.ValidateDelete(ctx, template)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})
})
