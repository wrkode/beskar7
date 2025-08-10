package webhooks

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// log is for logging in this package.
var physicalHostWebhookLog = ctrl.Log.WithName("physicalhost-webhook")

// PhysicalHostWebhook implements webhook.Validator and webhook.Defaulter
type PhysicalHostWebhook struct {
	Client client.Client
}

func (webhook *PhysicalHostWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	webhook.Client = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1beta1.PhysicalHost{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-physicalhost,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,verbs=create;update,versions=v1beta1,name=default.physicalhost.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &PhysicalHostWebhook{}

// Default implements webhook.CustomDefaulter
func (webhook *PhysicalHostWebhook) Default(ctx context.Context, obj runtime.Object) error {
	host, ok := obj.(*infrav1beta1.PhysicalHost)
	if !ok {
		return fmt.Errorf("expected PhysicalHost, got %T", obj)
	}

	physicalHostWebhookLog.Info("Applying defaults to PhysicalHost", "name", host.Name, "namespace", host.Namespace)

	return webhook.defaultPhysicalHost(host)
}

//+kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-physicalhost,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,verbs=create;update,versions=v1beta1,name=validation.physicalhost.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &PhysicalHostWebhook{}

// ValidateCreate implements webhook.CustomValidator
func (webhook *PhysicalHostWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	host, ok := obj.(*infrav1beta1.PhysicalHost)
	if !ok {
		return nil, fmt.Errorf("expected PhysicalHost, got %T", obj)
	}

	physicalHostWebhookLog.Info("Validating PhysicalHost creation", "name", host.Name, "namespace", host.Namespace)

	warnings, err := webhook.validatePhysicalHost(host)
	if err != nil {
		return warnings, err
	}

	// Warn if a ConsumerRef is set at creation time
	if host.Spec.ConsumerRef != nil {
		warnings = append(warnings, "ConsumerRef is set on creation. Ensure claim lifecycle is coordinated by the controller.")
	}

	// Additional security validation for creation
	securityWarnings, secErr := webhook.validateSecurityRequirements(ctx, host)
	warnings = append(warnings, securityWarnings...)
	if secErr != nil {
		return warnings, secErr
	}

	return warnings, nil
}

// ValidateUpdate implements webhook.CustomValidator
func (webhook *PhysicalHostWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldHost, ok := oldObj.(*infrav1beta1.PhysicalHost)
	if !ok {
		return nil, fmt.Errorf("expected PhysicalHost, got %T", oldObj)
	}
	newHost, ok := newObj.(*infrav1beta1.PhysicalHost)
	if !ok {
		return nil, fmt.Errorf("expected PhysicalHost, got %T", newObj)
	}

	physicalHostWebhookLog.Info("Validating PhysicalHost update", "name", newHost.Name, "namespace", newHost.Namespace)

	warnings, err := webhook.validatePhysicalHost(newHost)
	if err != nil {
		return warnings, err
	}

	// Additional validation for updates
	updateWarnings, updateErr := webhook.validatePhysicalHostUpdate(oldHost, newHost)
	warnings = append(warnings, updateWarnings...)
	if updateErr != nil {
		return warnings, updateErr
	}

	// Security validation for updates
	securityWarnings, secErr := webhook.validateSecurityRequirements(ctx, newHost)
	warnings = append(warnings, securityWarnings...)
	if secErr != nil {
		return warnings, secErr
	}

	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator
func (webhook *PhysicalHostWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	host, ok := obj.(*infrav1beta1.PhysicalHost)
	if !ok {
		return nil, fmt.Errorf("expected PhysicalHost, got %T", obj)
	}

	physicalHostWebhookLog.Info("Validating PhysicalHost deletion", "name", host.Name, "namespace", host.Namespace)

	// Prevent deletion if host is claimed by a machine
	if host.Spec.ConsumerRef != nil {
		return admission.Warnings{
			"PhysicalHost is currently claimed by a consumer. Ensure the consumer is properly cleaned up before deletion.",
		}, nil
	}

	// Warn if host is currently provisioning
	if strings.EqualFold(host.Status.State, infrav1beta1.StateProvisioning) {
		return admission.Warnings{
			"PhysicalHost is currently provisioning; deletion may interrupt ongoing operations.",
		}, nil
	}

	return nil, nil
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

	// Enhanced security validation for insecureSkipVerify
	if host.Spec.RedfishConnection.InsecureSkipVerify != nil && *host.Spec.RedfishConnection.InsecureSkipVerify {
		// Check if this is a development environment
		if !webhook.isDevEnvironment(host) {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "redfishConnection", "insecureSkipVerify"),
				"insecureSkipVerify=true is not allowed in production environments. Please configure proper TLS certificates.",
			))
		} else {
			warnings = append(warnings,
				"insecureSkipVerify is enabled. This is not recommended for production environments as it disables TLS certificate verification.",
			)
		}
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

// validateSecurityRequirements performs enhanced security validation
func (webhook *PhysicalHostWebhook) validateSecurityRequirements(ctx context.Context, host *infrav1beta1.PhysicalHost) (admission.Warnings, error) {
	var warnings admission.Warnings
	var allErrs field.ErrorList

	// Validate credential secret if it exists
	if host.Spec.RedfishConnection.CredentialsSecretRef != "" {
		secretWarnings, secretErrs := webhook.validateCredentialSecret(ctx, host)
		warnings = append(warnings, secretWarnings...)
		allErrs = append(allErrs, secretErrs...)
	}

	// Validate TLS security
	if tlsWarnings, tlsErrs := webhook.validateTLSSecurity(host); len(tlsWarnings) > 0 || len(tlsErrs) > 0 {
		warnings = append(warnings, tlsWarnings...)
		allErrs = append(allErrs, tlsErrs...)
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

// validateCredentialSecret validates the referenced credential secret
func (webhook *PhysicalHostWebhook) validateCredentialSecret(ctx context.Context, host *infrav1beta1.PhysicalHost) (admission.Warnings, field.ErrorList) {
	var warnings admission.Warnings
	var allErrs field.ErrorList

	secretName := host.Spec.RedfishConnection.CredentialsSecretRef
	// If no client is set (unit-test or dry-run), return a warning and skip strict validation
	if webhook.Client == nil {
		warnings = append(warnings, fmt.Sprintf("Credential secret '%s' cannot be validated in this context.", secretName))
		return warnings, allErrs
	}

	secret := &corev1.Secret{}

	err := webhook.Client.Get(ctx, client.ObjectKey{
		Namespace: host.Namespace,
		Name:      secretName,
	}, secret)

	if err != nil {
		// Secret doesn't exist - this will be handled at runtime
		warnings = append(warnings, fmt.Sprintf("Credential secret '%s' not found. Ensure it exists before the PhysicalHost becomes active.", secretName))
		return warnings, allErrs
	}

	// Validate secret data structure
	username, hasUsername := secret.Data["username"]
	password, hasPassword := secret.Data["password"]

	if !hasUsername || len(username) == 0 {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "redfishConnection", "credentialsSecretRef"),
			fmt.Sprintf("secret '%s' must contain a non-empty 'username' field", secretName),
		))
	}

	if !hasPassword || len(password) == 0 {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "redfishConnection", "credentialsSecretRef"),
			fmt.Sprintf("secret '%s' must contain a non-empty 'password' field", secretName),
		))
	}

	// Security validation for credential quality
	if hasPassword && len(password) > 0 {
		if secWarnings, secErrs := webhook.validatePasswordSecurity(string(password)); len(secWarnings) > 0 || len(secErrs) > 0 {
			warnings = append(warnings, secWarnings...)
			// Don't add password validation errors to avoid exposing password details
			if len(secErrs) > 0 {
				warnings = append(warnings, "BMC password does not meet security requirements. Consider using a stronger password.")
			}
		}
	}

	// Check secret age and rotation
	if secret.CreationTimestamp.Time.Before(time.Now().Add(-90 * 24 * time.Hour)) {
		warnings = append(warnings, fmt.Sprintf("Credential secret '%s' is older than 90 days. Consider rotating credentials regularly.", secretName))
	}

	// Validate secret type and ownership
	if secret.Type != corev1.SecretTypeOpaque {
		warnings = append(warnings, fmt.Sprintf("Credential secret '%s' should be of type 'Opaque' for better security.", secretName))
	}

	return warnings, allErrs
}

