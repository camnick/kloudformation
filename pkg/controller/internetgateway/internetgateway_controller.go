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

package internetgateway

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

// Add creates a new InternetGateway Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`internetgateway-controller`)
	return &ReconcileInternetGateway{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("internetgateway-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to InternetGateway
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.InternetGateway{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileInternetGateway{}

// ReconcileInternetGateway reconciles a InternetGateway object
type ReconcileInternetGateway struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a InternetGateway object and makes changes based on the state read
// and what is in the InternetGateway.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=internetgateways,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileInternetGateway) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the InternetGateway instance
	instance := &eccv1alpha1.InternetGateway{}
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

	vpc := &eccv1alpha1.VPC{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.VPCName, Namespace: instance.Namespace}, vpc)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	} else if len(vpc.ObjectMeta.Annotations[`vpcid`]) <= 0 {
		return reconcile.Result{}, fmt.Errorf(`VPC not ready`)
	}

	svc := ec2.New(r.sess)
	// get the InternetGatewayId out of the annotations
	// if absent then create
	internetGatewayId, ok := instance.ObjectMeta.Annotations[`internetGatewayId`]
	if !ok {

		// log intention to attempt, then attempt
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS InternetGateway in %s", *r.sess.Config.Region)
		createGatewayOutput, err := svc.CreateInternetGateway(&ec2.CreateInternetGatewayInput{})

		//catch the errors fromt the create attempt
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createGatewayOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreateInternetGatewayOutput was nil`)
		}

		// save the output into an id for the gatway, note the success, and then save the id in the object annotations
		internetGatewayId = *createGatewayOutput.InternetGateway.InternetGatewayId
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS InternetGateway (%s)", internetGatewayId)
		instance.ObjectMeta.Annotations[`internetGatewayId`] = internetGatewayId


		//append finalizer now that everything is done
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `internetgateways.ecc.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the InternetGateway resource will not be able to track the created InternetGateway and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS InternetGateway before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{
				InternetGatewayId: aws.String(internetGatewayId),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupinternetGatewayId`: internetGatewayId},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the InternetGateway: %s", ierr.Error())

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
					map[string]string{`cleanupInternetGatewayId`: internetGatewayId},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the InternetGateway recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeleteInternetGatewayOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

		// Make sure that there are tags to add before attempting to add them.
		if len(instance.Spec.Tags) >= 1 {
			// Tag the new InternetGateway
			ts := []*ec2.Tag{}
			for _, t := range instance.Spec.Tags {
				ts = append(ts, &ec2.Tag{
					Key:   aws.String(t.Key),
					Value: aws.String(t.Value),
				})
			}
			tagOutput, err := svc.CreateTags(&ec2.CreateTagsInput{
				Resources: []*string{aws.String(internetGatewayId)},
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
			if f == `internetgateways.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// must delete

		r.events.Eventf(instance, `Normal`, `ResourceDeleteAttempt`, "Deleting AWS InternetGateway (%s)", instance.ObjectMeta.Annotations[`internetGatewayId`])

		_, err = svc.DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{
			InternetGatewayId: aws.String(internetGatewayId),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the InternetGateway: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidInternetGatewayID.NotFound`:
					// we want to keep going
					// event type was changed from 'Success' to 'Normal', because i got an error that 'Success' wasn't supported
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The InternetGateway: %s was already deleted", err.Error())
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
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted InternetGateway and removed finalizers")
	}

	return reconcile.Result{}, nil
}
