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
	"strings"
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

//+kubebuilder:rbac:groups=infrastructure.beskar7.io,resources=beskar7clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.beskar7.io,resources=beskar7clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.beskar7.io,resources=beskar7clusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch
//+kubebuilder:rbac:groups=infrastructure.beskar7.io,resources=physicalhosts,verbs=get;list;watch

// Beskar7ClusterReconciler reconciles a Beskar7Cluster object
type Beskar7ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Config holds the controller configuration
	Config *config.Config
}

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

	// Handle deletion reconciliation loop.
	if !b7cluster.DeletionTimestamp.IsZero() {
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

	// --- Validate Failure Domain Labels ---
	if len(b7cluster.Spec.FailureDomainLabels) > 0 {
		for _, label := range b7cluster.Spec.FailureDomainLabels {
			// Validate label format according to Kubernetes label requirements
			if !isValidLabelKey(label) {
				logger.Error(nil, "Invalid failure domain label format", "label", label)
				conditions.MarkFalse(b7cluster, infrastructurev1alpha1.FailureDomainsReady, infrastructurev1alpha1.InvalidFailureDomainLabelReason, clusterv1.ConditionSeverityError, "Invalid failure domain label format: %s", label)
				return ctrl.Result{}, nil
			}
		}
	}
	// --- End Validate Failure Domain Labels ---

	// --- Reconcile Failure Domains ---
	// Discover available failure domains from PhysicalHosts in the same namespace.
	// Assumes hosts are labeled with `topology.kubernetes.io/zone=<zone-name>`.
	phList := &infrastructurev1alpha1.PhysicalHostList{}
	listOpts := []client.ListOption{client.InNamespace(b7cluster.Namespace)}
	if err := r.List(ctx, phList, listOpts...); err != nil {
		logger.Error(err, "Failed to list PhysicalHosts to determine failure domains")
		conditions.MarkFalse(b7cluster, infrastructurev1alpha1.FailureDomainsReady, infrastructurev1alpha1.ListFailedReason, clusterv1.ConditionSeverityError, "Failed to list PhysicalHosts: %v", err)
		return ctrl.Result{}, err
	}

	failureDomains := make(clusterv1.FailureDomains)
	// Use custom labels if specified, otherwise use default
	zoneLabels := []string{"topology.kubernetes.io/zone"} // Default zone label
	if len(b7cluster.Spec.FailureDomainLabels) > 0 {
		zoneLabels = b7cluster.Spec.FailureDomainLabels
		logger.V(1).Info("Using custom failure domain labels", "labels", zoneLabels)
	}

	// Track which hosts have been processed to avoid duplicates
	processedHosts := make(map[string]bool)

	for _, ph := range phList.Items {
		if ph.Labels == nil {
			continue
		}

		// Try each label in order until we find a match
		for _, label := range zoneLabels {
			if zone, ok := ph.Labels[label]; ok && zone != "" {
				// Skip if we've already processed this host
				if processedHosts[ph.Name] {
					break
				}

				if _, domainExists := failureDomains[zone]; !domainExists {
					logger.V(1).Info("Discovered failure domain zone", "zone", zone, "label", label, "host", ph.Name)
					failureDomains[zone] = clusterv1.FailureDomainSpec{
						ControlPlane: true, // Assume all discovered zones can host control plane for now
					}
				}
				processedHosts[ph.Name] = true
				break // Found a matching label, no need to check others
			}
		}
	}

	// Update status with discovered failure domains
	if len(failureDomains) > 0 {
		b7cluster.Status.FailureDomains = failureDomains
		conditions.MarkTrue(b7cluster, infrastructurev1alpha1.FailureDomainsReady)
		logger.Info("Updated cluster status with discovered failure domains", "count", len(failureDomains), "labels", zoneLabels)
	} else {
		// Clear failure domains if none are found
		b7cluster.Status.FailureDomains = nil
		conditions.MarkFalse(b7cluster, infrastructurev1alpha1.FailureDomainsReady, infrastructurev1alpha1.NoFailureDomainsReason, clusterv1.ConditionSeverityWarning, "No PhysicalHosts with zone labels found in namespace with labels: %v", zoneLabels)
		logger.Info("No PhysicalHosts with zone labels found in namespace", "labels", zoneLabels)
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

		// Get the associated Beskar7Machine
		b7machine := &infrastructurev1alpha1.Beskar7Machine{}
		b7machineKey := client.ObjectKey{
			Namespace: machine.Namespace,
			Name:      machine.Spec.InfrastructureRef.Name,
		}
		if err := r.Get(ctx, b7machineKey, b7machine); err != nil {
			logger.V(1).Info("Skipping machine, failed to get Beskar7Machine", "machine", machine.Name, "error", err)
			continue
		}

		// First try to use the preferred IP from Beskar7Machine status
		if b7machine.Status.IPAddresses.PreferredIP != "" {
			logger.Info("Using preferred IP from Beskar7Machine status", "machine", machine.Name, "ip", b7machine.Status.IPAddresses.PreferredIP)
			return &clusterv1.APIEndpoint{
				Host: b7machine.Status.IPAddresses.PreferredIP,
				Port: 6443, // Default Kubernetes API server port
			}, nil
		}

		// If no preferred IP, try to use internal IPs
		if len(b7machine.Status.IPAddresses.InternalIPs) > 0 {
			logger.Info("Using first internal IP from Beskar7Machine status", "machine", machine.Name, "ip", b7machine.Status.IPAddresses.InternalIPs[0])
			return &clusterv1.APIEndpoint{
				Host: b7machine.Status.IPAddresses.InternalIPs[0],
				Port: 6443,
			}, nil
		}

		// If no internal IPs, try to use external IPs
		if len(b7machine.Status.IPAddresses.ExternalIPs) > 0 {
			logger.Info("Using first external IP from Beskar7Machine status", "machine", machine.Name, "ip", b7machine.Status.IPAddresses.ExternalIPs[0])
			return &clusterv1.APIEndpoint{
				Host: b7machine.Status.IPAddresses.ExternalIPs[0],
				Port: 6443,
			}, nil
		}

		// Fallback to the original Machine.Status.Addresses if no IPs in Beskar7Machine status
		if len(machine.Status.Addresses) > 0 {
			// Try to find an internal IP first
			for _, addr := range machine.Status.Addresses {
				if addr.Type == clusterv1.MachineInternalIP {
					logger.Info("Using internal IP from Machine status", "machine", machine.Name, "ip", addr.Address)
					return &clusterv1.APIEndpoint{
						Host: addr.Address,
						Port: 6443,
					}, nil
				}
			}
			// Fallback to the first address
			logger.Info("Using first address from Machine status", "machine", machine.Name, "ip", machine.Status.Addresses[0].Address)
			return &clusterv1.APIEndpoint{
				Host: machine.Status.Addresses[0].Address,
				Port: 6443,
			}, nil
		}

		logger.V(1).Info("Skipping machine, no suitable IP addresses found", "machine", machine.Name)
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

// isValidLabelKey checks if a string is a valid Kubernetes label key
func isValidLabelKey(key string) bool {
	// Kubernetes label keys must:
	// 1. Be a valid DNS subdomain
	// 2. Consist of alphanumeric characters, '-', '_' or '.'
	// 3. Start and end with an alphanumeric character
	// 4. Not contain consecutive dots
	// 5. Not be empty
	if len(key) == 0 {
		return false
	}

	// Check for consecutive dots
	if strings.Contains(key, "..") {
		return false
	}

	// Check first and last character
	if !isAlphanumeric(rune(key[0])) || !isAlphanumeric(rune(key[len(key)-1])) {
		return false
	}

	// Check all characters
	for _, c := range key {
		if !isAlphanumeric(c) && c != '-' && c != '_' && c != '.' {
			return false
		}
	}

	return true
}

// isAlphanumeric checks if a rune is alphanumeric
func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
