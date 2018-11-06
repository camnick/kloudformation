/*
Copyright 2018 Jeff Nickoloff (jeff@allingeek.com).

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

package iamattachrolepolicy

import (
	"context"
	"fmt"

	aws "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	iam "github.com/aws/aws-sdk-go/service/iam"
	iamv1alpha1 "github.com/gotopple/kloudformation/pkg/apis/iam/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new IAMAttachRolePolicy Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`iamattachrolepolicy-controller`)
	return &ReconcileIAMAttachRolePolicy{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("iamattachrolepolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to IAMAttachRolePolicy
	err = c.Watch(&source.Kind{Type: &iamv1alpha1.IAMAttachRolePolicy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileIAMAttachRolePolicy{}

// ReconcileIAMAttachRolePolicy reconciles a IAMAttachRolePolicy object
type ReconcileIAMAttachRolePolicy struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a IAMAttachRolePolicy object and makes changes based on the state read
// and what is in the IAMAttachRolePolicy.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=iam.aws.gotopple.com,resources=iamattachrolepolicies,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileIAMAttachRolePolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the IAMAttachRolePolicy instance
	instance := &iamv1alpha1.IAMAttachRolePolicy{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	role := &iamv1alpha1.Role{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.IamRoleName, Namespace: instance.Namespace}, role)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Role not found")
			return reconcile.Result{}, fmt.Errorf(`Role not ready`)
		}
		return reconcile.Result{}, err
	} else if len(role.ObjectMeta.Annotations[`roleId`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "Role not ready")
		return reconcile.Result{}, fmt.Errorf(`Role not ready`)
	}

	policy := &iamv1alpha1.IAMPolicy{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.IamPolicyName, Namespace: instance.Namespace}, policy)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "IAMPolicy not found")
			return reconcile.Result{}, fmt.Errorf(`IAMPolicy not ready`)
		}
		return reconcile.Result{}, err
	} else if len(policy.ObjectMeta.Annotations[`iamPolicyArn`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "IAMPolicy not ready")
		return reconcile.Result{}, fmt.Errorf(`IAMPolicy not ready`)
	}

	svc := iam.New(r.sess)

	// get the IAMAttachRolePolicyId out of the annotations
	// if absent then create
	iamRolePolicyAttached, ok := instance.ObjectMeta.Annotations[`iamRolePolicyAttached`]
	if !ok {
		instance.ObjectMeta.Annotations = make(map[string]string) // This keeps the controller from crashing when annotation later on
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS IAMAttachRolePolicy in %s", *r.sess.Config.Region)
		createOutput, err := svc.AttachRolePolicy(&iam.AttachRolePolicyInput{
			PolicyArn: aws.String(policy.ObjectMeta.Annotations[`iamPolicyArn`]),
			RoleName:  aws.String(role.Spec.RoleName),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreateAttachRolePolicyOutput was nil`)
		}

		iamRolePolicyAttached = "yes"
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS IAMAttachRolePolicy")
		instance.ObjectMeta.Annotations[`iamRolePolicyAttached`] = iamRolePolicyAttached
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `iamattachrolepolicies.iam.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the IAMAttachRolePolicy resource will not be able to track the created IAMAttachRolePolicy and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS IAMAttachRolePolicy before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.DetachRolePolicy(&iam.DetachRolePolicyInput{
				PolicyArn: aws.String(policy.ObjectMeta.Annotations[`iamPolicyArn`]),
				RoleName:  aws.String(role.Spec.RoleName),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupAttachRolePolicy`: (policy.ObjectMeta.Annotations[`iamPolicyArn`] + " & " + role.Spec.RoleName)},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the IAMAttachRolePolicy: %s", ierr.Error())

				if aerr, ok := ierr.(awserr.Error); ok {
					switch aerr.Code() {
					default:
						r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Delete failed: %s", aerr.Error())
					}
				} else {
					// Print the error, cast err to awserr.Error to get the Code and
					// Message from an error.
					r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Delete failed: %s", ierr.Error())
				}

			} else if deleteOutput == nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupAttachRolePolicy`: (policy.ObjectMeta.Annotations[`iamPolicyArn`] + " & " + role.Spec.RoleName)},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the IAMAttachRolePolicy recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DetachRolePolicyOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

	} else if instance.ObjectMeta.DeletionTimestamp != nil {
		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `iamattachrolepolicies.iam.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// must delete
		_, err = svc.DetachRolePolicy(&iam.DetachRolePolicyInput{
			PolicyArn: aws.String(policy.ObjectMeta.Annotations[`iamPolicyArn`]),
			RoleName:  aws.String(role.Spec.RoleName),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the IAMAttachRolePolicy: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidInstanceProfileID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The IAMAttachRolePolicy: %s was already deleted", err.Error())
				default:
					return reconcile.Result{}, err
				}
			} else {
				return reconcile.Result{}, err
			}
		}

		// after a successful delete update the resource with the removed finalizer
		err = r.Update(context.TODO(), instance)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted IAMAttachRolePolicy and removed finalizers")
	}

	return reconcile.Result{}, nil
}
