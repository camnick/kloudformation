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

package vpc

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
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new VPC Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`vpc-controller`)
	return &ReconcileVPC{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("vpc-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to VPC
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.VPC{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVPC{}

// ReconcileVPC reconciles a VPC object
type ReconcileVPC struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a VPC object and makes changes based on the state read
// and what is in the VPC.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=swarm.aws.gotopple.com,resources=dockerswarms,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileVPC) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the VPC instance
	instance := &eccv1alpha1.VPC{}
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
	// get the VPCId out of the annotations
	// if absent then create
	vpcid, ok := instance.ObjectMeta.Annotations[`vpcid`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS VPC in %s", *r.sess.Config.Region)
		createOutput, err := svc.CreateVpc(&ec2.CreateVpcInput{
			CidrBlock:       aws.String(instance.Spec.CIDRBlock),
			InstanceTenancy: aws.String(instance.Spec.InstanceTenancy),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, `Create failed: %s`, err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreateVPCOutput was nil`)
		}

		if createOutput.Vpc.VpcId == nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, `createOutput.Vpc.VpcId was nil`)
			return reconcile.Result{}, fmt.Errorf(`createOutput.Vpc.VpcId was nil`)
		}
		vpcid = *createOutput.Vpc.VpcId

		r.events.Eventf(instance, `Normal`, `CreateSuccess`, "Created AWS VPC (%s)", vpcid)
		instance.ObjectMeta.Annotations = make(map[string]string)
		instance.ObjectMeta.Annotations[`vpcid`] = vpcid
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `vpcs.ecc.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the VPC resource will not be able to track the created VPC and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS VPC before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`UpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.DeleteVpc(&ec2.DeleteVpcInput{
				VpcId: aws.String(vpcid),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupVpcId`: vpcid},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the VPC: %s", ierr.Error())
			} else if deleteOutput == nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupVpcId`: vpcid},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the VPC recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeleteVPCOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `UpdateSuccess`, "Added finalizer and annotations")

		// Make sure that there are tags to add before attempting to add them.
		if len(instance.Spec.Tags) >= 1 {
			// Tag the new VPC
			ts := []*ec2.Tag{}
			for _, t := range instance.Spec.Tags {
				ts = append(ts, &ec2.Tag{
					Key:   aws.String(t.Key),
					Value: aws.String(t.Value),
				})
			}
			tagOutput, err := svc.CreateTags(&ec2.CreateTagsInput{
				Resources: []*string{aws.String(vpcid)},
				Tags:      ts,
			})
			if err != nil {
				r.events.Eventf(instance, `Warning`, `UpdateFailure`, "Tagging failed: %s", err.Error())
				return reconcile.Result{}, err
			}
			if tagOutput == nil {
				return reconcile.Result{}, fmt.Errorf(`CreateTagsOutput was nil`)
			}
			r.events.Event(instance, `Normal`, `UpdateSuccess`, "Added tags")
		}
	} else if instance.ObjectMeta.DeletionTimestamp != nil {

		// check for other Finalizers
		for i := range instance.ObjectMeta.Finalizers {
			if instance.ObjectMeta.Finalizers[i] != `vpcs.ecc.aws.gotopple.com` {
				r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the VPC with remaining finalizers")
				return reconcile.Result{}, fmt.Errorf(`Unable to delete the VPC with remaining finalizers`)
			}
		}

		// must delete
		deleteOutput, err := svc.DeleteVpc(&ec2.DeleteVpcInput{
			VpcId: aws.String(vpcid),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the VPC: %s", err.Error())
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidVpcID.NotFound`:
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The VPC: %s was already deleted", err.Error())
				default:
					return reconcile.Result{}, err
				}
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				fmt.Println(err.Error())
				return reconcile.Result{}, err
			}
		}
		if deleteOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`DeleteVPCOutput was nil`)
		}

		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `vpcs.ecc.aws.gotopple.com` {
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
		r.events.Event(instance, `Normal`, `DeleteSuccess`, "Deleted VPC and removed finalizers")
	}

	return reconcile.Result{}, nil
}
