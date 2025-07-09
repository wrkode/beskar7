package webhooks

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// PhysicalHostWebhook implements a validating and defaulting webhook for PhysicalHost.
type PhysicalHostWebhook struct{}

// SetupWebhookWithManager sets up the webhook with the manager.
func (webhook *PhysicalHostWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1beta1.PhysicalHost{}).
		WithValidator(webhook).
		WithDefaulter(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-physicalhost,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,versions=v1beta1,name=validation.physicalhost.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-physicalhost,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,versions=v1beta1,name=defaulting.physicalhost.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.CustomValidator = &PhysicalHostWebhook{}
var _ webhook.CustomDefaulter = &PhysicalHostWebhook{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PhysicalHostWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	host := obj.(*infrav1beta1.PhysicalHost)
	warnings, err := webhook.validatePhysicalHost(host)
	if err != nil {
		return warnings, err
	}

	// Create-specific validations
	createWarnings, createErr := webhook.validatePhysicalHostCreate(host)
	warnings = append(warnings, createWarnings...)
	if createErr != nil {
		return warnings, createErr
	}

	return warnings, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PhysicalHostWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldHost := oldObj.(*infrav1beta1.PhysicalHost)
	newHost := newObj.(*infrav1beta1.PhysicalHost)

	warnings, err := webhook.validatePhysicalHost(newHost)
	if err != nil {
		return warnings, err
	}

	// Update-specific validations
	updateWarnings, updateErr := webhook.validatePhysicalHostUpdate(oldHost, newHost)
	warnings = append(warnings, updateWarnings...)
	if updateErr != nil {
		return warnings, updateErr
	}

	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PhysicalHostWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	host := obj.(*infrav1beta1.PhysicalHost)
	var warnings admission.Warnings

	// Check if host is currently claimed/provisioning
	if host.Spec.ConsumerRef != nil {
		warnings = append(warnings, fmt.Sprintf(
			"PhysicalHost %s is currently claimed by %s/%s. Ensure proper cleanup before deletion.",
			host.Name, host.Spec.ConsumerRef.Namespace, host.Spec.ConsumerRef.Name,
		))
	}

	if host.Status.State == infrav1beta1.StateProvisioning {
		warnings = append(warnings, fmt.Sprintf(
			"PhysicalHost %s is currently provisioning. Deletion may leave the host in an inconsistent state.",
			host.Name,
		))
	}

	return warnings, nil
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *PhysicalHostWebhook) Default(ctx context.Context, obj runtime.Object) error {
	host := obj.(*infrav1beta1.PhysicalHost)
	return webhook.defaultPhysicalHost(host)
}

func (webhook *PhysicalHostWebhook) validatePhysicalHost(host *infrav1beta1.PhysicalHost) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var warnings admission.Warnings

	// Validate RedfishConnection
	if host.Spec.RedfishConnection.Address == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "redfishConnection", "address"),
			"address is required",
		))
	} else {
		// Validate address format
		if errs := webhook.validateRedfishAddress(host.Spec.RedfishConnection.Address); len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	if host.Spec.RedfishConnection.CredentialsSecretRef == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "redfishConnection", "credentialsSecretRef"),
			"credentialsSecretRef is required",
		))
	}

	// Security warning for insecureSkipVerify
	if host.Spec.RedfishConnection.InsecureSkipVerify != nil && *host.Spec.RedfishConnection.InsecureSkipVerify {
		warnings = append(warnings,
			"insecureSkipVerify is enabled. This is not recommended for production environments as it disables TLS certificate verification.",
		)
	}

	// Validate ConsumerRef if present
	if host.Spec.ConsumerRef != nil {
		if errs := webhook.validateConsumerRef(host.Spec.ConsumerRef); len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	// Validate UserDataSecretRef if present
	if host.Spec.UserDataSecretRef != nil {
		if host.Spec.UserDataSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(
				field.NewPath("spec", "userDataSecretRef", "name"),
				"name is required when userDataSecretRef is specified",
			))
		}
	}

	// Validate consistency between fields
	if errs := webhook.validateFieldConsistency(host); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			host.GroupVersionKind().GroupKind(),
			host.Name,
			allErrs,
		)
	}

	return warnings, nil
}

func (webhook *PhysicalHostWebhook) validatePhysicalHostCreate(host *infrav1beta1.PhysicalHost) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var warnings admission.Warnings

	// On create, ConsumerRef should typically not be set (hosts start unclaimed)
	if host.Spec.ConsumerRef != nil {
		warnings = append(warnings,
			"ConsumerRef is set on creation. Typically hosts should be created unclaimed and later claimed by machines.",
		)
	}

	// On create, BootISOSource should typically not be set
	if host.Spec.BootISOSource != nil {
		warnings = append(warnings,
			"BootISOSource is set on creation. This field is typically managed by the consuming machine.",
		)
	}

	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			host.GroupVersionKind().GroupKind(),
			host.Name,
			allErrs,
		)
	}

	return warnings, nil
}

