// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package specsync

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"open-cluster-management.io/governance-policy-propagator/controllers/common"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"open-cluster-management.io/governance-policy-framework-addon/controllers/uninstall"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/utils"
)

const ControllerName string = "policy-spec-sync"

var log = logf.Log.WithName(ControllerName)

// SetupWithManager sets up the controller with the Manager.
func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager, additionalSource source.Source) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&policiesv1.Policy{}).
		Named(ControllerName).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.ConcurrentReconciles})

	if additionalSource != nil {
		builder = builder.WatchesRawSource(additionalSource)
	}

	return builder.Complete(r)
}

// blank assignment to verify that ReconcilePolicy implements reconcile.Reconciler
var _ reconcile.Reconciler = &PolicyReconciler{}

// ReconcilePolicy reconciles a Policy object
type PolicyReconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	HubClient       client.Client
	ManagedClient   client.Client
	ManagedRecorder record.EventRecorder
	Scheme          *runtime.Scheme
	// The namespace that the replicated policies should be synced to.
	TargetNamespace      string
	ConcurrentReconciles int
	// StatusSyncRequests triggers status-sync controller reconciles based on what is observed on the hub
	StatusSyncRequests chan<- event.GenericEvent
}

//+kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=policies,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=policies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=policies/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;delete;get;list;patch;update;watch
// This is required for the status lease for the addon framework
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list

// Reconcile reads that state of the cluster for a Policy object and makes changes based on the state read
// and what is in the Policy.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *PolicyReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues(
		"Request.Namespace", request.Namespace, "Request.Name", request.Name, "TargetNamespace", r.TargetNamespace,
	)

	if uninstall.DeploymentIsUninstalling {
		reqLogger.Info("Skipping reconcile because the deployment is in uninstallation mode")

		return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	reqLogger.V(1).Info("Reconciling Policy...")

	// Fetch the Policy instance
	instance := &policiesv1.Policy{}

	err := r.HubClient.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// replicated policy on hub was deleted, remove policy on managed cluster
			reqLogger.Info("Policy was deleted, removing on managed cluster...")

			err = r.ManagedClient.Delete(ctx, &policiesv1.Policy{
				TypeMeta: metav1.TypeMeta{
					Kind:       policiesv1.Kind,
					APIVersion: policiesv1.SchemeGroupVersion.Group,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      request.Name,
					Namespace: r.TargetNamespace,
				},
			})

			if err != nil && !errors.IsNotFound(err) {
				reqLogger.Error(err, "Failed to remove policy on managed cluster...")

				return reconcile.Result{}, err
			}

			reqLogger.Info("Policy has been removed from managed cluster...Reconciliation complete.")

			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get policy from hub...")

		return reconcile.Result{}, err
	}

	managedPlc := &policiesv1.Policy{}

	err = r.ManagedClient.Get(ctx, types.NamespacedName{Namespace: r.TargetNamespace, Name: request.Name}, managedPlc)
	if err != nil {
		if errors.IsNotFound(err) {
			// not found on managed cluster, create it
			reqLogger.Info("Policy not found on managed cluster, creating it...")

			managedPlc = instance.DeepCopy()
			managedPlc.Namespace = r.TargetNamespace

			if managedPlc.Labels[common.ClusterNamespaceLabel] != "" {
				managedPlc.Labels[common.ClusterNamespaceLabel] = r.TargetNamespace
			}

			managedPlc.SetOwnerReferences(nil)
			managedPlc.SetResourceVersion("")

			err = r.ManagedClient.Create(ctx, managedPlc)
			if err != nil {
				reqLogger.Error(err, "Failed to create policy on managed...")

				return reconcile.Result{}, err
			}

			r.ManagedRecorder.Event(managedPlc, "Normal", "PolicySpecSync",
				fmt.Sprintf("Policy %s was synchronized to cluster namespace %s", instance.GetName(),
					r.TargetNamespace))
		} else {
			reqLogger.Error(err, "Failed to get policy from managed...")

			return reconcile.Result{}, err
		}
	}
	// found, then compare and update
	if !utils.EquivalentReplicatedPolicies(instance, managedPlc) {
		// update needed
		reqLogger.Info("Policy mismatch between hub and managed, updating it...")
		managedPlc.SetAnnotations(instance.GetAnnotations())
		managedPlc.Spec = instance.Spec
		err = r.ManagedClient.Update(ctx, managedPlc)

		if err != nil && errors.IsNotFound(err) {
			reqLogger.Error(err, "Failed to update policy on managed...")

			return reconcile.Result{}, err
		}

		r.ManagedRecorder.Event(managedPlc, "Normal", "PolicySpecSync",
			fmt.Sprintf("Policy %s was updated in cluster namespace %s", instance.GetName(),
				r.TargetNamespace))
	} else if !equality.Semantic.DeepEqual(instance.Status, managedPlc.Status) {
		reqLogger.Info("Policy status does not match on the hub. Triggering the status-sync to update it.")

		r.StatusSyncRequests <- event.GenericEvent{Object: managedPlc}
	}

	reqLogger.V(2).Info("Reconciliation complete.")

	return reconcile.Result{}, nil
}
