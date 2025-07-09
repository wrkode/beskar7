package webhooks

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// Beskar7MachineWebhook implements a validating and defaulting webhook for Beskar7Machine.
type Beskar7MachineWebhook struct{}

// SetupWebhookWithManager sets up the webhook with the manager.
func (webhook *Beskar7MachineWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1beta1.Beskar7Machine{}).
		WithValidator(webhook).
		WithDefaulter(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-beskar7machine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines,versions=v1beta1,name=validation.beskar7.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-beskar7machine,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines,versions=v1beta1,name=defaulting.beskar7.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.CustomValidator = &Beskar7MachineWebhook{}
var _ webhook.CustomDefaulter = &Beskar7MachineWebhook{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7MachineWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	machine := obj.(*infrav1beta1.Beskar7Machine)
	return nil, webhook.validateMachine(machine)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7MachineWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newMachine := newObj.(*infrav1beta1.Beskar7Machine)
	return nil, webhook.validateMachine(newMachine)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7MachineWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *Beskar7MachineWebhook) Default(ctx context.Context, obj runtime.Object) error {
	machine := obj.(*infrav1beta1.Beskar7Machine)
	return webhook.defaultMachine(machine)
}

func (webhook *Beskar7MachineWebhook) validateMachine(machine *infrav1beta1.Beskar7Machine) error {
	var allErrs field.ErrorList

	// Validate required fields
	if machine.Spec.ImageURL == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "imageURL"),
			"imageURL is required",
		))
	}

	if machine.Spec.OSFamily == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "osFamily"),
			"osFamily is required",
		))
	}

	// Validate OS family
	validOSFamilies := map[string]bool{
		"kairos":    true,
		"talos":     true,
		"flatcar":   true,
		"LeapMicro": true,
	}
	if !validOSFamilies[machine.Spec.OSFamily] {
		allErrs = append(allErrs, field.NotSupported(
			field.NewPath("spec", "osFamily"),
			machine.Spec.OSFamily,
			[]string{"kairos", "talos", "flatcar", "LeapMicro"},
		))
	}

	// Validate provisioning mode if specified
	if machine.Spec.ProvisioningMode != "" {
		validModes := map[string]bool{
			"RemoteConfig": true,
			"PreBakedISO":  true,
		}
		if !validModes[machine.Spec.ProvisioningMode] {
			allErrs = append(allErrs, field.NotSupported(
				field.NewPath("spec", "provisioningMode"),
				machine.Spec.ProvisioningMode,
				[]string{"RemoteConfig", "PreBakedISO"},
			))
		}
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			machine.GroupVersionKind().GroupKind(),
			machine.Name,
			allErrs,
		)
	}

	return nil
}

func (webhook *Beskar7MachineWebhook) defaultMachine(machine *infrav1beta1.Beskar7Machine) error {
	// Set default provisioning mode if not specified
	if machine.Spec.ProvisioningMode == "" {
		machine.Spec.ProvisioningMode = "RemoteConfig"
	}

	return nil
}