func (webhook *PhysicalHostWebhook) validatePhysicalHostUpdate(oldHost, newHost *infrav1beta1.PhysicalHost) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var warnings admission.Warnings

	// Validate immutable fields
	if oldHost.Spec.RedfishConnection.Address != newHost.Spec.RedfishConnection.Address {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "redfishConnection", "address"),
			"address is immutable after creation",
		))
	}

	// Validate state-dependent changes
	if newHost.Status.State == infrav1beta1.StateProvisioning {
		// During provisioning, certain fields should not be changed
		if oldHost.Spec.ConsumerRef != nil && newHost.Spec.ConsumerRef == nil {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "consumerRef"),
				"cannot remove consumerRef while host is provisioning",
			))
		}

		if oldHost.Spec.ConsumerRef != nil && newHost.Spec.ConsumerRef != nil &&
			(oldHost.Spec.ConsumerRef.Name != newHost.Spec.ConsumerRef.Name ||
				oldHost.Spec.ConsumerRef.Namespace != newHost.Spec.ConsumerRef.Namespace) {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "consumerRef"),
				"cannot change consumerRef while host is provisioning",
			))
		}
	}

	// Validate ConsumerRef transitions
	if oldHost.Spec.ConsumerRef == nil && newHost.Spec.ConsumerRef != nil {
		// Host is being claimed
		if newHost.Status.State != "" &&
			newHost.Status.State != infrav1beta1.StateAvailable &&
			newHost.Status.State != infrav1beta1.StateEnrolling {
			warnings = append(warnings, fmt.Sprintf(
				"Host is being claimed while in state %s. Ensure this is intentional.",
				newHost.Status.State,
			))
		}
	}

	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			newHost.GroupVersionKind().GroupKind(),
			newHost.Name,
			allErrs,
		)
	}

	return warnings, nil
}

func (webhook *PhysicalHostWebhook) validateRedfishAddress(address string) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "redfishConnection", "address")

	// Parse URL
	parsedURL, err := url.Parse(address)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			address,
			fmt.Sprintf("invalid URL format: %v", err),
		))
		return allErrs
	}

	// Validate scheme
	if parsedURL.Scheme == "" {
		// Try adding https and parsing again
		if _, err := url.Parse("https://" + address); err != nil {
			allErrs = append(allErrs, field.Invalid(
				fieldPath,
				address,
				"address must be a valid URL with scheme (https:// or http://)",
			))
		}
	} else if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		allErrs = append(allErrs, field.NotSupported(
			fieldPath,
			parsedURL.Scheme,
			[]string{"https", "http"},
		))
	}

	// Validate host
	if parsedURL.Host == "" && parsedURL.Scheme != "" {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			address,
			"address must include a host",
		))
	}

	// Security recommendations
	if parsedURL.Scheme == "http" {
		// This will be reported as a warning in the main validation
	}

	return allErrs
}

func (webhook *PhysicalHostWebhook) validateConsumerRef(consumerRef *corev1.ObjectReference) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "consumerRef")

	if consumerRef.Name == "" {
		allErrs = append(allErrs, field.Required(
			fieldPath.Child("name"),
			"name is required",
		))
	}

	if consumerRef.Namespace == "" {
		allErrs = append(allErrs, field.Required(
			fieldPath.Child("namespace"),
			"namespace is required",
		))
	}

	// Validate APIVersion format if provided
	if consumerRef.APIVersion != "" && !strings.Contains(consumerRef.APIVersion, "/") {
		allErrs = append(allErrs, field.Invalid(
			fieldPath.Child("apiVersion"),
			consumerRef.APIVersion,
			"apiVersion must be in the format 'group/version'",
		))
	}

	return allErrs
}

func (webhook *PhysicalHostWebhook) validateFieldConsistency(host *infrav1beta1.PhysicalHost) field.ErrorList {
	var allErrs field.ErrorList

	// If ConsumerRef is set, we expect this to be a claimed host
	// If BootISOSource is set, we expect ConsumerRef to also be set
	if host.Spec.BootISOSource != nil && host.Spec.ConsumerRef == nil {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "bootIsoSource"),
			"bootIsoSource should only be set when host has a consumerRef",
		))
	}

	return allErrs
}

func (webhook *PhysicalHostWebhook) defaultPhysicalHost(host *infrav1beta1.PhysicalHost) error {
	// Set default values for InsecureSkipVerify if not specified
	if host.Spec.RedfishConnection.InsecureSkipVerify == nil {
		defaultValue := false
		host.Spec.RedfishConnection.InsecureSkipVerify = &defaultValue
	}

	// If address doesn't have a scheme, default to https
	if host.Spec.RedfishConnection.Address != "" {
		if !strings.HasPrefix(host.Spec.RedfishConnection.Address, "http://") &&
			!strings.HasPrefix(host.Spec.RedfishConnection.Address, "https://") {
			host.Spec.RedfishConnection.Address = "https://" + host.Spec.RedfishConnection.Address
		}
	}

	return nil
}