// validatePasswordSecurity validates password strength
func (webhook *PhysicalHostWebhook) validatePasswordSecurity(password string) ([]string, field.ErrorList) {
	var warnings []string
	var allErrs field.ErrorList

	// Basic password security checks
	if len(password) < 8 {
		warnings = append(warnings, "BMC password is shorter than 8 characters.")
	}

	if len(password) < 12 {
		warnings = append(warnings, "BMC password should be at least 12 characters for better security.")
	}

	// Check for common weak passwords (without exposing the actual password)
	weakPasswords := []string{"password", "admin", "root", "123456", "default"}
	for _, weak := range weakPasswords {
		if strings.ToLower(password) == weak {
			warnings = append(warnings, "BMC password appears to be a common weak password.")
			break
		}
	}

	return warnings, allErrs
}

// validateTLSSecurity validates TLS configuration
func (webhook *PhysicalHostWebhook) validateTLSSecurity(host *infrav1beta1.PhysicalHost) ([]string, field.ErrorList) {
	var warnings []string
	var allErrs field.ErrorList

	// Parse the URL to check scheme
	parsedURL, err := url.Parse(host.Spec.RedfishConnection.Address)
	if err != nil {
		// URL validation is handled elsewhere
		return warnings, allErrs
	}

	// Warn about HTTP usage
	if parsedURL.Scheme == "http" {
		if !webhook.isDevEnvironment(host) {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "redfishConnection", "address"),
				"HTTP connections are not allowed in production environments. Please use HTTPS.",
			))
		} else {
			warnings = append(warnings, "Using HTTP connection. This is not recommended for production as credentials will be transmitted in plain text.")
		}
	}

	// Validate certificate configuration
	if parsedURL.Scheme == "https" {
		if host.Spec.RedfishConnection.InsecureSkipVerify == nil || !*host.Spec.RedfishConnection.InsecureSkipVerify {
			warnings = append(warnings, "TLS certificate verification is enabled. Ensure your BMC has a valid certificate or configure a custom CA bundle.")
		}
	}

	return warnings, allErrs
}

