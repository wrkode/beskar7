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

// Beskar7ClusterWebhook implements a validating and defaulting webhook for Beskar7Cluster.
type Beskar7ClusterWebhook struct{}

// SetupWebhookWithManager sets up the webhook with the manager.
func (webhook *Beskar7ClusterWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1beta1.Beskar7Cluster{}).
		WithValidator(webhook).
		WithDefaulter(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-beskar7cluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=beskar7clusters,versions=v1beta1,name=validation.beskar7.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-beskar7cluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=beskar7clusters,versions=v1beta1,name=defaulting.beskar7.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.CustomValidator = &Beskar7ClusterWebhook{}
var _ webhook.CustomDefaulter = &Beskar7ClusterWebhook{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7ClusterWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cluster := obj.(*infrav1beta1.Beskar7Cluster)
	return nil, webhook.validateCluster(cluster)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7ClusterWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newCluster := newObj.(*infrav1beta1.Beskar7Cluster)
	return nil, webhook.validateCluster(newCluster)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Beskar7ClusterWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *Beskar7ClusterWebhook) Default(ctx context.Context, obj runtime.Object) error {
	cluster := obj.(*infrav1beta1.Beskar7Cluster)
	return webhook.defaultCluster(cluster)
}

func (webhook *Beskar7ClusterWebhook) validateCluster(cluster *infrav1beta1.Beskar7Cluster) error {
	var allErrs field.ErrorList

	// Validate control plane endpoint
	if cluster.Spec.ControlPlaneEndpoint.Host == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "controlPlaneEndpoint", "host"),
			"host is required",
		))
	}

	if cluster.Spec.ControlPlaneEndpoint.Port <= 0 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "controlPlaneEndpoint", "port"),
			cluster.Spec.ControlPlaneEndpoint.Port,
			"port must be greater than 0",
		))
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			cluster.GroupVersionKind().GroupKind(),
			cluster.Name,
			allErrs,
		)
	}

	return nil
}

func (webhook *Beskar7ClusterWebhook) defaultCluster(cluster *infrav1beta1.Beskar7Cluster) error {
	// Set default port if not specified
	if cluster.Spec.ControlPlaneEndpoint.Port == 0 {
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
	}

	return nil
}
