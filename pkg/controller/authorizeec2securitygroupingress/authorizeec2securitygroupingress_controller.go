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

package authorizeec2securitygroupingress

import (
	"context"
	"encoding/json"
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

// Add creates a new authorizeec2securitygroupingress Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	sess := awssession.Must(awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	}))
	r := mgr.GetRecorder(`authorizeec2securitygroupingress-controller`)
	return &ReconcileAuthorizeEC2SecurityGroupIngress{Client: mgr.GetClient(), scheme: mgr.GetScheme(), sess: sess, events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("authorizeec2securitygroupingress-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to AuthorizeEC2SecurityGroupIngress
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.AuthorizeEC2SecurityGroupIngress{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileAuthorizeEC2SecurityGroupIngress{}

// ReconcileAuthorizeEC2SecurityGroupIngress reconciles a AuthorizeEC2SecurityGroupIngress object
type ReconcileAuthorizeEC2SecurityGroupIngress struct {
	client.Client
	scheme *runtime.Scheme
	sess   *awssession.Session
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a AuthorizeEC2SecurityGroupIngress object and makes changes based on the state read
// and what is in the AuthorizeEC2SecurityGroupIngress.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=authorizeec2securitygroupingress,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileAuthorizeEC2SecurityGroupIngress) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the AuthorizeEC2SecurityGroupIngress instance
	instance := &eccv1alpha1.AuthorizeEC2SecurityGroupIngress{}
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

	// Need to lookup the security group to attach to
	ec2SecurityGroup := &eccv1alpha1.EC2SecurityGroup{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.EC2SecurityGroupName, Namespace: instance.Namespace}, ec2SecurityGroup)
	if err != nil {
		if errors.IsNotFound(err) {
			r.events.Eventf(instance, `Warning`, `CreateFailure`, "Can't find EC2SecurityGroup")
			return reconcile.Result{}, fmt.Errorf(`EC2SecurityGroup not ready`)
		}
		return reconcile.Result{}, err
	} else if len(ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`]) <= 0 {
		r.events.Eventf(instance, `Warning`, `CreateFailure`, "EC2SecurityGroup has no ID annotation")
		return reconcile.Result{}, fmt.Errorf(`EC2SecurityGroup not ready`)
	}

	svc := ec2.New(r.sess)
	// get the AuthorizeEC2SecurityGroupIngressId out of the annotations
	// if absent then create
	ingressAuthorized, ok := instance.ObjectMeta.Annotations[`ingressAuthorized`]
	if !ok {
		r.events.Eventf(instance, `Normal`, `CreateAttempt`, "Creating AWS AuthorizeEC2SecurityGroupIngress in %s", *r.sess.Config.Region)
		authorizeOutput, err := svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			CidrIp:     aws.String(instance.Spec.SourceCidrIp),
			FromPort:   aws.Int64(instance.Spec.FromPort),
			GroupId:    aws.String(ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`]),
			IpProtocol: aws.String(instance.Spec.IpProtocol),
			ToPort:     aws.Int64(instance.Spec.ToPort),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `AuthorizeFailure`, "Create failed: %s", err.Error())
			return reconcile.Result{}, err
		}
		if authorizeOutput == nil {
			return reconcile.Result{}, fmt.Errorf(`AuthorizeOutput was nil`)
		}
		ingressAuthorized = "yes"
		r.events.Eventf(instance, `Normal`, `Created`, "Created AWS AuthorizeEC2SecurityGroupIngress for EC2SecurityGroup (%s)", ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`])
		instance.ObjectMeta.Annotations = make(map[string]string)
		instance.ObjectMeta.Annotations[`ingressAuthorized`] = ingressAuthorized
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, `authorizeec2securitygroupingress.ecc.aws.gotopple.com`)

		// check if security group already has a finalizer placed on it by this controller
		ingressFinalizerPresent := false
		for _, i := range ec2SecurityGroup.ObjectMeta.Finalizers {
			if i == `authorizeec2securitygroupingress.ecc.aws.gotopple.com` {
				ingressFinalizerPresent = true
			}
		}

		// If there isn't a finalizer already on the security group, place it and create an empty list for ingress rules
		if ingressFinalizerPresent != true {
			ec2SecurityGroup.ObjectMeta.Finalizers = append(ec2SecurityGroup.ObjectMeta.Finalizers, `authorizeec2securitygroupingress.ecc.aws.gotopple.com`)
			ec2SecurityGroup.ObjectMeta.Annotations[`ingressRules`] = `[]`
		}

		//add the rule being created to the security groups annotations
		ruleList := []string{}
		err = json.Unmarshal([]byte(ec2SecurityGroup.ObjectMeta.Annotations[`ingressRules`]), &ruleList)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, `Failed to parse ingress rules`)
		}
		ruleList = append(ruleList, instance.Name)
		newAnnotation, err := json.Marshal(ruleList)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, `Failed to update ingress rules`)
		}
		ec2SecurityGroup.ObjectMeta.Annotations[`ingressRules`] = string(newAnnotation)
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer to Security Group")
		r.events.Event(ec2SecurityGroup, `Normal`, `Annotated`, "Added finalizer to Security Group")

		//update the Security Group, now that it's done.
		err = r.Update(context.TODO(), ec2SecurityGroup)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, `Couldn't update Security Group annotations: %s`, err.Error())
			r.events.Eventf(ec2SecurityGroup, `Warning`, `ResourceUpdateFailure`, `Couldn't update Security Group annotations: %s`, err.Error())
		}

		err = r.Update(context.TODO(), instance)
		if err != nil {
			// If the call to update the resource annotations has failed then
			// the AuthorizeEC2SecurityGroupIngress resource will not be able to track the created AuthorizeEC2SecurityGroupIngress and
			// no finalizer will have been appended.
			//
			// This routine should attempt to delete the AWS AuthorizeEC2SecurityGroupIngress before
			// returning the error and retrying.

			r.events.Eventf(instance,
				`Warning`,
				`ResourceUpdateFailure`,
				"Failed to update the resource: %s", err.Error())

			revokeSecurityGroupIngressOutput, ierr := svc.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
				CidrIp:     aws.String(instance.Spec.SourceCidrIp),
				FromPort:   aws.Int64(instance.Spec.FromPort),
				GroupId:    aws.String(ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`]),
				IpProtocol: aws.String(instance.Spec.IpProtocol),
				ToPort:     aws.Int64(instance.Spec.ToPort),
			})
			if ierr != nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupIngressRule`: ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`]},
					`Warning`,
					`DeleteFailure`,
					"Unable to delete (revoke) the AuthorizeEC2SecurityGroupIngress: %s", ierr.Error())

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

			} else if revokeSecurityGroupIngressOutput == nil {
				// Send an appropriate event that has been annotated
				// for async AWS resource GC.
				r.events.AnnotatedEventf(instance,
					map[string]string{`cleanupIngressRule`: ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`]},
					`Warning`,
					`DeleteAmbiguity`,
					"Attempt to delete the AuthorizeEC2SecurityGroupIngress recieved a nil response")
				return reconcile.Result{}, fmt.Errorf(`RevokeSecurityGroupIngressOutput was nil`)
			}
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Annotated`, "Added finalizer and annotations")

	} else if instance.ObjectMeta.DeletionTimestamp != nil {

		// check for other Finalizers
		for _, f := range instance.ObjectMeta.Finalizers {
			if f != `authorizeec2securitygroupingress.ecc.aws.gotopple.com` {
				r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the AuthorizeEC2SecurityGroupIngress with remaining finalizers")
				return reconcile.Result{}, fmt.Errorf(`Unable to delete the AuthorizeEC2SecurityGroupIngress with remaining finalizers`)
			}
		}

		// must delete
		_, err = svc.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
			CidrIp:     aws.String(instance.Spec.SourceCidrIp),
			FromPort:   aws.Int64(instance.Spec.FromPort),
			GroupId:    aws.String(ec2SecurityGroup.ObjectMeta.Annotations[`ec2SecurityGroupId`]),
			IpProtocol: aws.String(instance.Spec.IpProtocol),
			ToPort:     aws.Int64(instance.Spec.ToPort),
		})
		if err != nil {
			r.events.Eventf(instance, `Warning`, `DeleteFailure`, "Unable to delete the AuthorizeEC2SecurityGroupIngress: %s", err.Error())

			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case `InvalidPermission.NotFound`:
					// we want to keep going
					r.events.Eventf(instance, `Normal`, `AlreadyDeleted`, "The AuthorizeEC2SecurityGroupIngress: %s was already deleted", err.Error())
				default:
					return reconcile.Result{}, err
				}
			} else {
				return reconcile.Result{}, err
			}
		}

		//remove the rule name from security group annotations
		ruleList := []string{}
		err = json.Unmarshal([]byte(ec2SecurityGroup.ObjectMeta.Annotations[`ingressRules`]), &ruleList)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, `Failed to parse ingress rules`)
		}
		for i, f := range ruleList {
			if f == instance.Name {
				ruleList = append(ruleList[:i], ruleList[i+1:]...)
			}
		}
		newAnnotation, err := json.Marshal(ruleList)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, `Failed to update ingress rules`)
		}
		ec2SecurityGroup.ObjectMeta.Annotations[`ingressRules`] = string(newAnnotation)

		//check if any rules remain
		if ec2SecurityGroup.ObjectMeta.Annotations[`ingressRules`] == `[]` {
			for i, f := range ec2SecurityGroup.ObjectMeta.Finalizers {
				if f == `authorizeec2securitygroupingress.ecc.aws.gotopple.com` {
					ec2SecurityGroup.ObjectMeta.Finalizers = append(
						ec2SecurityGroup.ObjectMeta.Finalizers[:i],
						ec2SecurityGroup.ObjectMeta.Finalizers[i+1:]...)
				}
			}
		}

		// remove the finalizer
		for i, f := range instance.ObjectMeta.Finalizers {
			if f == `authorizeec2securitygroupingress.ecc.aws.gotopple.com` {
				instance.ObjectMeta.Finalizers = append(
					instance.ObjectMeta.Finalizers[:i],
					instance.ObjectMeta.Finalizers[i+1:]...)
			}
		}

		err = r.Update(context.TODO(), ec2SecurityGroup)
		if err != nil {
			r.events.Eventf(ec2SecurityGroup, `Warning`, `ResourceUpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Eventf(ec2SecurityGroup, `Normal`, `Deleted`, "Deleted finalizer: %s", `authorizeec2securitygroupingress.ecc.aws.gotopple.com`)

		// after a successful delete update the resource with the removed finalizer
		err = r.Update(context.TODO(), instance)
		if err != nil {
			r.events.Eventf(instance, `Warning`, `ResourceUpdateFailure`, "Unable to remove finalizer: %s", err.Error())
			return reconcile.Result{}, err
		}
		r.events.Event(instance, `Normal`, `Deleted`, "Deleted AuthorizeEC2SecurityGroupIngress and removed finalizers")

	}

	return reconcile.Result{}, nil
}
