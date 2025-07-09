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
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// Beskar7ClusterFinalizer allows Beskar7ClusterReconciler to clean up resources associated with Beskar7Cluster before removing it from the apiserver.
	Beskar7ClusterFinalizer = "beskar7cluster.infrastructure.cluster.x-k8s.io"
)

// Beskar7ClusterReconciler reconciles a Beskar7Cluster object
type Beskar7ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7clusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch // Needed to find control plane machine addresses
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,verbs=get;list;watch // Needed to discover failure domains

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Beskar7ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := r.Log.WithValues("beskar7cluster", req.NamespacedName)
	log.Info("Starting reconciliation")

	// Fetch the Beskar7Cluster instance
	b7cluster := &infrastructurev1beta1.Beskar7Cluster{}
	err := r.Get(ctx, req.NamespacedName, b7cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Beskar7Cluster resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch Beskar7Cluster")
		return ctrl.Result{}, err
	}

	// Check if the Beskar7Cluster is paused
	if isPaused(b7cluster) {
		log.Info("Beskar7Cluster reconciliation is paused")
		return ctrl.Result{}, nil
	}

	// Set the ownerRefs on the Beskar7Cluster
	cluster, err := util.GetOwnerCluster(ctx, r.Client, b7cluster.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner Cluster")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef on Beskar7Cluster")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	// Check if the owner cluster is paused
	if isClusterPaused(cluster) {
		log.Info("Beskar7Cluster reconciliation is paused because owner cluster is paused")
		return ctrl.Result{}, nil
	}

	// Initialize patch helper.
	patchHelper, err := patch.NewHelper(b7cluster, r.Client)
	if err != nil {
		log.Error(err, "Failed to init patch helper")
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the Beskar7Cluster object and status after reconciliation.
	defer func() {
		// Set the summary condition based on ControlPlaneEndpointReady
		conditions.SetSummary(b7cluster, conditions.WithConditions(infrastructurev1beta1.ControlPlaneEndpointReady))

		if err := patchHelper.Patch(ctx, b7cluster); err != nil {
			log.Error(err, "Failed to patch Beskar7Cluster")
			if reterr == nil {
				reterr = err
			}
		}
		log.Info("Finished reconciliation")
	}()

	// Handle deletion reconciliation
	if !b7cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, b7cluster)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, log, cluster, b7cluster)
}

func (r *Beskar7ClusterReconciler) reconcileNormal(ctx context.Context, logger logr.Logger, cluster *clusterv1.Cluster, b7cluster *infrastructurev1beta1.Beskar7Cluster) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Cluster create/update")

	// If the Beskar7Cluster doesn't have our finalizer, add it.
	if controllerutil.AddFinalizer(b7cluster, Beskar7ClusterFinalizer) {
		logger.Info("Adding finalizer")
		return ctrl.Result{Requeue: true}, nil
	}

	// --- Reconcile Failure Domains ---
	if err := r.reconcileFailureDomains(ctx, logger, b7cluster); err != nil {
		// Treat failure to list PhysicalHosts as a transient error
		return ctrl.Result{}, err
	}

	// --- Reconcile ControlPlaneEndpoint ---
	if err := r.reconcileControlPlaneEndpoint(ctx, logger, cluster, b7cluster); err != nil {
		// Treat failure to find endpoint as a transient error
		return ctrl.Result{}, err
	}

	// If the endpoint is not ready, it will be set in the reconcileControlPlaneEndpoint function,
	// and we should requeue.
	if !b7cluster.Status.Ready {
		logger.Info("Control plane endpoint not yet available, requeuing")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	logger.Info("Beskar7Cluster reconciliation complete")
	return ctrl.Result{}, nil
}

