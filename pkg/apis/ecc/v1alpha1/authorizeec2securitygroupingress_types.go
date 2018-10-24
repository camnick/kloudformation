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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AuthorizeEC2SecurityGroupIngressSpec defines the desired state of AuthorizeEC2SecurityGroupIngress
type AuthorizeEC2SecurityGroupIngressSpec struct {
	DestinationCidrIp 			string 	`json:"destinationCidrIp"`
	EC2SecurityGroupName		string	`json:"ec2SecurityGroupName"`
	FromPort								int			`json:"fromPort"`
	ToPort									int			`json:"toPort"`
	IpProtocol							string	`json:"ipProtocol"`
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// AuthorizeEC2SecurityGroupIngressStatus defines the observed state of AuthorizeEC2SecurityGroupIngress
type AuthorizeEC2SecurityGroupIngressStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AuthorizeEC2SecurityGroupIngress is the Schema for the authorizeec2securitygroupingresses API
// +k8s:openapi-gen=true
type AuthorizeEC2SecurityGroupIngress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthorizeEC2SecurityGroupIngressSpec   `json:"spec,omitempty"`
	Status AuthorizeEC2SecurityGroupIngressStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AuthorizeEC2SecurityGroupIngressList contains a list of AuthorizeEC2SecurityGroupIngress
type AuthorizeEC2SecurityGroupIngressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthorizeEC2SecurityGroupIngress `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthorizeEC2SecurityGroupIngress{}, &AuthorizeEC2SecurityGroupIngressList{})
}
