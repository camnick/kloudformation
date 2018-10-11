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

// EC2InstanceSpec defines the desired state of EC2Instance
type EC2InstanceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	//AdditionalInfo 										string 																	`json:"additionalInfo"`
	//BlockDeviceMappings 							[]BlocklDeviceMapping 									`json:"blockDeviceMapping"`
	//ClientToken 											string 																	`json:"clientToken"`
	//CpuOptions 												struct 																	`json:"cpuOptions"`
	//CreditSpecification 							struct 																	`json:"creditSpecification"`
	//DisableApiTermination 						bool 																		`json:"disableApiTermination"`
	//DryRun 														bool 																		`json:"dryRun"`
	//EbsOptimized 											bool 																		`json:"ebsOptimized"`
	//ElasticGpuSpecification 					[]ElasticGpuSpecification 							`json:"elasticGpuSpecification"`
	//IamInstanceProfile 								struct 																	`json:"iamInstanceProfile"`
	ImageId string `json:"imageId"`
	//InstanceInitatedShutdownBehavior 	string 																	`json:"instancInitiatedShutdownBehavior"`
	//InstanceMarketOptions 						struct 																	`json:"instanceMarketOptions"`
	InstanceType string `json:"instanceType"`
	//Ipv6AddressCount 									int64 																	`json:"ipv6AddressCount"`
	//Ipv6Addresses 										[]InstanceIpv6Address 									`json"ipv6Address"`
	//KernalId 													string 																	`json:"kernalId"`
	//KeyName 													string 																	`json:"keyName"`
	//LaunchTemplate 										struct 																	`json:"launchTemplate"`
	//MaxCount int64 `json:"maxCount"`
	//MinCount int64 `json:"minCount"`
	//Monitoring												struct																	`json:"monitoring"`
	//NetworkInterfaces									[]InstanceNetworkInterfaceSpecification	`json:"networkInterfaces"`
	//Placement													struct																	`json:"placement"`
	//PrivateIpAddress									string																	`json:"privateIpAddress"`
	//RamDiskId													string																	`json:"ramDiskId"`
	//SecurityGroupIds									[]SecurityGroupId												`json:"securityGroupId"` // I don't think this is right
	//SecurityGroups										[]SecurityGroup													`json:"securityGroup"`
	SubnetName string `json:"subnetName"` //Will look up the Subnet by K8S name and retireve ID from annotations
	//Tags              []ResourceTag `json:"tags"`
	//UserData													string																	`json:"userData"`
}

// EC2InstanceStatus defines the observed state of EC2Instance
type EC2InstanceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EC2Instance is the Schema for the ec2instances API
// +k8s:openapi-gen=true
type EC2Instance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EC2InstanceSpec   `json:"spec,omitempty"`
	Status EC2InstanceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EC2InstanceList contains a list of EC2Instance
type EC2InstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EC2Instance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EC2Instance{}, &EC2InstanceList{})
}
