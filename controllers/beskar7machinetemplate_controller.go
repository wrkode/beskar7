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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	internalmetrics "github.com/wrkode/beskar7/internal/metrics"
)

const (
	// Beskar7MachineTemplateFinalizer allows Beskar7MachineTemplateReconciler to clean up resources associated with Beskar7MachineTemplate before removing it from the apiserver.
	Beskar7MachineTemplateFinalizer = "beskar7machinetemplate.infrastructure.cluster.x-k8s.io"
)

// Beskar7MachineTemplateReconciler reconciles a Beskar7MachineTemplate object
type Beskar7MachineTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machinetemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machinetemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machinetemplates/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines,verbs=get;list;watch // Needed to find machines created from this template

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Beskar7MachineTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	startTime := time.Now()
	log := r.Log.WithValues("beskar7machinetemplate", req.NamespacedName)
	log.Info("Starting reconciliation")

	// Initialize outcome tracking for metrics
	outcome := internalmetrics.ReconciliationOutcomeSuccess
	var errorType internalmetrics.ErrorType

	// Record reconciliation attempt and duration at the end
	defer func() {
		duration := time.Since(startTime)
		internalmetrics.RecordReconciliation("beskar7machinetemplate", req.Namespace, outcome, duration)

		// Record errors if any occurred
		if reterr != nil {
			internalmetrics.RecordError("beskar7machinetemplate", req.Namespace, errorType)
		}
	}()

	// Fetch the Beskar7MachineTemplate instance.
	template := &infrastructurev1beta1.Beskar7MachineTemplate{}
	err := r.Get(ctx, req.NamespacedName, template)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Beskar7MachineTemplate resource not found. Ignoring since object must be deleted")
			outcome = internalmetrics.ReconciliationOutcomeNotFound
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch Beskar7MachineTemplate")
		outcome = internalmetrics.ReconciliationOutcomeError
		errorType = internalmetrics.ErrorTypeUnknown
		return ctrl.Result{}, err
	}

	// Check if the Beskar7MachineTemplate is paused
	if isPaused(template) {
		log.Info("Beskar7MachineTemplate reconciliation is paused")
		return ctrl.Result{}, nil
	}

	// Handle deletion reconciliation
	if !template.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, template)
	}

	// Handle non-deletion reconciliation
	return r.reconcileNormal(ctx, log, template)
}

// reconcileNormal handles the logic when the Beskar7MachineTemplate is not being deleted.
func (r *Beskar7MachineTemplateReconciler) reconcileNormal(ctx context.Context, logger logr.Logger, template *infrastructurev1beta1.Beskar7MachineTemplate) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7MachineTemplate create/update")

	// If the Beskar7MachineTemplate doesn't have our finalizer, add it.
	if controllerutil.AddFinalizer(template, Beskar7MachineTemplateFinalizer) {
		logger.Info("Adding finalizer")
		if err := r.Update(ctx, template); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Validate template integrity
	if err := r.validateTemplate(ctx, logger, template); err != nil {
		logger.Error(err, "Template validation failed")
		return ctrl.Result{}, err
	}

	// For templates, we mainly ensure they exist and are valid
	// The actual work is done by webhook validation and by consumers of the template
	logger.Info("Beskar7MachineTemplate reconciliation complete")
	return ctrl.Result{}, nil
}

