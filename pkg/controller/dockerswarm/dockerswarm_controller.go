/*
Copyright 2018 The KloudFormation authors.

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

package dockerswarm

import (
	"context"
	//"fmt"
	//"log"
	"reflect"

	eccv1alpha1 "github.com/gotopple/kloudformation/pkg/apis/ecc/v1alpha1"
	swarmv1alpha1 "github.com/gotopple/kloudformation/pkg/apis/swarm/v1alpha1"
	//testv1alpha1 "github.com/gotopple/kloudformation/pkg/apis/test/v1alpha1"
	//iamv1alpha1 "github.com/gotopple/kloudformation/pkg/apis/iam/v1alpha1"
	//appsv1 "k8s.io/api/apps/v1"
	//corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new DockerSwarm Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this dockerswarm.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := mgr.GetRecorder(`dockerswarm-controller`)
	return &ReconcileDockerSwarm{Client: mgr.GetClient(), scheme: mgr.GetScheme(), events: r}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("dockerswarm-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to DockerSwarm
	err = c.Watch(&source.Kind{Type: &swarmv1alpha1.DockerSwarm{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a VPC created by DockerSwarm - change this for objects you create
	err = c.Watch(&source.Kind{Type: &eccv1alpha1.VPC{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &eccv1alpha1.VPC{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileDockerSwarm{}

// ReconcileDockerSwarm reconciles a DockerSwarm object
type ReconcileDockerSwarm struct {
	client.Client
	scheme *runtime.Scheme
	events record.EventRecorder
}

// Reconcile reads that state of the cluster for a DockerSwarm object and makes changes based on the state read
// and what is in the DockerSwarm.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// Automatically generate RBAC rules to allow the Controller to read and write VPCs
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=swarm.aws.gotopple.com,resources=dockerswarms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ecc.aws.gotopple.com,resources=subnets,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileDockerSwarm) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the DockerSwarm instance
	instance := &swarmv1alpha1.DockerSwarm{}
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

	// TODO(user): Change this to be the object type created by your controller
	// Define the desired VPC object
	r.events.Eventf(instance, `Normal`, `Info`, "Defining swarm VPC")
	vpc := &eccv1alpha1.VPC{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-vpc",
			Namespace: instance.Namespace,
		},
		Spec: eccv1alpha1.VPCSpec{
			CIDRBlock:          "10.20.0.0/16",
			EnableDNSSupport:   true,
			EnableDNSHostnames: true,
			InstanceTenancy:    "default",
			Tags: []eccv1alpha1.ResourceTag{
				{
					Key:   "Name",
					Value: "SwarmVPC",
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(instance, vpc, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// TODO(user): Change this for the object type created by your controller
	// Check if the VPC already exists
	r.events.Eventf(instance, `Normal`, `Info`, "Checking if swarm VPC exists")
	vpcFound := &eccv1alpha1.VPC{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: vpc.Name, Namespace: vpc.Namespace}, vpcFound)

	if err != nil && errors.IsNotFound(err) {
		r.events.Eventf(instance, `Normal`, `Info`, "Creating swarm VPC")
		err = r.Create(context.TODO(), vpc)
		if err != nil {
			r.events.Eventf(instance, `Normal`, `Info`, "Error in creating swarm VPC %s", err.Error())
			return reconcile.Result{}, err
		}
	} else if err != nil {
		return reconcile.Result{}, err
	}

	r.events.Eventf(instance, `Normal`, `Info`, "Assuming VPC exists?!?")

	// TODO(user): Change this for the object type created by your controller
	// Update the found object and write the result back if there are any changes
	if !reflect.DeepEqual(vpc.Spec, vpcFound.Spec) {
		vpcFound.Spec = vpc.Spec
		r.events.Eventf(instance, `Normal`, `Info`, "Updating swarm VPC")
		err = r.Update(context.TODO(), vpcFound)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
