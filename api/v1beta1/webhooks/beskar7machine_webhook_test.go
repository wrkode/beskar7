package webhooks

import (
	"context"
	"fmt"
	"strings"
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
					ImageURL:         "https://example.com/image.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "RemoteConfig",
					ConfigURL:        "https://example.com/config.yaml",
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
					ImageURL:         "ftp://example.com/prebaked.iso",
					OSFamily:         "ubuntu",
					ProvisioningMode: "PreBakedISO",
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should accept file:// URLs", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "file:///path/to/image.qcow2.gz",
					OSFamily:         "debian",
					ProvisioningMode: "RemoteConfig",
					ConfigURL:        "file:///path/to/config.json",
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject invalid ImageURL scheme", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "invalid://example.com/image.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "RemoteConfig",
					ConfigURL:        "https://example.com/config.yaml",
				},
			}

			_, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unsupported value: \"invalid\""))
		})

		It("should reject invalid ImageURL extension", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "https://example.com/notanimage.txt",
					OSFamily:         "kairos",
					ProvisioningMode: "RemoteConfig",
					ConfigURL:        "https://example.com/config.yaml",
				},
			}

			_, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("imageURL should point to a valid image file"))
		})

		It("should reject invalid ConfigURL extension", func() {
			machine := &infrav1beta1.Beskar7Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: "default",
				},
				Spec: infrav1beta1.Beskar7MachineSpec{
					ImageURL:         "https://example.com/image.iso",
					OSFamily:         "kairos",
					ProvisioningMode: "RemoteConfig",
					ConfigURL:        "https://example.com/config.exe",
				},
			}

			_, err := webhook.ValidateCreate(ctx, machine)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configURL should point to a valid configuration file"))
		})

		It("should accept extended OS families", func() {
			validOSFamilies := []string{"kairos", "talos", "flatcar", "LeapMicro", "ubuntu", "rhel", "centos", "fedora", "debian", "opensuse"}

			for _, osFamily := range validOSFamilies {
				machine := &infrav1beta1.Beskar7Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-machine-" + osFamily,
						Namespace: "default",
					},
					Spec: infrav1beta1.Beskar7MachineSpec{
						ImageURL:         "https://example.com/image.iso",
						OSFamily:         osFamily,
						ProvisioningMode: "RemoteConfig",
						ConfigURL:        "https://example.com/config.yaml",
					},
				}

				warnings, err := webhook.ValidateCreate(ctx, machine)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("OS family %s should be valid", osFamily))
				Expect(warnings).To(BeEmpty())
			}
		})

		It("should accept extended provisioning modes", func() {
			validModes := []string{"RemoteConfig", "PreBakedISO", "PXE", "iPXE"}

			for _, mode := range validModes {
				machine := &infrav1beta1.Beskar7Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-machine-" + strings.ToLower(mode),
						Namespace: "default",
					},
					Spec: infrav1beta1.Beskar7MachineSpec{
						ImageURL:         "https://example.com/image.iso",
						OSFamily:         "kairos",
						ProvisioningMode: mode,
					},
				}

				if mode == "RemoteConfig" {
					machine.Spec.ConfigURL = "https://example.com/config.yaml"
				}

				warnings, err := webhook.ValidateCreate(ctx, machine)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Provisioning mode %s should be valid", mode))
				Expect(warnings).To(BeEmpty())
			}
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
