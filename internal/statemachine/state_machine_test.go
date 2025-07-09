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

package statemachine

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

func TestStateMachine(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "State Machine Suite")
}

var _ = Describe("PhysicalHostStateMachine", func() {
	var (
		sm         *PhysicalHostStateMachine
		testLogger logr.Logger
		testHost   *infrastructurev1beta1.PhysicalHost
	)

	BeforeEach(func() {
		testLogger = logr.Discard()
		sm = NewPhysicalHostStateMachine(testLogger)

		testHost = &infrastructurev1beta1.PhysicalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-host",
				Namespace: "default",
			},
			Spec: infrastructurev1beta1.PhysicalHostSpec{
				RedfishConnection: infrastructurev1beta1.RedfishConnection{
					Address:              "https://192.168.1.100",
					CredentialsSecretRef: "test-secret",
					InsecureSkipVerify:   nil,
				},
			},
			Status: infrastructurev1beta1.PhysicalHostStatus{
				State: infrastructurev1beta1.StateNone,
			},
		}
	})

	Context("State Validation", func() {
		It("should validate all PhysicalHost states as valid", func() {
			validStates := []string{
				infrastructurev1beta1.StateNone,
				infrastructurev1beta1.StateEnrolling,
				infrastructurev1beta1.StateAvailable,
				infrastructurev1beta1.StateClaimed,
				infrastructurev1beta1.StateProvisioning,
				infrastructurev1beta1.StateProvisioned,
				infrastructurev1beta1.StateDeprovisioning,
				infrastructurev1beta1.StateError,
				infrastructurev1beta1.StateUnknown,
			}

			for _, state := range validStates {
				Expect(sm.IsStateValid(state)).To(BeTrue(), "State %s should be valid", state)
			}
		})

		It("should reject invalid states", func() {
			invalidStates := []string{
				"InvalidState",
				"NotAState",
				"",
			}

			for _, state := range invalidStates {
				if state == "" {
					// Empty state is treated as StateNone which is valid
					continue
				}
				Expect(sm.IsStateValid(state)).To(BeFalse(), "State %s should be invalid", state)
			}
		})
	})

	Context("Basic State Transitions", func() {
		It("should allow transition from None to Enrolling", func() {
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateEnrolling)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject transition from None to Available without going through Enrolling", func() {
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateAvailable)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not allowed"))
		})

		It("should allow transition from Enrolling to Available", func() {
			testHost.Status.State = infrastructurev1beta1.StateEnrolling
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateAvailable)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow transition from Available to Claimed when ConsumerRef is set", func() {
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			testHost.Spec.ConsumerRef = &corev1.ObjectReference{
				Name:      "test-machine",
				Namespace: "default",
			}
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateClaimed)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject transition from Available to Claimed when ConsumerRef is not set", func() {
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			testHost.Spec.ConsumerRef = nil
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateClaimed)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no ConsumerRef set"))
		})
	})

	Context("Provisioning Transitions", func() {
		BeforeEach(func() {
			testHost.Status.State = infrastructurev1beta1.StateClaimed
			testHost.Spec.ConsumerRef = &corev1.ObjectReference{
				Name:      "test-machine",
				Namespace: "default",
			}
		})

		It("should allow transition from Claimed to Provisioning when BootISOSource is set", func() {
			isoURL := "http://example.com/boot.iso"
			testHost.Spec.BootISOSource = &isoURL
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateProvisioning)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject transition from Claimed to Provisioning when BootISOSource is not set", func() {
			testHost.Spec.BootISOSource = nil
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateProvisioning)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no BootISOSource set"))
		})

		It("should allow transition from Provisioning to Provisioned", func() {
			testHost.Status.State = infrastructurev1beta1.StateProvisioning
			isoURL := "http://example.com/boot.iso"
			testHost.Spec.BootISOSource = &isoURL
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateProvisioned)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Error and Recovery Transitions", func() {
		It("should allow transition to Error from any state", func() {
			states := []string{
				infrastructurev1beta1.StateEnrolling,
				infrastructurev1beta1.StateAvailable,
				infrastructurev1beta1.StateClaimed,
				infrastructurev1beta1.StateProvisioning,
			}

			for _, state := range states {
				testHost.Status.State = state
				err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateError)
				Expect(err).NotTo(HaveOccurred(), "Should allow transition from %s to Error", state)
			}
		})

		It("should allow recovery from Error to Enrolling when connection info is present", func() {
			testHost.Status.State = infrastructurev1beta1.StateError
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateEnrolling)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject recovery from Error to Enrolling when connection info is missing", func() {
			testHost.Status.State = infrastructurev1beta1.StateError
			testHost.Spec.RedfishConnection.Address = ""
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateEnrolling)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing Redfish address"))
		})

		It("should allow direct recovery from Error to Available when host is not claimed", func() {
			testHost.Status.State = infrastructurev1beta1.StateError
			testHost.Spec.ConsumerRef = nil
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateAvailable)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject direct recovery from Error to Available when host is claimed", func() {
			testHost.Status.State = infrastructurev1beta1.StateError
			testHost.Spec.ConsumerRef = &corev1.ObjectReference{
				Name:      "test-machine",
				Namespace: "default",
			}
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateAvailable)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("host is claimed"))
		})
	})

	Context("Release Transitions", func() {
		It("should allow release from Claimed to Available when ConsumerRef is cleared", func() {
			testHost.Status.State = infrastructurev1beta1.StateClaimed
			testHost.Spec.ConsumerRef = nil
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateAvailable)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow release from Provisioned to Available when ConsumerRef is cleared", func() {
			testHost.Status.State = infrastructurev1beta1.StateProvisioned
			testHost.Spec.ConsumerRef = nil
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateAvailable)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject release when ConsumerRef is still set", func() {
			testHost.Status.State = infrastructurev1beta1.StateClaimed
			testHost.Spec.ConsumerRef = &corev1.ObjectReference{
				Name:      "test-machine",
				Namespace: "default",
			}
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateAvailable)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ConsumerRef still set"))
		})
	})

	Context("Deprovisioning Transitions", func() {
		BeforeEach(func() {
			now := metav1.Now()
			testHost.DeletionTimestamp = &now
		})

		It("should allow transition from Available to Deprovisioning when marked for deletion", func() {
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			testHost.Spec.ConsumerRef = nil
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateDeprovisioning)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow transition from Provisioned to Deprovisioning when marked for deletion", func() {
			testHost.Status.State = infrastructurev1beta1.StateProvisioned
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateDeprovisioning)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject transition to Deprovisioning when not marked for deletion", func() {
			testHost.DeletionTimestamp = nil
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateDeprovisioning)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not marked for deletion"))
		})
	})

	Context("State Consistency Validation", func() {
		It("should validate Available state consistency", func() {
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			testHost.Spec.ConsumerRef = nil
			testHost.Spec.BootISOSource = nil
			err := sm.ValidateStateConsistency(testHost)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject Available state with ConsumerRef set", func() {
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			testHost.Spec.ConsumerRef = &corev1.ObjectReference{
				Name: "test-machine",
			}
			err := sm.ValidateStateConsistency(testHost)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot have ConsumerRef set"))
		})

		It("should reject Available state with BootISOSource set", func() {
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			isoURL := "http://example.com/boot.iso"
			testHost.Spec.BootISOSource = &isoURL
			err := sm.ValidateStateConsistency(testHost)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot have BootISOSource set"))
		})

		It("should validate Claimed state consistency", func() {
			testHost.Status.State = infrastructurev1beta1.StateClaimed
			testHost.Spec.ConsumerRef = &corev1.ObjectReference{
				Name: "test-machine",
			}
			err := sm.ValidateStateConsistency(testHost)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject Claimed state without ConsumerRef", func() {
			testHost.Status.State = infrastructurev1beta1.StateClaimed
			testHost.Spec.ConsumerRef = nil
			err := sm.ValidateStateConsistency(testHost)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must have ConsumerRef set"))
		})

		It("should validate Provisioning state consistency", func() {
			testHost.Status.State = infrastructurev1beta1.StateProvisioning
			testHost.Spec.ConsumerRef = &corev1.ObjectReference{
				Name: "test-machine",
			}
			isoURL := "http://example.com/boot.iso"
			testHost.Spec.BootISOSource = &isoURL
			err := sm.ValidateStateConsistency(testHost)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject Provisioning state without BootISOSource", func() {
			testHost.Status.State = infrastructurev1beta1.StateProvisioning
			testHost.Spec.ConsumerRef = &corev1.ObjectReference{
				Name: "test-machine",
			}
			testHost.Spec.BootISOSource = nil
			err := sm.ValidateStateConsistency(testHost)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must have BootISOSource set"))
		})
	})

	Context("Idempotent Transitions", func() {
		It("should allow same state transition (idempotent)", func() {
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			err := sm.ValidateTransition(testHost, infrastructurev1beta1.StateAvailable)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow TransitionTo with same state", func() {
			testHost.Status.State = infrastructurev1beta1.StateAvailable
			err := sm.TransitionTo(context.Background(), testHost, infrastructurev1beta1.StateAvailable, "idempotent")
			Expect(err).NotTo(HaveOccurred())
			Expect(testHost.Status.State).To(Equal(infrastructurev1beta1.StateAvailable))
		})
	})

	Context("Valid Transitions List", func() {
		It("should return valid transitions from StateNone", func() {
			transitions := sm.GetValidTransitions(infrastructurev1beta1.StateNone)
			Expect(transitions).To(ContainElement(infrastructurev1beta1.StateEnrolling))
		})

		It("should return valid transitions from StateAvailable", func() {
			transitions := sm.GetValidTransitions(infrastructurev1beta1.StateAvailable)
			Expect(transitions).To(ContainElement(infrastructurev1beta1.StateClaimed))
			Expect(transitions).To(ContainElement(infrastructurev1beta1.StateError))
			Expect(transitions).To(ContainElement(infrastructurev1beta1.StateDeprovisioning))
		})

		It("should return empty transitions for unknown state", func() {
			transitions := sm.GetValidTransitions("UnknownState")
			Expect(transitions).To(BeEmpty())
		})
	})
})

