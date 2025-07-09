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

// Beskar7MachineTemplateWebhook implements a validating and defaulting webhook for Beskar7MachineTemplate.
type Beskar7MachineTemplateWebhook struct{}

// SetupWebhookWithManager sets up the webhook with the manager.
func (webhook *Beskar7MachineTemplateWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1beta1.Beskar7MachineTemplate{}).
		WithValidator(webhook).
		WithDefaulter(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-beskar7machinetemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=beskar7machinetemplates,versions=v1beta1,name=validation.beskar7machinetemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-beskar7machinetemplate,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=beskar7machinetemplates,versions=v1beta1,name=defaulting.beskar7machinetemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.CustomValidator = &Beskar7MachineTemplateWebhook{}
var _ webhook.CustomDefaulter = &Beskar7MachineTemplateWebhook{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7MachineTemplateWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	template := obj.(*infrav1beta1.Beskar7MachineTemplate)
	return nil, webhook.validateMachineTemplate(template)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7MachineTemplateWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newTemplate := newObj.(*infrav1beta1.Beskar7MachineTemplate)
	oldTemplate := oldObj.(*infrav1beta1.Beskar7MachineTemplate)

	// Validate the new template
	if err := webhook.validateMachineTemplate(newTemplate); err != nil {
		return nil, err
	}

	// Validate immutable fields
	if err := webhook.validateImmutableFields(oldTemplate, newTemplate); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7MachineTemplateWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No specific validations needed for deletion
	return nil, nil
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *Beskar7MachineTemplateWebhook) Default(ctx context.Context, obj runtime.Object) error {
	template := obj.(*infrav1beta1.Beskar7MachineTemplate)

	// Apply defaults to the template spec
	return webhook.defaultMachineTemplate(template)
}

func (webhook *Beskar7MachineTemplateWebhook) validateMachineTemplate(template *infrav1beta1.Beskar7MachineTemplate) error {
	var allErrs field.ErrorList

	// Validate the template spec using the existing Beskar7Machine validation logic
	machineWebhook := &Beskar7MachineWebhook{}

	// Create a temporary machine with the template spec for validation
	tempMachine := &infrav1beta1.Beskar7Machine{
		Spec: template.Spec.Template.Spec,
	}

	if err := machineWebhook.validateMachine(tempMachine); err != nil {
		// Convert the machine validation errors to template path
		if fieldErr, ok := err.(*apierrors.StatusError); ok && fieldErr.Status().Details != nil {
			for _, cause := range fieldErr.Status().Details.Causes {
				// Adjust the field path to reflect template structure
				templatePath := "spec.template." + cause.Field
				allErrs = append(allErrs, field.Invalid(
					field.NewPath(templatePath),
					cause.Message,
					"template validation failed",
				))
			}
		} else {
			// Fallback for non-field errors
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "template"),
				template.Spec.Template,
				err.Error(),
			))
		}
	}

	// Additional template-specific validation
	if template.Spec.Template.Spec.ProviderID != nil && *template.Spec.Template.Spec.ProviderID != "" {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "template", "spec", "providerID"),
			"providerID should not be set in machine templates",
		))
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			template.GroupVersionKind().GroupKind(),
			template.Name,
			allErrs,
		)
	}

	return nil
}

func (webhook *Beskar7MachineTemplateWebhook) validateImmutableFields(oldTemplate, newTemplate *infrav1beta1.Beskar7MachineTemplate) error {
	var allErrs field.ErrorList

	// In machine templates, the entire spec should be immutable after creation
	// This is because changing templates could affect existing machines
	oldSpec := oldTemplate.Spec.Template.Spec
	newSpec := newTemplate.Spec.Template.Spec

	// Check immutable fields
	if oldSpec.ImageURL != newSpec.ImageURL {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "template", "spec", "imageURL"),
			"imageURL is immutable in machine templates",
		))
	}

	if oldSpec.OSFamily != newSpec.OSFamily {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "template", "spec", "osFamily"),
			"osFamily is immutable in machine templates",
		))
	}

	if oldSpec.ProvisioningMode != newSpec.ProvisioningMode {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "template", "spec", "provisioningMode"),
			"provisioningMode is immutable in machine templates",
		))
	}

	if oldSpec.ConfigURL != newSpec.ConfigURL {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "template", "spec", "configURL"),
			"configURL is immutable in machine templates",
		))
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			newTemplate.GroupVersionKind().GroupKind(),
			newTemplate.Name,
			allErrs,
		)
	}

	return nil
}

func (webhook *Beskar7MachineTemplateWebhook) defaultMachineTemplate(template *infrav1beta1.Beskar7MachineTemplate) error {
	// Apply defaults using the existing Beskar7Machine defaulting logic
	machineWebhook := &Beskar7MachineWebhook{}

	// Create a temporary machine with the template spec for defaulting
	tempMachine := &infrav1beta1.Beskar7Machine{
		Spec: template.Spec.Template.Spec,
	}

	if err := machineWebhook.defaultMachine(tempMachine); err != nil {
		return err
	}

	// Copy the defaults back to the template
	template.Spec.Template.Spec = tempMachine.Spec

	return nil
}
