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

package iampolicy

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
	//"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new IAMPolicy Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`iampolicy-controller`)
	return &ReconcileIAMPolicy{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("iampolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to IAMPolicy
	err = c.Watch(&source.Kind{Type: &iamv1alpha1.IAMPolicy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileIAMPolicy{}

// ReconcileIAMPolicy reconciles a IAMPolicy object
type ReconcileIAMPolicy struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a IAMPolicy object and makes changes based on the state read
// and what is in the IAMPolicy.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=iampolicies,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileIAMPolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the IAMPolicy instance
	instance := &iamv1alpha1.IAMPolicy{}
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

	svc := iam.New(r.sess)

	// get the IAMPolicyId out of the annotations
	// if absent then create
	iamPolicyId, ok := instance.ObjectMeta.Annotations[`iamPolicyId`]
	if !ok {
		instance.ObjectMeta.Annotations = make(map[string]string) // This keeps the controller from crashing when annotation later on
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS IAMPolicy in %s", *r.sess.Config.Region)
		createOutput, err := svc.CreatePolicy(&iam.CreatePolicyInput{
			PolicyDocument: aws.String(instance.Spec.PolicyDocument),
			Description:    aws.String(instance.Spec.Description),
			Path:           aws.String(instance.Spec.Path),
			PolicyName:     aws.String(instance.Spec.PolicyName),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreatePolicyOutput was nil`)
		}

		iamPolicyId = *createOutput.Policy.PolicyId
		iamPolicyArn := *createOutput.Policy.Arn
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS IAMPolicy (%s)", iamPolicyId)
		instance.ObjectMeta.Annotations[`iamPolicyId`] = iamPolicyId
		instance.ObjectMeta.Annotations[`iamPolicyArn`] = iamPolicyArn
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `iampolicies.ecc.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the IAMPolicy resource will not be able to track the created IAMPolicy and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS IAMPolicy before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.DeletePolicy(&iam.DeletePolicyInput{
				PolicyArn: aws.String(instance.ObjectMeta.Annotations[`iamPolicyArn`]),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupPolicyId`: iamPolicyId},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the Policy: %s", ierr.Error())

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
					map[string]string{`cleanupPolicyId`: iamPolicyId},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the IAMPolicy recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeletePolicyOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

	} else if instance.ObjectMeta.DeletionTimestamp != nil {
		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `iampolicies.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// must delete
		_, err = svc.DeletePolicy(&iam.DeletePolicyInput{
			PolicyArn: aws.String(instance.ObjectMeta.Annotations[`iamPolicyArn`]),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the Policy: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidPolicyID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The Policy: %s was already deleted", err.Error())
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
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted IAMPolicy and removed finalizers")
	}

	return reconcile.Result{}, nil
}