var _ = Describe("StateTransitionGuard", func() {
	var (
		guard      *StateTransitionGuard
		testLogger logr.Logger
	)

	BeforeEach(func() {
		testLogger = logr.Discard()
		guard = NewStateTransitionGuard(nil, testLogger)
	})

	Context("Safe State Transitions", func() {
		It("should validate transition guards exist", func() {
			Expect(guard).NotTo(BeNil())
		})
	})
})

var _ = Describe("StateRecoveryManager", func() {
	var (
		rm         *StateRecoveryManager
		testHost   *infrastructurev1beta1.PhysicalHost
		testLogger logr.Logger
	)

	BeforeEach(func() {
		testLogger = logr.Discard()
		rm = NewStateRecoveryManager(nil, testLogger)

		testHost = &infrastructurev1beta1.PhysicalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-host",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
			},
			Spec: infrastructurev1beta1.PhysicalHostSpec{
				RedfishConnection: infrastructurev1beta1.RedfishConnection{
					Address:              "https://192.168.1.100",
					CredentialsSecretRef: "test-secret",
				},
			},
			Status: infrastructurev1beta1.PhysicalHostStatus{
				State: infrastructurev1beta1.StateEnrolling,
			},
		}
	})

	Context("Stuck State Detection", func() {
		It("should detect stuck state when timeout exceeded", func() {
			timeout := 1 * time.Hour
			isStuck := rm.DetectStuckState(testHost, timeout)
			Expect(isStuck).To(BeTrue())
		})

		It("should not detect stuck state when within timeout", func() {
			timeout := 3 * time.Hour
			isStuck := rm.DetectStuckState(testHost, timeout)
			Expect(isStuck).To(BeFalse())
		})
	})
})
