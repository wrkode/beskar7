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

	"github.com/pkg/errors"
	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Beskar7MachineReconciler reconciles a Beskar7Machine object
type Beskar7MachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch // Needed to interact with Machine object
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,verbs=get;list;watch;patch // Needed to find and claim/patch PhysicalHost
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch // Needed for UserData

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Beskar7MachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling Beskar7Machine", "Name", req.Name, "Namespace", req.Namespace)

	// Fetch the Beskar7Machine instance.
	b7machine := &infrastructurev1alpha1.Beskar7Machine{}
	if err := r.Get(ctx, req.NamespacedName, b7machine); err != nil {
		// Handle error
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the Machine instance.
	machine, err := util.GetOwnerMachine(ctx, r.Client, b7machine.ObjectMeta)
	if err != nil {
		// Handle error
		return ctrl.Result{}, errors.Wrapf(err, "failed to get owner machine")
	}
	if machine == nil {
		logger.Info("Machine Controller is not yet set owner ref to Beskar7Machine")
		return ctrl.Result{}, nil
	}

	// Fetch the Cluster instance.
	// cluster, err := util.GetOwnerCluster(ctx, r.Client, machine.ObjectMeta)
	// if err != nil {
	// 	// Handle error
	// 	return ctrl.Result{}, errors.Wrapf(err, "failed to get cluster")
	// }

	// TODO(user): your logic here
	// 1. Handle deletion and finalizers
	// 2. Check if already provisioned (ProviderID set?)
	// 3. Find available PhysicalHost (based on selectors? TBD)
	// 4. Claim PhysicalHost by setting annotation/label and patching it.
	// 5. Fetch UserData secret if specified.
	// 6. Update PhysicalHost spec (e.g., BootISOSource = b7machine.Spec.Image.URL, CloudInitDataRef = UserData)
	// 7. Wait for PhysicalHost to become Ready/Provisioned (check PhysicalHost status)
	// 8. Once provisioned, set ProviderID on Beskar7Machine
	// 9. Update Beskar7Machine Status (Ready, Addresses, Phase, Conditions)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Beskar7MachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.Beskar7Machine{}).
		// TODO: Add watches for CAPI Machine, PhysicalHost?
		Complete(r)
}
