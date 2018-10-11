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

package volumeattachment

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

// Add creates a new VolumeAttachment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`volumeattachment-controller`)
	return &ReconcileVolumeAttachment{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("volumeattachment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to VolumeAttachment
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.VolumeAttachment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileVolumeAttachment{}

// ReconcileVolumeAttachment reconciles a VolumeAttachment object
type ReconcileVolumeAttachment struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a VolumeAttachment object and makes changes based on the state read
// and what is in the VolumeAttachment.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=volumeattachments,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileVolumeAttachment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the VolumeAttachment instance
	instance := &eccv1alpha1.VolumeAttachment{}
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
			print("can't find volume")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	} else if len(volume.ObjectMeta.Annotations[`volumeId`]) <= 0 {
		print("volume has no volume id")
		return reconcile.Result{}, fmt.Errorf(`Volume not ready`)
	}
	print("working with volume: ", volume.ObjectMeta.Annotations[`volumeId`])

	ec2Instance := &eccv1alpha1.EC2Instance{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.EC2InstanceName, Namespace: instance.Namespace}, ec2Instance)
	if err != nil {
		if errors.IsNotFound(err) {
			print(" can't find the ec2 instance ")
			print(" the spec calls for: ", instance.Spec.EC2InstanceName)
			print(" would like to be referencing id: ", ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`])
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	} else if len(ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`]) <= 0 {
		print("the ec2 instance id isn't right")
		return reconcile.Result{}, fmt.Errorf(`EC2Instance not ready`)
	}
	print("working with ec2Instance: ", ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`])

	svc := ec2.New(r.sess)
	// get the VolumeAttachmentId out of the annotations
	// if absent then create
	volumeAttachmentState, ok := instance.ObjectMeta.Annotations[`volumeAttachmentState`]
	if !ok {
		print(" Going to attempt to create the attachment ")
		print(" The mount path will be: ", instance.Spec.DevicePath)
		print(" The volumeId is: ", volume.ObjectMeta.Annotations[`volumeId`])
		print(" The ec2InstanceId is: ", ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`])
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS VolumeAttachment in %s", *r.sess.Config.Region)
		attachOutput, err := svc.AttachVolume(&ec2.AttachVolumeInput{
			Device:     aws.String(instance.Spec.DevicePath),
			InstanceId: aws.String(ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`]),
			VolumeId:   aws.String(volume.ObjectMeta.Annotations[`volumeId`]),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			print("The attachment didn't work.")
			fmt.Println(err.Error())
			return reconcile.Result{}, err
		}
		if attachOutput == nil {
			print("the attachment output was nil")
			return reconcile.Result{}, fmt.Errorf(`VolumeAttachmentOutput was nil`)
		}

		volumeAttachmentState = *attachOutput.State
		print("the attachment state is: ", volumeAttachmentState)
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS VolumeAttachment for Volume %s on EC2Instance %s", volume.ObjectMeta.Annotations[`volumeId`], ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`])
		instance.ObjectMeta.Annotations[`volumeAttachmentState`] = volumeAttachmentState

		// Label both involved resources with each other's IDs. For now. This only works w/ one volume per EC2Instance
		ec2Instance.ObjectMeta.Annotations[`attachedVolumes`] = volume.ObjectMeta.Annotations[`volumeId`]
		volume.ObjectMeta.Annotations[`instanceAttachedTo`] = ec2Instance.ObjectMeta.Annotations[`ec2InstanceId`]

		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `volumeattachments.ecc.aws.gotopple.com`)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the VolumeAttachment resource will not be able to track the created VolumeAttachment and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS VolumeAttachment before
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
					map[string]string{`cleanupVolumeAttachmentName`: instance.Name},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the VolumeAttachment: %s", ierr.Error())

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
					map[string]string{`cleanupVolumeAttachmentName`: instance.Name},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the VolumeAttachment recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeleteVolumeAttachmentOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

	} else if instance.ObjectMeta.DeletionTimestamp != nil {
		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `volumeattachments.ecc.aws.gotopple.com` {
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
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the VolumeAttachment: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidVolumeID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The VolumeAttachment for Volume: %s was already deleted", err.Error())
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
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted VolumeAttachment and removed finalizers")
	}

	return reconcile.Result{}, nil
}