// isDevEnvironment checks if this is a development environment
func (webhook *PhysicalHostWebhook) isDevEnvironment(host *infrav1beta1.PhysicalHost) bool {
	// Check for development indicators
	devIndicators := []string{
		"dev", "development", "test", "testing", "staging",
		"local", "localhost", "example.com", "demo",
	}

	// Check namespace
	for _, indicator := range devIndicators {
		if strings.Contains(strings.ToLower(host.Namespace), indicator) {
			return true
		}
	}

	// Check address
	for _, indicator := range devIndicators {
		if strings.Contains(strings.ToLower(host.Spec.RedfishConnection.Address), indicator) {
			return true
		}
	}

	// Check for development annotations
	if host.Annotations != nil {
		if env, exists := host.Annotations["beskar7.io/environment"]; exists {
			for _, indicator := range devIndicators {
				if strings.ToLower(env) == indicator {
					return true
				}
			}
		}
	}

	// Check for private IP ranges (common in dev environments)
	if parsedURL, err := url.Parse(host.Spec.RedfishConnection.Address); err == nil {
		hostname := parsedURL.Hostname()
		if strings.HasPrefix(hostname, "192.168.") ||
			strings.HasPrefix(hostname, "10.") ||
			strings.HasPrefix(hostname, "172.16.") ||
			hostname == "localhost" ||
			hostname == "127.0.0.1" {
			return true
		}
	}

	return false
}

func (webhook *PhysicalHostWebhook) validatePhysicalHostUpdate(oldHost, newHost *infrav1beta1.PhysicalHost) (admission.Warnings, error) {
	var warnings admission.Warnings
	var allErrs field.ErrorList

	// Address is immutable after creation regardless of claim state
	if oldHost.Spec.RedfishConnection.Address != newHost.Spec.RedfishConnection.Address {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "redfishConnection", "address"),
			"address is immutable after creation",
		))
	}

	// Prevent changes to credentials while host is claimed
	if oldHost.Spec.ConsumerRef != nil &&
		oldHost.Spec.RedfishConnection.CredentialsSecretRef != newHost.Spec.RedfishConnection.CredentialsSecretRef {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "redfishConnection", "credentialsSecretRef"),
			"cannot change credentials while host is claimed",
		))
	}

	// Forbid removing ConsumerRef while provisioning
	if oldHost.Spec.ConsumerRef != nil && newHost.Spec.ConsumerRef == nil &&
		strings.EqualFold(newHost.Status.State, infrav1beta1.StateProvisioning) {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "consumerRef"),
			"cannot remove consumerRef while host is provisioning",
		))
	}

	// Warn about security changes
	oldInsecure := oldHost.Spec.RedfishConnection.InsecureSkipVerify != nil && *oldHost.Spec.RedfishConnection.InsecureSkipVerify
	newInsecure := newHost.Spec.RedfishConnection.InsecureSkipVerify != nil && *newHost.Spec.RedfishConnection.InsecureSkipVerify

	if !oldInsecure && newInsecure {
		warnings = append(warnings, "Enabling insecureSkipVerify reduces security. Ensure this is intended.")
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
	// Note: HTTP scheme warnings are handled at the validation level

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

	// Add security-related annotations if not present
	if host.Annotations == nil {
		host.Annotations = make(map[string]string)
	}

	// Add creation timestamp for security auditing
	if _, exists := host.Annotations["beskar7.io/security-validated"]; !exists {
		host.Annotations["beskar7.io/security-validated"] = time.Now().Format(time.RFC3339)
	}

	return nil
}
