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

package natgateway

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

// Add creates a new NATGateway Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`natgateway-controller`)
	return &ReconcileNATGateway{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("natgateway-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to NATGateway
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.NATGateway{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileNATGateway{}

// ReconcileNATGateway reconciles a NATGateway object
type ReconcileNATGateway struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a NATGateway object and makes changes based on the state read
// and what is in the NATGateway.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=natgateways,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileNATGateway) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the NATGateway instance
	instance := &eccv1alpha1.NATGateway{}
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

	subnet := &eccv1alpha1.Subnet{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.SubnetName, Namespace: instance.Namespace}, subnet)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Can't find Subnet")
			return reconcile.Result{}, fmt.Errorf(`Subnet not ready`)
		}
		return reconcile.Result{}, err
	} else if len(subnet.ObjectMeta.Annotations[`subnetid`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "Subnet has no ID annotation")
		return reconcile.Result{}, fmt.Errorf(`Subnet not ready`)
	}

	eip := &eccv1alpha1.EIP{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.EIPAllocationName, Namespace: instance.Namespace}, eip)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "EIP Allocation not found")
			return reconcile.Result{}, fmt.Errorf(`EIP not ready`)
		}
		return reconcile.Result{}, err
	} else if len(eip.ObjectMeta.Annotations[`eipAllocationId`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "EIP allocation has ID annotation")
		return reconcile.Result{}, fmt.Errorf(`EIP not ready`)
	}

	vpc := &eccv1alpha1.VPC{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: subnet.Spec.VPCName, Namespace: instance.Namespace}, vpc)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateAttempt`, "Can't find VPC")
			return reconcile.Result{}, fmt.Errorf(`VPC not ready`)
		}
		return reconcile.Result{}, err
	} else if len(vpc.ObjectMeta.Annotations[`vpcid`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "VPC has no ID annotation")
		return reconcile.Result{}, fmt.Errorf(`VPC not ready`)
	}

	//internetGateway := &eccv1alpha1.InternetGateway{}
	svc := ec2.New(r.sess)
	// get the NATGatewayId out of the annotations
	// if absent then create
	natGatewayId, ok := instance.ObjectMeta.Annotations[`natGatewayId`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS NATGateway in %s", *r.sess.Config.Region)

		createOutput, err := svc.CreateNatGateway(&ec2.CreateNatGatewayInput{
			AllocationId: aws.String(eip.ObjectMeta.Annotations[`eipAllocationId`]),
			SubnetId:     aws.String(subnet.ObjectMeta.Annotations[`subnetid`]),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreateNatGatewayOutput was nil`)
		}

		natGatewayId = *createOutput.NatGateway.NatGatewayId
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS NATGateway (%s)", natGatewayId)
		instance.ObjectMeta.Annotations = make(map[string]string)
		instance.ObjectMeta.Annotations[`natGatewayId`] = natGatewayId
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `natgateways.ecc.aws.gotopple.com`)
		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the NATGateway resource will not be able to track the created NATGateway and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS NATGateway before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.DeleteNatGateway(&ec2.DeleteNatGatewayInput{
				NatGatewayId: aws.String(natGatewayId),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupNATGatewayId`: natGatewayId},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the NATGateway: %s", ierr.Error())

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
					map[string]string{`cleanupNATGatewayId`: natGatewayId},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the NATGateway recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeleteNatGatewayOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

		// Make sure that there are tags to add before attempting to add them.
		if len(instance.Spec.Tags) >= 1 {
			// Tag the new NATGateway
			ts := []*ec2.Tag{}
			for _, t := range instance.Spec.Tags {
				ts = append(ts, &ec2.Tag{
					Key:   aws.String(t.Key),
					Value: aws.String(t.Value),
				})
			}
			tagOutput, err := svc.CreateTags(&ec2.CreateTagsInput{
				Resources: []*string{aws.String(natGatewayId)},
				Tags:      ts,
			})
			if err != nil {
				r.events.Eventf(instance, `Warning`, `TaggingFailure`, "Tagging failed: %s", err.Error())
				return reconcile.Result{}, err
			}
			if tagOutput == nil {
				return reconcile.Result{}, fmt.Errorf(`CreateTagsOutput was nil`)
			}
			r.events.Event(instance, `Normal`, `Tagged`, "Added tags")
		}

	} else if instance.ObjectMeta.DeletionTimestamp != nil {
		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `natgateways.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// must delete
		_, err = svc.DeleteNatGateway(&ec2.DeleteNatGatewayInput{
			NatGatewayId: aws.String(natGatewayId),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the NATGateway: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidNATGatewayID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The NATGateway: %s was already deleted", err.Error())
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
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted NATGateway and removed finalizers")
	}

	return reconcile.Result{}, nil
}
