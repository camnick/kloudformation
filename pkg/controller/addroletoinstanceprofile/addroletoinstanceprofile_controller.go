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

package addroletoinstanceprofile

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

// Add creates a new AddRoleToInstanceProfile Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`addroletoinstanceprofile-controller`)
	return &ReconcileAddRoleToInstanceProfile{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("addroletoinstanceprofile-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to AddRoleToInstanceProfile
	err = c.Watch(&source.Kind{Type: &iamv1alpha1.AddRoleToInstanceProfile{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileAddRoleToInstanceProfile{}

// ReconcileAddRoleToInstanceProfile reconciles a AddRoleToInstanceProfile object
type ReconcileAddRoleToInstanceProfile struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a AddRoleToInstanceProfile object and makes changes based on the state read
// and what is in the AddRoleToInstanceProfile.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=iam.aws.gotopple.com,resources=addroletoinstanceprofiles,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileAddRoleToInstanceProfile) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the AddRoleToInstanceProfile instance
	instance := &iamv1alpha1.AddRoleToInstanceProfile{}
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
	} else if len(role.ObjectMeta.Annotations[`awsRoleId`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "Role not ready")
		return reconcile.Result{}, fmt.Errorf(`Role not ready`)
	}

	instanceProfile := &iamv1alpha1.IAMInstanceProfile{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.IamInstanceProfileName, Namespace: instance.Namespace}, instanceProfile)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "IAMInstanceProfile not found")
			return reconcile.Result{}, fmt.Errorf(`IAMInstanceProfile not ready`)
		}
		return reconcile.Result{}, err
	} else if len(instanceProfile.ObjectMeta.Annotations[`iamInstanceProfileArn`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "IAMInstanceProfile not ready")
		return reconcile.Result{}, fmt.Errorf(`IAMInstanceProfile not ready`)
	}

	svc := iam.New(r.sess)

	// get the AddRoleToInstanceProfileId out of the annotations
	// if absent then create
	iamRoleAddedToInstanceProfile, ok := instance.ObjectMeta.Annotations[`iamRoleAddedToInstanceProfile`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS AddRoleToInstanceProfile in %s", *r.sess.Config.Region)
		createOutput, err := svc.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{
			InstanceProfileName: aws.String(instanceProfile.ObjectMeta.Annotations[`awsInstanceProfileName`]),
			RoleName:            aws.String(role.ObjectMeta.Annotations[`awsRoleName`]),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreateAddRoleToInstanceProfile was nil`)
		}

		iamRoleAddedToInstanceProfile = "yes"
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS AddRoleToInstanceProfile")
		instance.ObjectMeta.Annotations = make(map[string]string)
		instance.ObjectMeta.Annotations[`iamRoleAddedToInstanceProfile`] = iamRoleAddedToInstanceProfile
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `addroletoinstanceprofiles.iam.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the AddRoleToInstanceProfile resource will not be able to track the created AddRoleToInstanceProfile and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS AddRoleToInstanceProfile before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.RemoveRoleFromInstanceProfile(&iam.RemoveRoleFromInstanceProfileInput{
				InstanceProfileName: aws.String(instanceProfile.ObjectMeta.Annotations[`awsInstanceProfileName`]),
				RoleName:            aws.String(role.ObjectMeta.Annotations[`awsRoleName`]),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupAddRoleToInstanceProfile`: (role.ObjectMeta.Annotations[`iamRoleName`] + " & " + instanceProfile.Spec.InstanceProfileName)},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the AddRoleToInstanceProfile: %s", ierr.Error())

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
					map[string]string{`cleanupAddRoleToInstanceProfile`: (role.ObjectMeta.Annotations[`iamRoleName`] + " & " + instanceProfile.Spec.InstanceProfileName)},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the AddRoleToInstanceProfile recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DetachRolePolicyOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

		instanceProfile.ObjectMeta.Annotations[`assignedAwsRoleName`] = role.ObjectMeta.Annotations[`awsRoleName`]
		err = r.Update(context.TODO(), instanceProfile)
		if err != nil {
			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

		}
		r.events.Event(instanceProfile, `Normal`, `Annotated`, "Added assgned AWS Role Name to Annotations")

	} else if instance.ObjectMeta.DeletionTimestamp != nil {

		// check for other Finalizers
		for i := range instance.ObjectMeta.Finalizers {
			if instance.ObjectMeta.Finalizers[i] != `addroletoinstanceprofiles.iam.aws.gotopple.com` {
				r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the AddRoleToInstanceProfile with remaining finalizers")
				return reconcile.Result{}, fmt.Errorf(`Unable to delete the AddRoleToInstanceProfile with remaining finalizers`)
			}
		}

		// must delete
		_, err = svc.RemoveRoleFromInstanceProfile(&iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: aws.String(instanceProfile.ObjectMeta.Annotations[`awsInstanceProfileName`]),
			RoleName:            aws.String(role.ObjectMeta.Annotations[`awsRoleName`]),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the AddRoleToInstanceProfile: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidInstanceProfileID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The AddRoleToInstanceProfile: %s was already deleted", err.Error())
				default:
					return reconcile.Result{}, err
				}
			} else {
				return reconcile.Result{}, err
			}
		}

		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `addroletoinstanceprofiles.iam.aws.gotopple.com` {
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
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted AddRoleToInstanceProfile and removed finalizers")
	}

	return reconcile.Result{}, nil
}
