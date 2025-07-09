package webhooks

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

func TestPhysicalHostWebhook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PhysicalHost Webhook Suite")
}

var _ = Describe("PhysicalHost Webhook", func() {
	var webhook *PhysicalHostWebhook
	var ctx context.Context

	BeforeEach(func() {
		webhook = &PhysicalHostWebhook{}
		ctx = context.Background()
	})

	Describe("ValidateCreate", func() {
		It("should accept valid PhysicalHost", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "https://bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, host)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject PhysicalHost with missing address", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						CredentialsSecretRef: "bmc-credentials",
					},
				},
			}

			_, err := webhook.ValidateCreate(ctx, host)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("address is required"))
		})

		It("should reject PhysicalHost with missing credentials", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address: "https://bmc.example.com",
					},
				},
			}

			_, err := webhook.ValidateCreate(ctx, host)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialsSecretRef is required"))
		})

		It("should reject PhysicalHost with invalid URL", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "ftp://invalid-scheme.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
				},
			}

			_, err := webhook.ValidateCreate(ctx, host)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unsupported value"))
		})

		It("should warn about insecureSkipVerify", func() {
			insecure := true
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "https://bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
						InsecureSkipVerify:   &insecure,
					},
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, host)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(string(warnings[0])).To(ContainSubstring("insecureSkipVerify is enabled"))
		})

		It("should warn about ConsumerRef on create", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "https://bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
					ConsumerRef: &corev1.ObjectReference{
						Name:      "test-machine",
						Namespace: "default",
					},
				},
			}

			warnings, err := webhook.ValidateCreate(ctx, host)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(string(warnings[0])).To(ContainSubstring("ConsumerRef is set on creation"))
		})

		It("should reject BootISOSource without ConsumerRef", func() {
			bootISO := "http://example.com/boot.iso"
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "https://bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
					BootISOSource: &bootISO,
				},
			}

			_, err := webhook.ValidateCreate(ctx, host)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bootIsoSource should only be set when host has a consumerRef"))
		})
	})

	Describe("ValidateUpdate", func() {
		It("should reject immutable address changes", func() {
			oldHost := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "https://old.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
				},
			}

			newHost := oldHost.DeepCopy()
			newHost.Spec.RedfishConnection.Address = "https://new.example.com"

			_, err := webhook.ValidateUpdate(ctx, oldHost, newHost)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("address is immutable after creation"))
		})

		It("should reject ConsumerRef removal during provisioning", func() {
			oldHost := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "https://bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
					ConsumerRef: &corev1.ObjectReference{
						Name:      "test-machine",
						Namespace: "default",
					},
				},
			}

			newHost := oldHost.DeepCopy()
			newHost.Spec.ConsumerRef = nil
			newHost.Status.State = infrav1beta1.StateProvisioning

			_, err := webhook.ValidateUpdate(ctx, oldHost, newHost)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot remove consumerRef while host is provisioning"))
		})
	})

	Describe("ValidateDelete", func() {
		It("should warn about deleting claimed host", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "https://bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
					ConsumerRef: &corev1.ObjectReference{
						Name:      "test-machine",
						Namespace: "default",
					},
				},
			}

			warnings, err := webhook.ValidateDelete(ctx, host)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(string(warnings[0])).To(ContainSubstring("currently claimed by"))
		})

		It("should warn about deleting provisioning host", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "https://bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
				},
				Status: infrav1beta1.PhysicalHostStatus{
					State: infrav1beta1.StateProvisioning,
				},
			}

			warnings, err := webhook.ValidateDelete(ctx, host)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(string(warnings[0])).To(ContainSubstring("currently provisioning"))
		})
	})

	Describe("Default", func() {
		It("should set default values", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
				},
			}

			err := webhook.Default(ctx, host)
			Expect(err).NotTo(HaveOccurred())

			// Should default to https
			Expect(host.Spec.RedfishConnection.Address).To(Equal("https://bmc.example.com"))

			// Should default insecureSkipVerify to false
			Expect(host.Spec.RedfishConnection.InsecureSkipVerify).NotTo(BeNil())
			Expect(*host.Spec.RedfishConnection.InsecureSkipVerify).To(BeFalse())
		})

		It("should not override existing scheme", func() {
			host := &infrav1beta1.PhysicalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-host",
					Namespace: "default",
				},
				Spec: infrav1beta1.PhysicalHostSpec{
					RedfishConnection: infrav1beta1.RedfishConnection{
						Address:              "http://bmc.example.com",
						CredentialsSecretRef: "bmc-credentials",
					},
				},
			}

			err := webhook.Default(ctx, host)
			Expect(err).NotTo(HaveOccurred())

			// Should keep existing http scheme
			Expect(host.Spec.RedfishConnection.Address).To(Equal("http://bmc.example.com"))
		})
	})
})