func (r *Beskar7ClusterReconciler) reconcileFailureDomains(ctx context.Context, logger logr.Logger, b7cluster *infrastructurev1beta1.Beskar7Cluster) error {
	logger.Info("Reconciling failure domains")

	// Get current failure domains for comparison
	currentFailureDomains := b7cluster.Status.FailureDomains

	phList := &infrastructurev1beta1.PhysicalHostList{}
	if err := r.List(ctx, phList, client.InNamespace(b7cluster.Namespace)); err != nil {
		logger.Error(err, "Failed to list PhysicalHosts to determine failure domains")
		return errors.Wrapf(err, "failed to list PhysicalHosts in namespace %s", b7cluster.Namespace)
	}

	discoveredFailureDomains := make(clusterv1.FailureDomains)
	zoneLabel := "topology.kubernetes.io/zone"

	// Early return optimization: if no PhysicalHosts exist and no current domains, skip processing
	if len(phList.Items) == 0 {
		if len(currentFailureDomains) == 0 {
			logger.V(1).Info("No PhysicalHosts found and no existing failure domains, skipping update")
			return nil
		}
		// Clear failure domains since no PhysicalHosts exist
		b7cluster.Status.FailureDomains = nil
		logger.Info("Cleared failure domains - no PhysicalHosts found in namespace")
		return nil
	}

	// Discover failure domains from PhysicalHosts with zone labels
	for _, ph := range phList.Items {
		if ph.Labels != nil {
			if zone, ok := ph.Labels[zoneLabel]; ok && zone != "" {
				if _, domainExists := discoveredFailureDomains[zone]; !domainExists {
					logger.V(1).Info("Discovered failure domain zone", "zone", zone)
					discoveredFailureDomains[zone] = clusterv1.FailureDomainSpec{
						ControlPlane: true,
					}
				}
			}
		}
	}

	// Compare discovered domains with current domains to check if update is needed
	if failureDomainsEqual(currentFailureDomains, discoveredFailureDomains) {
		logger.V(1).Info("Failure domains unchanged, skipping status update",
			"count", len(discoveredFailureDomains))
		return nil
	}

	// Update status with discovered failure domains
	if len(discoveredFailureDomains) > 0 {
		b7cluster.Status.FailureDomains = discoveredFailureDomains
		logger.Info("Updated cluster status with discovered failure domains",
			"count", len(discoveredFailureDomains))
	} else {
		b7cluster.Status.FailureDomains = nil
		logger.Info("No PhysicalHosts with zone labels found in namespace")
	}

	return nil
}

// failureDomainsEqual compares two FailureDomains maps for equality
func failureDomainsEqual(a, b clusterv1.FailureDomains) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return len(a) == 0 && len(b) == 0
	}

	// Use reflect.DeepEqual for comprehensive comparison
	return reflect.DeepEqual(a, b)
}

