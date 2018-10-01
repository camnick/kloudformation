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

package route

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

// Add creates a new Route Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`route-controller`)
	return &ReconcileRoute{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("route-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Route
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.Route{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileRoute{}

// ReconcileRoute reconciles a Route object
type ReconcileRoute struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a Route object and makes changes based on the state read
// and what is in the Route.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=routes,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileRoute) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Route instance
	instance := &eccv1alpha1.Route{}
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

	//vpc := &eccv1alpha1.VPC{}
	routetable := &eccv1alpha1.RouteTable{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.RouteTableName, Namespace: instance.Namespace}, routetable)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	} else if len(routetable.ObjectMeta.Annotations[`routeTableId`]) <= 0 {
		return reconcile.Result{}, fmt.Errorf(`RouteTable not ready`)
	}

	svc := ec2.New(r.sess)
	// get the RouteId out of the annotations
	// if absent then create
	routeCreated, ok := instance.ObjectMeta.Annotations[`routeCreated`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS Route in %s", *r.sess.Config.Region)
		createOutput, err := svc.CreateRoute(&ec2.CreateRouteInput{
			DestinationCidrBlock: aws.String(instance.Spec.DestinationCidrBlock),
			//DestinationIpv6CidrBlock: aws.String(instance.Spec.DestinationIpv6CidrBlock),
			//EgressOnlyInternetGatewayName:
			GatewayId: aws.String("igw-0327c065"),
			//InstanceName: aws.String(instance.Spec.InstanceName),
			//NatGatewayName: aws.String(instance.Spec.NatGatewayName),
			//NetworkInterfaceName: aws.String(instance.Spec.NetworkInterfaceName),
			RouteTableId: aws.String(routetable.ObjectMeta.Annotations[`routeTableId`]),
			//VpcPeeringConnectionName: aws.String(instance.Spec.VpcPeeringConnectionName),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreateRouteOutput was nil`)
		}

		routeCreated = fmt.Sprint(*createOutput.Return)
		print(routeCreated)
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS Route (%s) as part of RouteTable", routetable.ObjectMeta.Annotations[`routeTableId`])
		instance.ObjectMeta.Annotations[`associatedRouteTable`] = routetable.ObjectMeta.Annotations[`routeTableId`]
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `routes.ecc.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the Route resource will not be able to track the created Route and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS Route before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.DeleteRoute(&ec2.DeleteRouteInput{
				RouteTableId:         aws.String(routetable.ObjectMeta.Annotations[`routeTableId`]),
				DestinationCidrBlock: aws.String(instance.Spec.DestinationCidrBlock),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupRouteId`: "routeid"},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the Route: %s", ierr.Error())

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
					map[string]string{`cleanupRouteId`: "routeid"},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the Route recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeleteRouteOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")
		/*
			// Make sure that there are tags to add before attempting to add them.
			if len(instance.Spec.Tags) >= 1 {
				// Tag the new Route
				ts := []*ec2.Tag{}
				for _, t := range instance.Spec.Tags {
					ts = append(ts, &ec2.Tag{
						Key:   aws.String(t.Key),
						Value: aws.String(t.Value),
					})
				}
				tagOutput, err := svc.CreateTags(&ec2.CreateTagsInput{
					Resources: []*string{aws.String(routeid)},
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
		*/
	} else if instance.ObjectMeta.DeletionTimestamp != nil {
		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `routes.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// must delete
		_, err = svc.DeleteRoute(&ec2.DeleteRouteInput{
			RouteTableId:         aws.String(routetable.ObjectMeta.Annotations[`routeTableId`]),
			DestinationCidrBlock: aws.String(instance.Spec.DestinationCidrBlock),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the Route: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidRouteID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Success`, `AlreadyDeleted`, "The Route: %s was already deleted", err.Error())
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
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted Route and removed finalizers")
	}

	return reconcile.Result{}, nil
}
