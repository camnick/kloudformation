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

package eipassociation

import (
	"context"
	"fmt"

	aws "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	ec2 "github.com/aws/aws-sdk-go/service/ec2"
	eccv1alpha1 "github.com/gotopple/kloudformation/pkg/apis/ecc/v1alpha1"
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

// Add creates a new EIPAssociation Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`eipassociation-controller`)
	return &ReconcileEIPAssociation{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("eipassociation-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to EIPAssociation
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.EIPAssociation{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileEIPAssociation{}

// ReconcileEIPAssociation reconciles a EIPAssociation object
type ReconcileEIPAssociation struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a EIPAssociation object and makes changes based on the state read
// and what is in the EIPAssociation.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=eipassociations,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileEIPAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the EIPAssociation instance
	instance := &eccv1alpha1.EIPAssociation{}
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

	eip := &eccv1alpha1.EIP{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.VPCName, Namespace: instance.Namespace}, eip)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	} else if len(vpc.ObjectMeta.Annotations[`eipAllocationId`]) <= 0 {
		return reconcile.Result{}, fmt.Errorf(`EIP not ready`)
	}

	svc := ec2.New(r.sess)
	// get the EIPAssociationId out of the annotations
	// if absent then create
	eipAssociationId, ok := instance.ObjectMeta.Annotations[`eipAssociationId`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS EIP Association in %s", *r.sess.Config.Region)
		associateOutput, err := svc.AssociateAddress(&ec2.AssociateAddressInput{
			AllocationId: aws.String(eip.ObjectMeta.Annotations[`eipAllocationId`]),
			InstanceId:   aws.String("i-07028223502b4ddcc"),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`associateOutput was nil`)
		}

		eipAssociationId = *associateOutput.AssociationId
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS EIP Associationt (%s)", eipAssociationId)
		instance.ObjectMeta.Annotations[`eipAssociationId`] = eipAssociation
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `eipassociations.ecc.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the Subnet resource will not be able to track the created Subnet and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS Subnet before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			_, ierr := svc.DisassociateAddress(&ec2.DisassociateAddressInput{
				AssociationId: aws.String(eipAssociationId),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupEIPAssociationId`: eipAssociationId},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the Subnet: %s", ierr.Error())

				if aerr, ok := ierr.(awserr.Error); ok {
					switch aerr.Code() {
					default:
						fmt.Println(aerr.Error())
					}
				} else {
					// Print the error, cast err to awserr.Error to get the Code and
					// Message from an error.
					fmt.Println(ierr.Error())
				}

			} else if deleteOutput == nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupEIPAssociationId`: eipAssociationId},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the Subnet recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DisassociateAddressOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

		// Make sure that there are tags to add before attempting to add them.
	} else if instance.ObjectMeta.DeletionTimestamp != nil {
		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `eipassociations.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// must delete
		_, err = svc.DisassociateAddress(&ec2.DisassociateAddressInput{
			AssociationId: aws.String(eipAssociationId),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to disassociate the address: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidAssociationId.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The address: %s was already disassociated", err.Error())
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
		r.events.Event(instance, `Normal`, `Deleted`, "Disassociated address and removed finalizers")
	}

	return reconcile.Result{}, nil
}
