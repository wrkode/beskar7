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
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	"github.com/wrkode/beskar7/internal/config"
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
	// Config holds the controller configuration
	Config *config.Config
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

	// --- Reconcile Failure Domains ---
	// Discover available failure domains from PhysicalHosts in the same namespace.
	// Assumes hosts are labeled with `topology.kubernetes.io/zone=<zone-name>`.
	phList := &infrastructurev1alpha1.PhysicalHostList{}
	listOpts := []client.ListOption{client.InNamespace(b7cluster.Namespace)}
	if err := r.List(ctx, phList, listOpts...); err != nil {
		logger.Error(err, "Failed to list PhysicalHosts to determine failure domains")
		// Continue reconciliation without failure domains if listing fails?
		// Or return error? For now, log and continue, but consider returning err.
	} else {
		failureDomains := make(clusterv1.FailureDomains)
		zoneLabel := "topology.kubernetes.io/zone" // Standard zone label

		for _, ph := range phList.Items {
			if ph.Labels != nil {
				if zone, ok := ph.Labels[zoneLabel]; ok && zone != "" {
					if _, domainExists := failureDomains[zone]; !domainExists {
						logger.V(1).Info("Discovered failure domain zone", "zone", zone)
						failureDomains[zone] = clusterv1.FailureDomainSpec{
							ControlPlane: true, // Assume all discovered zones can host control plane for now
						}
					}
				}
			}
		}
		// TODO: Check if failureDomains actually changed before updating status?
		if len(failureDomains) > 0 {
			b7cluster.Status.FailureDomains = failureDomains
			logger.Info("Updated cluster status with discovered failure domains", "count", len(failureDomains))
		} else {
			// Clear failure domains if none are found (optional, depends on desired behavior)
			// b7cluster.Status.FailureDomains = nil
			logger.Info("No PhysicalHosts with zone labels found in namespace")
		}
	}
	// --- End Reconcile Failure Domains ---

	// --- Reconcile ControlPlaneEndpoint ---
	// Derive endpoint from control plane machines instead of spec.
	cpEndpoint, err := r.findControlPlaneEndpoint(ctx, logger, cluster)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to find control plane endpoint for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	if cpEndpoint == nil {
		logger.Info("Control plane endpoint not yet available, waiting for control plane machines.")
		conditions.MarkFalse(b7cluster, infrastructurev1alpha1.ControlPlaneEndpointReady, infrastructurev1alpha1.ControlPlaneEndpointNotSetReason, clusterv1.ConditionSeverityInfo, "Waiting for control plane Beskar7Machine(s) to have IP addresses")
		b7cluster.Status.Ready = false
		// Requeue after a delay to check again
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Endpoint found, update status
	logger.Info("Control plane endpoint found", "host", cpEndpoint.Host, "port", cpEndpoint.Port)
	b7cluster.Status.ControlPlaneEndpoint = *cpEndpoint
	conditions.MarkTrue(b7cluster, infrastructurev1alpha1.ControlPlaneEndpointReady)
	b7cluster.Status.Ready = true

	logger.Info("Beskar7Cluster reconciliation complete")
	return ctrl.Result{}, nil
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
		// TODO: Add more sophisticated address selection logic if needed (IPv4 vs IPv6?)
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
