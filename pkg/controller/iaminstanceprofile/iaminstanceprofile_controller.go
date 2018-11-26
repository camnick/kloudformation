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

package iaminstanceprofile

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

// Add creates a new IAMInstanceProfile Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`iaminstanceprofile-controller`)
	return &ReconcileIAMInstanceProfile{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("iaminstanceprofile-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to IAMInstanceProfile
	err = c.Watch(&source.Kind{Type: &iamv1alpha1.IAMInstanceProfile{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileIAMInstanceProfile{}

// ReconcileIAMInstanceProfile reconciles a IAMInstanceProfile object
type ReconcileIAMInstanceProfile struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a IAMInstanceProfile object and makes changes based on the state read
// and what is in the IAMInstanceProfile.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=iam.aws.gotopple.com,resources=iaminstanceprofiles,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileIAMInstanceProfile) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the IAMInstanceProfile instance
	instance := &iamv1alpha1.IAMInstanceProfile{}
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

	// get the IAMInstanceProfileId out of the annotations
	// if absent then create
	iamInstanceProfileId, ok := instance.ObjectMeta.Annotations[`iamInstanceProfileId`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS IAMInstanceProfile in %s", *r.sess.Config.Region)
		createOutput, err := svc.CreateInstanceProfile(&iam.CreateInstanceProfileInput{
			Path:                aws.String(instance.Spec.Path),
			InstanceProfileName: aws.String(instance.Spec.InstanceProfileName),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreateInstanceProfileOutput was nil`)
		}

		if createOutput.InstanceProfile.InstanceProfileId == nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, `createOutput.InstanceProfile.InstanceProfileId was nil`)
			return reconcile.Result{}, fmt.Errorf(`createOutput.InstanceProfile.InstanceProfileId was nil`)
		}
		iamInstanceProfileId = *createOutput.InstanceProfile.InstanceProfileId

		if createOutput.InstanceProfile.Arn == nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, `createOutput.InstanceProfile.Arn was nil`)
			return reconcile.Result{}, fmt.Errorf(`createOutput.InstanceProfile.Arn was nil`)
		}
		iamInstanceProfileArn := *createOutput.InstanceProfile.Arn

		if createOutput.InstanceProfile.InstanceProfileName == nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, `createOutput.InstanceProfile.InstanceProfileName was nil`)
			return reconcile.Result{}, fmt.Errorf(`createOutput.InstanceProfile.InstanceProfileName was nil`)
		}
		iamInstanceProfileName := *createOutput.InstanceProfile.InstanceProfileName

		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS IAMInstanceProfile (%s)", iamInstanceProfileId)
		instance.ObjectMeta.Annotations = make(map[string]string)
		instance.ObjectMeta.Annotations[`iamInstanceProfileId`] = iamInstanceProfileId
		instance.ObjectMeta.Annotations[`iamInstanceProfileArn`] = iamInstanceProfileArn
		instance.ObjectMeta.Annotations[`awsInstanceProfileName`] = iamInstanceProfileName
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `iaminstanceprofiles.iam.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the IAMInstanceProfile resource will not be able to track the created IAMInstanceProfile and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS IAMInstanceProfile before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.DeleteInstanceProfile(&iam.DeleteInstanceProfileInput{
				InstanceProfileName: aws.String(instance.Spec.InstanceProfileName),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupInstanceProfileId`: iamInstanceProfileId},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the IAMInstanceProfile: %s", ierr.Error())

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
					map[string]string{`cleanupInstanceProfileId`: iamInstanceProfileId},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the IAMInstanceProfile recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeleteInstanceProfileOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

	} else if instance.ObjectMeta.DeletionTimestamp != nil {

		// check for other Finalizers
		for i := range instance.ObjectMeta.Finalizers {
			if instance.ObjectMeta.Finalizers[i] != `iaminstanceprofiles.iam.aws.gotopple.com` {
				r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the IAMInstanceProfile with remaining finalizers")
				return reconcile.Result{}, fmt.Errorf(`Unable to delete the IAMInstanceProfile with remaining finalizers`)
			}
		}

		// must delete
		_, err = svc.DeleteInstanceProfile(&iam.DeleteInstanceProfileInput{
			InstanceProfileName: aws.String(instance.Spec.InstanceProfileName),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the IAMInstanceProfile: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidInstanceProfileID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The IAMInstanceProfile: %s was already deleted", err.Error())
				default:
					return reconcile.Result{}, err
				}
			} else {
				return reconcile.Result{}, err
			}
		}

		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `iaminstanceprofiles.iam.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// after a successful delete update the resource with the removed finalizer
		err = r.Update(context.TODO(), instance)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted IAMInstanceProfile and removed finalizers")
	}

	return reconcile.Result{}, nil
}
