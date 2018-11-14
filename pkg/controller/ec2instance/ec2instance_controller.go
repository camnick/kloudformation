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

package ec2instance

import (
	"context"
	"encoding/base64"
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

// Add creates a new EC2Instance Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`ec2instance-controller`)
	return &ReconcileEC2Instance{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("ec2instance-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to EC2Instance
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.EC2Instance{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileEC2Instance{}

// ReconcileEC2Instance reconciles an EC2Instance object
type ReconcileEC2Instance struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for an EC2Instance object and makes changes based on the state read
// and what is in the EC2Instance.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=ec2instances,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileEC2Instance) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the EC2Instance instance
	instance := &eccv1alpha1.EC2Instance{}
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

	// check for the subnet that the instance will be launched into and grab the subnetid
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

	ec2SecurityGroup := &eccv1alpha1.EC2SecurityGroup{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.EC2SecurityGroupName, Namespace: instance.Namespace}, ec2SecurityGroup)
	if err != nil {
		if errors.IsNotFound(err) {
			println(err)
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Can't find EC2SecurityGroup")
			return reconcile.Result{}, fmt.Errorf(`EC2SecurityGroup not ready`)
		}
		return reconcile.Result{}, err
	} else if len(ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "EC2SecurityGroup has no ID annotation")
		return reconcile.Result{}, fmt.Errorf(`EC2SecurityGroup not ready`)
	}

	ec2KeyPair := &eccv1alpha1.EC2KeyPair{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.EC2KeyPair, Namespace: instance.Namespace}, ec2KeyPair)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Can't find KeyPair")
			return reconcile.Result{}, fmt.Errorf(`EC2KeyPair not ready`)
		}
		return reconcile.Result{}, err
	} else if len(ec2KeyPair.ObjectMeta.Annotations[`awsKeyName`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "EC2Keypair has no AWS Key Name to lookup")
		return reconcile.Result{}, fmt.Errorf(`EC2KeyPair not ready`)
	}

	svc := ec2.New(r.sess)
	// get the EC2InstanceId out of the annotations
	// if absent then create
	ec2InstanceId, ok := instance.ObjectMeta.Annotations[`ec2InstanceId`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS EC2Instance in %s", *r.sess.Config.Region)
		reservation, err := svc.RunInstances(&ec2.RunInstancesInput{
			ImageId:      aws.String(instance.Spec.ImageId),
			InstanceType: aws.String(instance.Spec.InstanceType),
			MaxCount:     aws.Int64(1), //this resource is for a single instance
			MinCount:     aws.Int64(1), //this resource is for a single instance
			SubnetId:     aws.String(subnet.ObjectMeta.Annotations[`subnetid`]),
			KeyName:      aws.String(ec2KeyPair.ObjectMeta.Annotations[`awsKeyName`]),
			SecurityGroupIds: []*string{
				aws.String(ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`]),
			}, // need to fix tags
			//TagSpecifications: []*ec2.TagSpecifications,
			UserData: aws.String(base64.StdEncoding.EncodeToString([]byte(instance.Spec.UserData))),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if reservation == nil {
			r.events.Eventf(instance, `Normal`, `CreateFailure`, "Reservation was nil")
			return reconcile.Result{}, fmt.Errorf(`Reservation was nil`)
		}
		if reservation.Instances[0].InstanceId == nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, `reservation.Instances[0].InstanceId was nil`)
			return reconcile.Result{}, fmt.Errorf(`reservation.Instances[0].InstanceId was nil`)
		}

		ec2InstanceId = *reservation.Instances[0].InstanceId
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS EC2Instance (%s)", ec2InstanceId)
		instance.ObjectMeta.Annotations = make(map[string]string)
		instance.ObjectMeta.Annotations[`ec2InstanceId`] = ec2InstanceId
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `ec2instances.ecc.aws.gotopple.com`)

		//add finalizers to each resource that the instance is dependent on
		ec2KeyPair.ObjectMeta.Finalizers = append(ec2KeyPair.ObjectMeta.Finalizers, (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`))
		err = r.Update(context.TODO(), ec2KeyPair)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, "Unable to add finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer to EC2 Key Pair")

		ec2SecurityGroup.ObjectMeta.Finalizers = append(ec2SecurityGroup.ObjectMeta.Finalizers, (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`))
		err = r.Update(context.TODO(), ec2SecurityGroup)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, "Unable to add finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer to EC2 Security Group")

		subnet.ObjectMeta.Finalizers = append(subnet.ObjectMeta.Finalizers, (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`))
		err = r.Update(context.TODO(), subnet)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, "Unable to add finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer to AWS Subnet")

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the EC2Instance resource will not be able to track the created EC2Instance and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS EC2Instance before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			terminateOutput, ierr := svc.TerminateInstances(&ec2.TerminateInstancesInput{
				InstanceIds: []*string{
					aws.String(instance.ObjectMeta.Annotations[`ec2InstanceId`]),
				}})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupEC2InstanceId`: ec2InstanceId},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the EC2Instance: %s", ierr.Error())

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

			} else if terminateOutput == nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupEC2InstanceId`: ec2InstanceId},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the EC2Instance recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`TerminateOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

		// Make sure that there are tags to add before attempting to add them.
		if len(instance.Spec.Tags) >= 1 {
			// Tag the new EC2Instance
			ts := []*ec2.Tag{}
			for _, t := range instance.Spec.Tags {
				ts = append(ts, &ec2.Tag{
					Key:   aws.String(t.Key),
					Value: aws.String(t.Value),
				})
			}
			tagOutput, err := svc.CreateTags(&ec2.CreateTagsInput{
				Resources: []*string{aws.String(ec2InstanceId)},
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

		// must delete
		_, err = svc.TerminateInstances(&ec2.TerminateInstancesInput{
			InstanceIds: []*string{
				aws.String(instance.ObjectMeta.Annotations[`ec2InstanceId`]),
			},
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the EC2Instance: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidInstanceID.NotFound`: /// this might not be right
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The EC2Instance: %s was already deleted", err.Error())
				default:
					return reconcile.Result{}, err
				}
			} else {
				return reconcile.Result{}, err
			}
		}

		// remove finalizer from instances subnet
		for i, f := range subnet.ObjectMeta.Finalizers {
			if f == (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`) {
				subnet.ObjectMeta.Finalizers = append(
					subnet.ObjectMeta.Finalizers[:i],
					subnet.ObjectMeta.Finalizers[i+1:]...)
			}
		}
		// remove finalizer from instance keypair
		for i, f := range ec2KeyPair.ObjectMeta.Finalizers {
			if f == (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`) {
				ec2KeyPair.ObjectMeta.Finalizers = append(
					ec2KeyPair.ObjectMeta.Finalizers[:i],
					ec2KeyPair.ObjectMeta.Finalizers[i+1:]...)
			}
		}
		// remove finalizer from instance securitygroup
		for i, f := range ec2SecurityGroup.ObjectMeta.Finalizers {
			if f == (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`) {
				ec2SecurityGroup.ObjectMeta.Finalizers = append(
					ec2SecurityGroup.ObjectMeta.Finalizers[:i],
					ec2SecurityGroup.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `ec2instances.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		err = r.Update(context.TODO(), subnet)
		if err != nil {
			r.events.Eventf(subnet, `Warning`, `ResourceUpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Eventf(subnet, `Normal`, `Deleted`, "Deleted finalizer: %s", (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`))

		err = r.Update(context.TODO(), ec2KeyPair)
		if err != nil {
			r.events.Eventf(ec2KeyPair, `Warning`, `ResourceUpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Eventf(ec2KeyPair, `Normal`, `Deleted`, "Deleted finalizer: %s", (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`))

		err = r.Update(context.TODO(), ec2SecurityGroup)
		if err != nil {
			r.events.Eventf(ec2SecurityGroup, `Warning`, `ResourceUpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Eventf(ec2SecurityGroup, `Normal`, `Deleted`, "Deleted finalizer: %s", (ec2InstanceId + `.ec2instances.ecc.aws.gotopple.com`))

		// after a successful delete update the resource with the removed finalizer
		err = r.Update(context.TODO(), instance)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted EC2Instance and removed finalizers")

	}

	return reconcile.Result{}, nil
}
