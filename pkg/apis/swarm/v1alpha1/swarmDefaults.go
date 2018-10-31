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

package v1alpha1

import (
	eccv1alpha1 "github.com/gotopple/kloudformation/pkg/apis/ecc/v1alpha1"
	iamv1alpha1 "github.com/gotopple/kloudformation/pkg/apis/iam/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type swarmDefaults struct {
	metav1.TypeMeta                       `json:",inline"`
	metav1.ObjectMeta                     `json:"metadata,omitempty"`
	eccv1alpha1.VPC                       `json:"VPC"`
	eccv1alpha1.Subnet                    `json:"Subnet"`
	ElasticIps                            []eccv1alpha1.EIP
	eccv1alpha1.InternetGateway           `json:"internetGateway"`
	eccv1alpha1.InternetGatewayAttachment `json:"internetGatewayAttachment"`
	eccv1alpha1.NATGateway                `json:"natGateway"`
	eccv1alpha1.RouteTable                `json:"routeTable"`
	eccv1alpha1.RouteTableAssociation     `json:"routeTableAssociation"`
	eccv1alpha1.Route                     `json:"route"`
	eccv1alpha1.EC2SecurityGroup          `json:"ec2SecurityGroup"`
	IngressRules                          []eccv1alpha1.AuthorizeEC2SecurityGroupIngress
	eccv1alpha1.EC2KeyPair                `json:"ec2KeyPair"`
	ManagerInfo                           
	WorkerInfo
	SwarmIamPolicies               []iamv1alpha1.IAMPolicy
	SwarmRoles                     []iamv1alpha1.Role
	SwarmInstanceProfiles          []iamv1alpha1.IAMInstanceProfile
	SwarmAttachRolePolicies        []iamv1alpha1.IAMAttachRolePolicy
	SwarmAddRoleToInstanceProfiles []iamv1alpha1.IAMInstanceProfile
}

type ManagerInfo struct {
	eccv1alpha1.EC2Instance `json:"managerEC2Info"`
}

type WorkerInfo struct {
	eccv1alpha1.EC2Instance `json:"workerEC2Info"`
}
