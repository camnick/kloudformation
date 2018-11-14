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

package ec2volumeattachment

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

// Add creates a new EC2VolumeAttachment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`ec2volumeattachment-controller`)
	return &ReconcileEC2VolumeAttachment{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("ec2volumeattachment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to EC2VolumeAttachment
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.EC2VolumeAttachment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileEC2VolumeAttachment{}

// ReconcileEC2VolumeAttachment reconciles a EC2VolumeAttachment object
type ReconcileEC2VolumeAttachment struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a EC2VolumeAttachment object and makes changes based on the state read
// and what is in the EC2VolumeAttachment.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=ec2volumeattachments,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileEC2VolumeAttachment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the EC2VolumeAttachment instance
	instance := &eccv1alpha1.EC2VolumeAttachment{}
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

	volume := &eccv1alpha1.Volume{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.VolumeName, Namespace: instance.Namespace}, volume)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Can't find Volume")
			return reconcile.Result{}, fmt.Errorf(`Volume not ready`)
		}
		return reconcile.Result{}, err
	} else if len(volume.ObjectMeta.Annotations[`volumeId`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "Volume has no ID annotation")
		return reconcile.Result{}, fmt.Errorf(`Volume not ready`)
	}

	ec2Instance := &eccv1alpha1.EC2Instance{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.EC2InstanceName, Namespace: instance.Namespace}, ec2Instance)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Can't find EC2Instance")
			return reconcile.Result{}, fmt.Errorf(`EC2Instance not ready`)
		}
		return reconcile.Result{}, err
	} else if len(ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "EC2Instance has no ID annotation")
		return reconcile.Result{}, fmt.Errorf(`EC2Instance not ready`)
	}

	svc := ec2.New(r.sess)
	// get the EC2VolumeAttachmentId out of the annotations
	// if absent then create
	ec2VolumeAttachmentResponse, ok := instance.ObjectMeta.Annotations[`ec2VolumeAttachmentResponse`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS EC2VolumeAttachment in %s", *r.sess.Config.Region)
		attachOutput, err := svc.AttachVolume(&ec2.AttachVolumeInput{
			Device:     aws.String(instance.Spec.DevicePath),
			InstanceId: aws.String(ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`]),
			VolumeId:   aws.String(volume.ObjectMeta.Annotations[`volumeId`]),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if attachOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`EC2VolumeAttachmentOutput was nil`)
		}

		if attachOutput.State == nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, `attachOutput.State was nil`)
			return reconcile.Result{}, fmt.Errorf(`attachOutput.State was nil`)
		}
		ec2VolumeAttachmentResponse = *attachOutput.State
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS EC2VolumeAttachment for VolumeId %s ", volume.ObjectMeta.Annotations[`volumeId`])
		instance.ObjectMeta.Annotations = make(map[string]string)

		// Will appear to be 'attaching' later on can have this update to 'attached'
		instance.ObjectMeta.Annotations[`ec2VolumeAttachmentResponse`] = ec2VolumeAttachmentResponse

		// Label both involved resources with each other's IDs. For now. This only works w/ one volume per EC2Instance
		ec2Instance.ObjectMeta.Annotations[`attachedVolumes`] = volume.ObjectMeta.Annotations[`volumeId`]

		volume.ObjectMeta.Annotations[`instanceAttachedTo`] = ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`]

		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `ec2volumeattachments.ecc.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the EC2VolumeAttachment resource will not be able to track the created EC2VolumeAttachment and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS EC2VolumeAttachment before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			detachOutput, ierr := svc.DetachVolume(&ec2.DetachVolumeInput{
				VolumeId: aws.String(volume.ObjectMeta.Annotations[`volumeId`]),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupEC2VolumeAttachmentName`: instance.Name},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the EC2VolumeAttachment: %s", ierr.Error())

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

			} else if detachOutput == nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupEC2VolumeAttachmentName`: instance.Name},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the EC2VolumeAttachment recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeleteEC2VolumeAttachmentOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

	} else if instance.ObjectMeta.DeletionTimestamp != nil {
		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `ec2volumeattachments.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// must delete
		_, err = svc.DetachVolume(&ec2.DetachVolumeInput{
			VolumeId: aws.String(volume.ObjectMeta.Annotations[`volumeId`]),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the EC2VolumeAttachment: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidVolumeID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The EC2VolumeAttachment for Volume: %s was already deleted", err.Error())
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
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted EC2VolumeAttachment and removed finalizers")
	}

	return reconcile.Result{}, nil
}
