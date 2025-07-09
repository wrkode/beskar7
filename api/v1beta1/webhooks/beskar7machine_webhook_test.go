package webhooks

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

func TestBeskar7MachineWebhook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Beskar7Machine Webhook Suite")
}

var _ = Describe("Beskar7Machine Webhook", func() {
	var webhook *Beskar7MachineWebhook
	var ctx context.Context

	BeforeEach(func() {
		webhook = &Beskar7MachineWebhook{}
		ctx = context.Background()
	})

	Describe("ValidateCreate", func() {
		It("should accept valid Beskar7Machine with RemoteConfig", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "http://example.com/image.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "RemoteConfig",
					ConfigURL:        "http://example.com/config.yaml",
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should accept valid Beskar7Machine with PreBakedISO", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "http://example.com/prebaked.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "PreBakedISO",
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject RemoteConfig without ConfigURL", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "http://example.com/image.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "RemoteConfig",
					// ConfigURL missing
				},
			}

			_, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configURL is required when provisioningMode is RemoteConfig"))
		})

		It("should reject PreBakedISO with ConfigURL", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "http://example.com/prebaked.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "PreBakedISO",
					ConfigURL:        "http://example.com/config.yaml", // Should not be set
				},
			}

			_, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configURL should not be set when provisioningMode is PreBakedISO"))
		})

		It("should reject invalid OSFamily", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "http://example.com/image.iso",
					OSFamily:         "unsupported-os",
					ProvisioningMode: "RemoteConfig",
					ConfigURL:        "http://example.com/config.yaml",
				},
			}

			_, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unsupported value"))
		})

		It("should reject invalid ProvisioningMode", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "http://example.com/image.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "InvalidMode",
					ConfigURL:        "http://example.com/config.yaml",
				},
			}

			_, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unsupported value"))
		})

		It("should reject missing required fields", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					// Missing ImageURL and OSFamily
				},
			}

			_, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("imageURL is required"))
			Expect(err.Error()).To(ContainSubstring("osFamily is required"))
		})
	})

	Describe("Default", func() {
		It("should set default ProvisioningMode", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL: "http://example.com/image.iso",
					OSFamily: "kairos",
					// ProvisioningMode not set
				},
			}

			err := webhook.Default(ctx, machine)
			Expect(err).NotTo(HaveOccurred())
			Expect(machine.Spec.ProvisioningMode).To(Equal("RemoteConfig"))
		})

		It("should not override existing ProvisioningMode", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "http://example.com/prebaked.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "PreBakedISO",
				},
			}

			err := webhook.Default(ctx, machine)
			Expect(err).NotTo(HaveOccurred())
			Expect(machine.Spec.ProvisioningMode).To(Equal("PreBakedISO"))
		})
	})
})