func (r *Beskar7ClusterReconciler) reconcileControlPlaneEndpoint(ctx context.Context, logger logr.Logger, cluster *clusterv1.Cluster, b7cluster *infrastructurev1beta1.Beskar7Cluster) error {
	logger.Info("Reconciling control plane endpoint")

	cpEndpoint, err := r.findControlPlaneEndpoint(ctx, logger, cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to find control plane endpoint for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	if cpEndpoint == nil {
		conditions.MarkFalse(b7cluster, infrastructurev1beta1.ControlPlaneEndpointReady, infrastructurev1beta1.ControlPlaneEndpointNotSetReason, clusterv1.ConditionSeverityInfo, "Waiting for control plane Beskar7Machine(s) to have IP addresses")
		b7cluster.Status.Ready = false
		return nil
	}

	logger.Info("Control plane endpoint found", "host", cpEndpoint.Host, "port", cpEndpoint.Port)
	b7cluster.Status.ControlPlaneEndpoint = *cpEndpoint
	conditions.MarkTrue(b7cluster, infrastructurev1beta1.ControlPlaneEndpointReady)
	b7cluster.Status.Ready = true

	return nil
}

// findControlPlaneEndpoint searches for a ready control plane machine and extracts its IP.
func (r *Beskar7ClusterReconciler) findControlPlaneEndpoint(ctx context.Context, logger logr.Logger, cluster *clusterv1.Cluster) (*clusterv1.APIEndpoint, error) {
	logger.Info("Searching for control plane machine endpoint")

	machineList := &clusterv1.MachineList{}
	listOpts := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:       cluster.Name,
			"cluster.x-k8s.io/control-plane": "", // Use literal string for the label key
		},
	}

	if err := r.List(ctx, machineList, listOpts...); err != nil {
		logger.Error(err, "Failed to list machines to find control plane endpoint")
		return nil, err
	}

	if len(machineList.Items) == 0 {
		logger.Info("No control plane machines found yet")
		return nil, nil
	}

	// Find the first ready control plane machine with an address
	for _, machine := range machineList.Items {
		// Check if Machine has InfrastructureReady condition (implies Beskar7Machine is ready)
		if !conditions.IsTrue(&machine, clusterv1.InfrastructureReadyCondition) {
			logger.V(1).Info("Skipping machine, infrastructure not ready", "machine", machine.Name)
			continue
		}

		// Check if Machine has an address in its status
		if len(machine.Status.Addresses) == 0 {
			logger.V(1).Info("Skipping machine, no addresses found in status", "machine", machine.Name)
			continue
		}

		// Select the first available address (prefer internal IP, then external)
		var selectedAddress string
		for _, addr := range machine.Status.Addresses {
			if addr.Type == clusterv1.MachineInternalIP {
				selectedAddress = addr.Address
				break
			}
		}
		if selectedAddress == "" {
			selectedAddress = machine.Status.Addresses[0].Address // Fallback to the first address
		}

		logger.Info("Found suitable control plane machine endpoint", "machine", machine.Name, "address", selectedAddress)
		return &clusterv1.APIEndpoint{
			Host: selectedAddress,
			Port: 6443, // Default Kubernetes API server port
		}, nil
	}

	// No suitable machine found yet
	logger.Info("No ready control plane machines with addresses found yet")
	return nil, nil
}

// reconcileDelete handles the cleanup when a Beskar7Cluster is marked for deletion.
func (r *Beskar7ClusterReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, b7cluster *infrastructurev1beta1.Beskar7Cluster) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Cluster deletion")

	// Mark conditions False
	conditions.MarkFalse(b7cluster, infrastructurev1beta1.ControlPlaneEndpointReady, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Beskar7Cluster is being deleted")

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
func (r *Beskar7ClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta1.Beskar7Cluster{}).
		Watches(
			&infrastructurev1beta1.PhysicalHost{},
			handler.EnqueueRequestsFromMapFunc(r.PhysicalHostToBeskar7Clusters),
		).
		Complete(r)
}

// PhysicalHostToBeskar7Clusters maps a PhysicalHost event to reconcile requests for all Beskar7Clusters in the same namespace.
func (r *Beskar7ClusterReconciler) PhysicalHostToBeskar7Clusters(ctx context.Context, obj client.Object) []reconcile.Request {
	log := r.Log.WithValues("mapping", "PhysicalHostToBeskar7Clusters")
	physicalHost, ok := obj.(*infrastructurev1beta1.PhysicalHost)
	if !ok {
		log.Error(errors.New("unexpected type"), "Expected a PhysicalHost but got a %T", obj)
		return nil
	}

	clusterList := &infrastructurev1beta1.Beskar7ClusterList{}
	if err := r.List(ctx, clusterList, client.InNamespace(physicalHost.Namespace)); err != nil {
		log.Error(err, "failed to list Beskar7Clusters in namespace", "namespace", physicalHost.Namespace)
		return nil
	}

	requests := make([]reconcile.Request, len(clusterList.Items))
	for i, cluster := range clusterList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
	}
	log.V(1).Info("Triggering reconciliation for Beskar7Clusters in namespace", "namespace", physicalHost.Namespace, "count", len(requests))
	return requests
}
