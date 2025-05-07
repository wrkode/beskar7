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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	conditions "sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

const (
	// Beskar7ClusterFinalizer allows Beskar7ClusterReconciler to clean up resources associated with Beskar7Cluster before removing it from the apiserver.
	Beskar7ClusterFinalizer = "beskar7cluster.infrastructure.cluster.x-k8s.io"
)

// Beskar7ClusterReconciler reconciles a Beskar7Cluster object
type Beskar7ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7clusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch // Needed to interact with Cluster object

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Beskar7ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx).WithValues("beskar7cluster", req.NamespacedName)
	logger.Info("Starting reconciliation")

	// Fetch the Beskar7Cluster instance.
	b7cluster := &infrastructurev1alpha1.Beskar7Cluster{}
	if err := r.Get(ctx, req.NamespacedName, b7cluster); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Unable to fetch Beskar7Cluster")
			return ctrl.Result{}, err
		}
		logger.Info("Beskar7Cluster resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}

	// Fetch the Cluster instance that owns this Beskar7Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, b7cluster.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to get owner Cluster")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		logger.Info("Waiting for Cluster Controller to set OwnerRef on Beskar7Cluster")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	// Initialize patch helper.
	patchHelper, err := patch.NewHelper(b7cluster, r.Client)
	if err != nil {
		logger.Error(err, "Failed to init patch helper")
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the Beskar7Cluster object and status after reconciliation.
	defer func() {
		// Set the summary condition based on ControlPlaneEndpointReady
		conditions.SetSummary(b7cluster, conditions.WithConditions(infrastructurev1alpha1.ControlPlaneEndpointReady))

		if err := patchHelper.Patch(ctx, b7cluster); err != nil {
			logger.Error(err, "Failed to patch Beskar7Cluster")
			if reterr == nil {
				reterr = err
			}
		}
		logger.Info("Finished reconciliation")
	}()

	// Handle deletion reconciliation
	if !b7cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, logger, b7cluster)
	}

	// Handle non-deletion reconciliation
	return r.reconcileNormal(ctx, logger, cluster, b7cluster)
}

// reconcileNormal handles the main reconciliation logic for Beskar7Cluster.
func (r *Beskar7ClusterReconciler) reconcileNormal(ctx context.Context, logger logr.Logger, cluster *clusterv1.Cluster, b7cluster *infrastructurev1alpha1.Beskar7Cluster) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Cluster create/update")

	// If the Beskar7Cluster doesn't have our finalizer, add it.
	if controllerutil.AddFinalizer(b7cluster, Beskar7ClusterFinalizer) {
		logger.Info("Adding finalizer")
		return ctrl.Result{Requeue: true}, nil
	}

	// TODO: Check if paused

	// Reconcile ControlPlaneEndpoint
	if !b7cluster.Spec.ControlPlaneEndpoint.IsValid() {
		logger.Info("ControlPlaneEndpoint is not set or invalid in Spec, waiting.")
		conditions.MarkFalse(b7cluster, infrastructurev1alpha1.ControlPlaneEndpointReady, infrastructurev1alpha1.ControlPlaneEndpointNotSetReason, clusterv1.ConditionSeverityInfo, "ControlPlaneEndpoint is not set in spec")
		// When the ControlPlaneEndpoint is not ready, the cluster infrastructure is not ready.
		b7cluster.Status.Ready = false
		return ctrl.Result{}, nil // Don't requeue immediately, wait for external update to Spec or watch.
	}

	logger.Info("ControlPlaneEndpoint is set in Spec, marking infrastructure as ready", "host", b7cluster.Spec.ControlPlaneEndpoint.Host, "port", b7cluster.Spec.ControlPlaneEndpoint.Port)
	conditions.MarkTrue(b7cluster, infrastructurev1alpha1.ControlPlaneEndpointReady)
	// Note: Beskar7Cluster.Status.Ready is now managed by the summary condition // This comment is no longer fully accurate
	// Set Status.Ready to true explicitly when conditions are met.
	b7cluster.Status.Ready = true

	// TODO: Potentially reconcile other cluster-wide settings if needed

	return ctrl.Result{}, nil
}

// reconcileDelete handles the cleanup when a Beskar7Cluster is marked for deletion.
func (r *Beskar7ClusterReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, b7cluster *infrastructurev1alpha1.Beskar7Cluster) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Cluster deletion")

	// Mark conditions False
	conditions.MarkFalse(b7cluster, infrastructurev1alpha1.ControlPlaneEndpointReady, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Beskar7Cluster is being deleted")

	// TODO: Add any cluster-level cleanup logic here if this controller manages shared resources.
	// Typically, the infra cluster object itself doesn't own external resources.
	logger.Info("No cluster-specific cleanup required.")

	// Beskar7Cluster is deleted, remove the finalizer.
	if controllerutil.RemoveFinalizer(b7cluster, Beskar7ClusterFinalizer) {
		logger.Info("Removing finalizer")
		// Patching is handled by the deferred patch function in Reconcile.
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Beskar7ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.Beskar7Cluster{}).
		// TODO: Add watches for CAPI Cluster if needed?
		Complete(r)
}
