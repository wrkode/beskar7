package webhooks

import (
	"context"
	"net/url"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// Provisioning mode constants for validation/defaulting in this package
const (
	provisioningModeRemoteConfig = "RemoteConfig"
	provisioningModePreBakedISO  = "PreBakedISO"
)

// URL scheme constants
const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
	SchemeFTP   = "ftp"
	SchemeFile  = "file"
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
	} else {
		// Validate ImageURL format
		if errs := webhook.validateImageURL(machine.Spec.ImageURL); len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	if machine.Spec.OSFamily == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "osFamily"),
			"osFamily is required",
		))
	} else {
		// Validate OS family
		if errs := webhook.validateOSFamily(machine.Spec.OSFamily); len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	// Validate provisioning mode if specified
	if machine.Spec.ProvisioningMode != "" {
		if errs := webhook.validateProvisioningMode(machine.Spec.ProvisioningMode); len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	// Validate boot mode if specified
	if machine.Spec.BootMode != "" {
		if errs := webhook.validateBootMode(machine.Spec.BootMode); len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	// Validate ConfigURL format if specified
	if machine.Spec.ConfigURL != "" {
		if errs := webhook.validateConfigURL(machine.Spec.ConfigURL); len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	// Cross-field validation: ConfigURL is required for RemoteConfig mode

	if machine.Spec.ProvisioningMode == provisioningModeRemoteConfig && machine.Spec.ConfigURL == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "configURL"),
			"configURL is required when provisioningMode is RemoteConfig",
		))
	}

	// ConfigURL should not be set for PreBakedISO mode
	if machine.Spec.ProvisioningMode == provisioningModePreBakedISO && machine.Spec.ConfigURL != "" {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "configURL"),
			"configURL should not be set when provisioningMode is PreBakedISO",
		))
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
		machine.Spec.ProvisioningMode = provisioningModeRemoteConfig
	}

	// Set default boot mode if not specified
	if machine.Spec.BootMode == "" {
		machine.Spec.BootMode = "UEFI"
	}

	return nil
}

func (webhook *Beskar7MachineWebhook) validateImageURL(imageURL string) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "imageURL")

	// Parse URL
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			imageURL,
			"invalid URL format",
		))
		return allErrs
	}

	// Validate scheme
	validSchemes := map[string]bool{
		SchemeHTTP:  true,
		SchemeHTTPS: true,
		SchemeFTP:   true,
		SchemeFile:  true,
	}
	if !validSchemes[parsedURL.Scheme] {
		allErrs = append(allErrs, field.NotSupported(
			fieldPath,
			parsedURL.Scheme,
			[]string{SchemeHTTP, SchemeHTTPS, SchemeFTP, SchemeFile},
		))
	}

	// Validate host for network schemes
	if (parsedURL.Scheme == SchemeHTTP || parsedURL.Scheme == SchemeHTTPS || parsedURL.Scheme == SchemeFTP) && parsedURL.Host == "" {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			imageURL,
			"URL must include a host for network schemes",
		))
	}

	// Validate file extension for image files
	if !webhook.isValidImageExtension(parsedURL.Path) {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			imageURL,
			"imageURL should point to a valid image file (.iso, .img, .qcow2, .vmdk, .raw)",
		))
	}

	return allErrs
}

func (webhook *Beskar7MachineWebhook) validateConfigURL(configURL string) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "configURL")

	// Parse URL
	parsedURL, err := url.Parse(configURL)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			configURL,
			"invalid URL format",
		))
		return allErrs
	}

	// Validate scheme
	validSchemes := map[string]bool{
		SchemeHTTP:  true,
		SchemeHTTPS: true,
		SchemeFile:  true,
	}
	if !validSchemes[parsedURL.Scheme] {
		allErrs = append(allErrs, field.NotSupported(
			fieldPath,
			parsedURL.Scheme,
			[]string{SchemeHTTP, SchemeHTTPS, SchemeFile},
		))
	}

	// Validate host for network schemes
	if (parsedURL.Scheme == SchemeHTTP || parsedURL.Scheme == SchemeHTTPS) && parsedURL.Host == "" {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			configURL,
			"URL must include a host for network schemes",
		))
	}

	// Validate file extension for config files
	if !webhook.isValidConfigExtension(parsedURL.Path) {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			configURL,
			"configURL should point to a valid configuration file (.yaml, .yml, .json, .toml)",
		))
	}

	return allErrs
}

func (webhook *Beskar7MachineWebhook) validateOSFamily(osFamily string) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "osFamily")

	validOSFamilies := map[string]bool{
		"kairos":    true,
		"flatcar":   true,
		"LeapMicro": true,
	}

	if !validOSFamilies[osFamily] {
		allErrs = append(allErrs, field.NotSupported(
			fieldPath,
			osFamily,
			[]string{"kairos", "flatcar", "LeapMicro"},
		))
	}

	return allErrs
}

func (webhook *Beskar7MachineWebhook) validateProvisioningMode(provisioningMode string) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "provisioningMode")

	validModes := map[string]bool{
		"RemoteConfig": true,
		"PreBakedISO":  true,
		"PXE":          true,
		"iPXE":         true,
	}

	if !validModes[provisioningMode] {
		allErrs = append(allErrs, field.NotSupported(
			fieldPath,
			provisioningMode,
			[]string{"RemoteConfig", "PreBakedISO", "PXE", "iPXE"},
		))
	}

	return allErrs
}

func (webhook *Beskar7MachineWebhook) validateBootMode(bootMode string) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "bootMode")

	validBootModes := map[string]bool{
		"UEFI":   true,
		"Legacy": true,
	}

	if !validBootModes[bootMode] {
		allErrs = append(allErrs, field.NotSupported(
			fieldPath,
			bootMode,
			[]string{"UEFI", "Legacy"},
		))
	}

	return allErrs
}

func (webhook *Beskar7MachineWebhook) isValidImageExtension(path string) bool {
	validExtensions := []string{
		".iso", ".img", ".qcow2", ".vmdk", ".raw", ".vhd", ".vhdx", ".ova", ".ovf",
	}

	path = strings.ToLower(path)
	for _, ext := range validExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	// Check for compressed files
	compressedExtensions := []string{
		".gz", ".bz2", ".xz", ".zip", ".tar", ".tgz", ".tbz2", ".txz",
	}
	for _, compExt := range compressedExtensions {
		if strings.HasSuffix(path, compExt) {
			// Remove compression extension and check again
			basePath := strings.TrimSuffix(path, compExt)
			for _, ext := range validExtensions {
				if strings.HasSuffix(basePath, ext) {
					return true
				}
			}
		}
	}

	return false
}

func (webhook *Beskar7MachineWebhook) isValidConfigExtension(path string) bool {
	validExtensions := []string{
		".yaml", ".yml", ".json", ".toml", ".conf", ".cfg", ".ini", ".properties",
	}

	path = strings.ToLower(path)
	for _, ext := range validExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	return false
}
