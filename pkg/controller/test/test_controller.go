package test

import (
	"context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/jboss-fuse/yaks/pkg/apis/yaks/v1alpha1"
	"github.com/jboss-fuse/yaks/pkg/client"
	"github.com/jboss-fuse/yaks/pkg/util/log"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Integration Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	c, err := client.FromManager(mgr)
	if err != nil {
		return err
	}
	return add(mgr, newReconciler(mgr, c))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, c client.Client) reconcile.Reconciler {
	return &ReconcileIntegrationTest{
		client: c,
		scheme: mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("test-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Integration
	err = c.Watch(&source.Kind{Type: &v1alpha1.Test{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldTest := e.ObjectOld.(*v1alpha1.Test)
			newTest := e.ObjectNew.(*v1alpha1.Test)
			// Ignore updates to the integration status in which case metadata.Generation does not change,
			// or except when the integration phase changes as it's used to transition from one phase
			// to another
			return oldTest.Generation != newTest.Generation ||
				oldTest.Status.Phase != newTest.Status.Phase
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted
			return !e.DeleteStateUnknown
		},
	})
	if err != nil {
		return err
	}

	// Watch for related Pods changing
	err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			pod := a.Object.(*v1.Pod)
			var requests []reconcile.Request

			if testName, ok := pod.Labels["yaks.dev/test"]; ok {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: pod.Namespace,
						Name:      testName,
					},
				})
			}

			return requests
		}),
	})

	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileIntegrationTest{}

// ReconcileIntegrationTest reconciles a IntegrationTest object
type ReconcileIntegrationTest struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Integration object and makes changes based on the state read
// and what is in the Integration.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileIntegrationTest) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	rlog := Log.WithValues("request-namespace", request.Namespace, "request-name", request.Name)
	rlog.Info("Reconciling Test")

	ctx := context.TODO()

	// Fetch the Test instance
	var instance v1alpha1.Test

	if err := r.client.Get(ctx, request.NamespacedName, &instance); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Delete phase
	if instance.GetDeletionTimestamp() != nil {
		instance.Status.Phase = v1alpha1.TestPhaseDeleting
	}

	target := instance.DeepCopy()
	targetLog := rlog.ForTest(target)

	actions := []Action{
		NewInitializeAction(),
		NewStartAction(),
		NewEvaluateAction(),
	}

	for _, a := range actions {
		a.InjectClient(r.client)
		a.InjectLogger(targetLog)

		if a.CanHandle(target) {
			targetLog.Infof("Invoking action %s", a.Name())

			newTarget, err := a.Handle(ctx, target)
			if err != nil {
				return reconcile.Result{}, err
			}

			if newTarget != nil {
				if r, err := r.update(ctx, targetLog, newTarget); err != nil {
					return r, err
				}

				if newTarget.Status.Phase != target.Status.Phase {
					targetLog.Info(
						"state transition",
						"phase-from", target.Status.Phase,
						"phase-to", newTarget.Status.Phase,
					)
				}
			}

			// handle one action at time so the resource
			// is always at its latest state
			break
		}
	}

	return reconcile.Result{}, nil
}

// Update --
func (r *ReconcileIntegrationTest) update(ctx context.Context, log log.Logger, target *v1alpha1.Test) (reconcile.Result, error) {
	err := r.client.Status().Update(ctx, target)
	if err != nil {
		if k8serrors.IsConflict(err) {
			log.Error(err, "conflict")

			return reconcile.Result{
				Requeue: true,
			}, nil
		}
	}

	return reconcile.Result{}, err
}
