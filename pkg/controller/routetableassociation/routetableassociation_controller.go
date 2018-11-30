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

package routetableassociation

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

// Add creates a new RouteTableAssociation Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`routetableassociation-controller`)
	return &ReconcileRouteTableAssociation{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("routetableassociation-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to RouteTableAssociation
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.RouteTableAssociation{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileRouteTableAssociation{}

// ReconcileRouteTableAssociation reconciles a RouteTableAssociation object
type ReconcileRouteTableAssociation struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a RouteTableAssociation object and makes changes based on the state read
// and what is in the RouteTableAssociation.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=routetableassociations,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileRouteTableAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the RouteTableAssociation instance
	instance := &eccv1alpha1.RouteTableAssociation{}
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

	svc := ec2.New(r.sess)
	// get the RouteTableAssociationId out of the annotations
	// if absent then create
	routeTableAssociationId, ok := instance.ObjectMeta.Annotations[`routeTableAssociationId`]
	if !ok {

		// check if subnet is ready and confirm id is good
		subnet := &eccv1alpha1.Subnet{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.SubnetName, Namespace: instance.Namespace}, subnet)
		if err != nil {
			if errors.IsNotFound(err) {
				r.events.Eventf(instance, `Warning`, `LookupFailure`, "Can't find specifed Subnet")
				return reconcile.Result{}, fmt.Errorf(`Subnet not ready`)
			}
			return reconcile.Result{}, err
		} else if len(subnet.ObjectMeta.Annotations[`subnetid`]) <= 0 {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Specified Subnet has no ID annotation")
			return reconcile.Result{}, fmt.Errorf(`Subnet not ready`)
		}
		// check if route table is ready and confirm id is good
		routeTable := &eccv1alpha1.RouteTable{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.RouteTableName, Namespace: instance.Namespace}, routeTable)
		if err != nil {
			if errors.IsNotFound(err) {
				r.events.Eventf(instance, `Warning`, `LookupFailure`, "Can't find specified RouteTable")
				return reconcile.Result{}, fmt.Errorf(`RouteTable not ready`)
			}
			return reconcile.Result{}, err
		} else if len(routeTable.ObjectMeta.Annotations[`routeTableId`]) <= 0 {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Specified RouteTable has no ID annotation")
			return reconcile.Result{}, fmt.Errorf(`RouteTable not ready`)
		}

		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS RouteTableAssociation in %s", *r.sess.Config.Region)
		associateOutput, err := svc.AssociateRouteTable(&ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(routeTable.ObjectMeta.Annotations[`routeTableId`]),
			SubnetId:     aws.String(subnet.ObjectMeta.Annotations[`subnetid`]),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if associateOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`AssociateOutput was nil`)
		}

		if associateOutput.AssociationId == nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, `associateOutput.AssociationId was nil`)
			return reconcile.Result{}, fmt.Errorf(`associateOutput.AssociationId was nil`)
		}
		routeTableAssociationId = *associateOutput.AssociationId

		r.events.Eventf(instance, `Normal`, `CreateSuccess`, "Created AWS RouteTableAssociation (%s)", routeTableAssociationId)
		instance.ObjectMeta.Annotations = make(map[string]string)
		instance.ObjectMeta.Annotations[`routeTableAssociationId`] = routeTableAssociationId
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `routetableassociations.ecc.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the RouteTableAssociation resource will not be able to track the created RouteTableAssociation and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS RouteTableAssociation before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`UpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			disassociateOutput, ierr := svc.DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
				AssociationId: aws.String(routeTableAssociationId),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupRouteTableAssociationId`: routeTableAssociationId},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the RouteTableAssociation: %s", ierr.Error())

				if aerr, ok := ierr.(awserr.Error); ok {
					switch aerr.Code() {
					default:
						r.events.Eventf(instance, `Warning`, `DeleteFailure`, `Delete Failure: %s`, aerr.Error())

					}
				} else {
					// Print the error, cast err to awserr.Error to get the Code and
					// Message from an error.
					r.events.Eventf(instance, `Warning`, `DeleteFailure`, `Delete Failure: %s`, ierr.Error())
				}

			} else if disassociateOutput == nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupRouteTableAssociationId`: routeTableAssociationId},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the RouteTableAssociation recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DisassociateOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `UpdateSuccess`, "Added finalizer and annotations")

	} else if instance.ObjectMeta.DeletionTimestamp != nil {

		// check if subnet is ready and confirm id is good
		subnet := &eccv1alpha1.Subnet{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.SubnetName, Namespace: instance.Namespace}, subnet)
		if err != nil {
			if errors.IsNotFound(err) {
				r.events.Eventf(instance, `Warning`, `LookupFailure`, "Can't find specified Subnet- Deleting anway")
			}
		} else if len(subnet.ObjectMeta.Annotations[`subnetid`]) <= 0 {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Specified Subnet has no ID annotation")
			return reconcile.Result{}, fmt.Errorf(`Subnet not ready`)
		}
		// check if route table is ready and confirm id is good
		routeTable := &eccv1alpha1.RouteTable{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.RouteTableName, Namespace: instance.Namespace}, routeTable)
		if err != nil {
			if errors.IsNotFound(err) {
				r.events.Eventf(instance, `Warning`, `LookupFailure`, "Can't find specified RouteTable- Deleting anyway")
			}
		} else if len(routeTable.ObjectMeta.Annotations[`routeTableId`]) <= 0 {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Specifed RouteTable has no ID annotation")
			return reconcile.Result{}, fmt.Errorf(`RouteTable not ready`)
		}

		// check for other Finalizers
		for i := range instance.ObjectMeta.Finalizers {
			if instance.ObjectMeta.Finalizers[i] != `routetableassociations.ecc.aws.gotopple.com` {
				r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the RouteTableAssociation with remaining finalizers")
				return reconcile.Result{}, fmt.Errorf(`Unable to delete the RouteTableAssociation with remaining finalizers`)
			}
		}

		// must delete
		_, err = svc.DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
			AssociationId: aws.String(routeTableAssociationId),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the RouteTableAssociation: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidAssociationID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The RouteTableAssociation: %s was already deleted", err.Error())
				default:
					return reconcile.Result{}, err
				}
			} else {
				return reconcile.Result{}, err
			}
		}

		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `routetableassociations.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// after a successful delete update the resource with the removed finalizer
		err = r.Update(context.TODO(), instance)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `UpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `DeleteSuccess`, "Deleted RouteTableAssociation and removed finalizers")
	}

	return reconcile.Result{}, nil
}
