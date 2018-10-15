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

package ec2keypair

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
	//"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new EC2KeyPair Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`ec2keypair-controller`)
	return &ReconcileEC2KeyPair{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("ec2keypair-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to EC2KeyPair
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.EC2KeyPair{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileEC2KeyPair{}

// ReconcileEC2KeyPair reconciles a EC2KeyPair object
type ReconcileEC2KeyPair struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a EC2KeyPair object and makes changes based on the state read
// and what is in the EC2KeyPair.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=ec2keypairs,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileEC2KeyPair) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the EC2KeyPair instance
	instance := &eccv1alpha1.EC2KeyPair{}
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
	// get the EC2KeyPairId out of the annotations
	// if absent then create
	awsKeyName, ok := instance.ObjectMeta.Annotations[`awsKeyName`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS EC2KeyPair in %s", *r.sess.Config.Region)
		createOutput, err := svc.CreateKeyPair(&ec2.CreateKeyPairInput{
			KeyName: aws.String(instance.Spec.EC2KeyPairName),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if createOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`CreateEC2KeyPairOutput was nil`)
		}

		awsKeyName = *createOutput.KeyName
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS EC2KeyPair (%s)", awsKeyName)
		instance.ObjectMeta.Annotations[`awsKeyName`] = awsKeyName
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `ec2keypairs.ecc.aws.gotopple.com`)
		print(*createOutput.KeyFingerprint)
		print(*createOutput.KeyMaterial)

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the EC2KeyPair resource will not be able to track the created EC2KeyPair and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS EC2KeyPair before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			deleteOutput, ierr := svc.DeleteKeyPair(&ec2.DeleteKeyPairInput{
				KeyName: aws.String(awsKeyName),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupEC2KeyPairName`: awsKeyName},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete the EC2KeyPair: %s", ierr.Error())

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
					map[string]string{`cleanupEC2KeyPairName`: awsKeyName},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the EC2KeyPair recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`DeleteEC2KeyPairOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

		// logic to generate kubernetes secret based off ec2keypair here
		// define the secret
		keySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instance.Name + "-private-key",
				Namespace: instance.Namespace,
			},
			Data: map[string][]byte{
				"PrivateKey":  []byte(*createOutput.KeyMaterial),
				"FingerPrint": []byte(*createOutput.KeyFingerprint),
			},
		}
		// create the Secret
		err = r.Create(context.TODO(), keySecret)

		//keySecret.ObjectMeta.Annotations[`generatedBy`] = awsKeyName
		keySecret.ObjectMeta.Finalizers = append(keySecret.ObjectMeta.Finalizers, `ec2keypairs.ecc.aws.gotopple.com`)
		r.events.Event(keySecret, `Normal`, `Annotated`, "Added finalizer and annotations")

		err = r.Update(context.TODO(), keySecret)

	} else if instance.ObjectMeta.DeletionTimestamp != nil {
		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `ec2keypairs.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		// must delete
		_, err = svc.DeleteKeyPair(&ec2.DeleteKeyPairInput{
			KeyName: aws.String(awsKeyName),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the EC2KeyPair: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidSubnetID.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The EC2KeyPair: %s was already deleted", err.Error())
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
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted EC2KeyPair and removed finalizers")
	}

	return reconcile.Result{}, nil
}