// reconcileDelete handles the logic when the Beskar7MachineTemplate is marked for deletion.
func (r *Beskar7MachineTemplateReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, template *infrastructurev1beta1.Beskar7MachineTemplate) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7MachineTemplate deletion")

	// Check if there are any Beskar7Machines still referencing this template
	if err := r.checkForReferencingMachines(ctx, logger, template); err != nil {
		logger.Error(err, "Failed to check for referencing machines")
		return ctrl.Result{}, err
	}

	// Perform any cleanup operations here if needed
	// For templates, cleanup is typically minimal since they're mostly immutable configuration

	// Remove the finalizer to allow deletion
	if controllerutil.RemoveFinalizer(template, Beskar7MachineTemplateFinalizer) {
		logger.Info("Removing finalizer")
		if err := r.Update(ctx, template); err != nil {
			logger.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// validateTemplate performs additional validation on the template
func (r *Beskar7MachineTemplateReconciler) validateTemplate(ctx context.Context, logger logr.Logger, template *infrastructurev1beta1.Beskar7MachineTemplate) error {
	// Basic validation - ensure template spec is not empty
	if template.Spec.Template.Spec.ImageURL == "" {
		logger.Error(nil, "Template validation failed: ImageURL is required")
		return apierrors.NewInvalid(
			template.GroupVersionKind().GroupKind(),
			template.Name,
			nil, // field.ErrorList can be nil for simple validation
		)
	}

	if template.Spec.Template.Spec.OSFamily == "" {
		logger.Error(nil, "Template validation failed: OSFamily is required")
		return apierrors.NewInvalid(
			template.GroupVersionKind().GroupKind(),
			template.Name,
			nil,
		)
	}

	// Additional validation can be added here
	// For example, checking if the ImageURL is accessible, validating OSFamily values, etc.

	logger.V(1).Info("Template validation passed")
	return nil
}

// checkForReferencingMachines checks if any Beskar7Machines are still using this template
func (r *Beskar7MachineTemplateReconciler) checkForReferencingMachines(ctx context.Context, logger logr.Logger, template *infrastructurev1beta1.Beskar7MachineTemplate) error {
	// List all Beskar7Machines in the same namespace
	machineList := &infrastructurev1beta1.Beskar7MachineList{}
	if err := r.List(ctx, machineList, client.InNamespace(template.Namespace)); err != nil {
		return err
	}

	// Check if any machines reference this template through owner references or other means
	var referencingMachines []string
	for _, machine := range machineList.Items {
		// Check owner references
		for _, ownerRef := range machine.OwnerReferences {
			if ownerRef.Kind == "Beskar7MachineTemplate" && ownerRef.Name == template.Name {
				referencingMachines = append(referencingMachines, machine.Name)
				break
			}
		}
	}

	if len(referencingMachines) > 0 {
		logger.Info("Template is still referenced by machines, waiting for cleanup",
			"referencingMachines", referencingMachines)
		// This will cause the reconcile to wait before allowing deletion
		return apierrors.NewConflict(
			infrastructurev1beta1.GroupVersion.WithResource("beskar7machinetemplates").GroupResource(),
			template.Name,
			fmt.Errorf("template is still referenced by machines: %v", referencingMachines),
		)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Beskar7MachineTemplateReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta1.Beskar7MachineTemplate{}).
		Watches(
			&infrastructurev1beta1.Beskar7Machine{},
			handler.EnqueueRequestsFromMapFunc(r.Beskar7MachineToTemplate),
		).
		WithOptions(options).
		Complete(r)
}

// Beskar7MachineToTemplate maps Beskar7Machine events to Beskar7MachineTemplate reconcile requests.
// This ensures that when machines are created/deleted, we reconcile the template to check references.
func (r *Beskar7MachineTemplateReconciler) Beskar7MachineToTemplate(ctx context.Context, obj client.Object) []reconcile.Request {
	machine, ok := obj.(*infrastructurev1beta1.Beskar7Machine)
	if !ok {
		return nil
	}

	var requests []reconcile.Request

	// Check if this machine has owner references to any Beskar7MachineTemplate
	for _, ownerRef := range machine.OwnerReferences {
		if ownerRef.Kind == "Beskar7MachineTemplate" &&
			ownerRef.APIVersion == "infrastructure.cluster.x-k8s.io/v1beta1" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: machine.Namespace,
					Name:      ownerRef.Name,
				},
			})
		}
	}

	if len(requests) > 0 {
		ctrl.LoggerFrom(ctx).V(1).Info("Beskar7Machine change mapped to Beskar7MachineTemplate reconcile requests",
			"machine", machine.Name, "requests", len(requests))
	}

	return requests
}
