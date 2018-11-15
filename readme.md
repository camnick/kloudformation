# Kloudformation

  Kloudformation is a (proof of concept) 1:1 translation of AWS Cloudformation resources into Kubernetes using custom resource definitions and the Kubebuilder scaffolding. Additionally, the DockerSwarm resource (also proof of concept at this time) abstracts multiple resources into a ready to go Docker Swarm cluster with sensible(?) defaults.

## Kloudformation basic resources

### AuthorizeEC2SecurityGroupIngress
#### Description
  AuthorizeEC2SecurityGroupIngress (soon to be renamed... too long) creates an ingress rule for an AWS EC2 Security Group
#### Spec Fields:
```
  - ruleName # string- k8s name of the rule. Used in the finalizer applied to the security group.
  - sourceCidrIp # string- ex. "0.0.0.0/0"
  - ec2SecurityGroupName # string- k8s name of the EC2SecurityGroup to assign the rule to.
  - fromPort # integer
  - toPort # integer
  - ipProtocol # string- tcp, udp, icmp, or protocol number. -1 is all protocols
```
#### Dependencies:
  - EC2SecurityGroup

### EC2Instance
#### Description
  An EC2 instance. Launches 1 instance.
#### Spec Fields:
```
  - imageId # string- AMI number.
  - instanceType # string- ex. "t2.micro"
  - subnetName # k8s name of the subnet to be launched in
  - userData # Use plaintext- Will be base64 encoded by the controller
  - ec2KeyPair
  - ec2SecurityGroupName # k8s name
  - tags
```
#### Dependencies:
  - Subnet
  - EC2KeyPair
  - EC2SecurityGroup

### EC2KeyPair
#### Description
An EC2 Keypair
#### Spec Fields:
```
  - ec2KeyPairName #
```
#### Dependencies:

### EC2SecurityGroup
#### Description
  Creates an AWS EC2 Security Group.
#### Spec Fields:
```
  - ec2SecurityGroupName # AWS name for the security group.
  - vpcName # k8s name of the AWS VPC to place the security group in.
  - description
  - tags
```
#### Dependencies:
  - EC2SecurityGroup
  - VPC

### EC2VolumeAttachment
#### Description
  Attaches an EBS Volume to an EC2 Instance.
#### Spec Fields:
```
  - devicePath
  - volumeName # k8s name of the AWS Volume
  - ec2InstanceName # k8s of the AWS EC2 instance to attach to
```  
#### Dependencies:
  - Volume
  - EC2Instance

### EIP
#### Description
  Creates an AWS Elastic IP
#### Spec Fields:
```
  - vpcName # k8s name of the AWS VPC to assign the EIP to.
  - tags
```  
#### Dependencies:
  - VPC

### EIPAssociation
#### Description
  Associates an EIP with an EC2 Instance.
#### Spec Fields:
```
  - allocationName # k8s name of an EIP to assign to an EC2 instance
  - ec2InstanceName # k8s name of the EC2 instance to assign the EIP to
```  
#### Dependencies:
  - EIP
  - EC2Instance

### InternetGateway
#### Description
  Creates an AWS Internet Gateway
#### Spec Fields:
```
  - vpcName
  - tags
```  
#### Dependencies:
  - VPC

### InternetGatewayAttachment
#### Description
#### Spec Fields:
```
  - vpcName
  - internetGatewayName
```
#### Dependencies:
  - VPC
  - InternetGateway

### NATGateway
#### Description
#### Spec Fields:
```
  - subnetName
  - eipAllocationName
  - tags
```  
#### Dependencies:
  - Subnet
  - EIP

### Route
#### Description
  Route to an InternetGateway
#### Spec Fields:
```
  - destinationCidrBlock
  - routeTableName
  - gatewayName
```  
#### Dependencies:
  - RouteTable
  - InternetGateway

### RouteTable
#### Description
#### Spec Fields:
```
  - vpcName
  - tags
```  
#### Dependencies:
  - VPC

### RouteTableAssociation
#### Description
#### Spec Fields:
```
  - subnetName
  - routeTableName
```  
#### Dependencies:
  - Subnet
  - RouteTable

### Subnet
#### Description
#### Spec Fields:
```
  - vpcName
  - availabilityZone
  - cidrBlock
  - tags
```  
#### Dependencies:
  - VPC

### Volume
#### Description
#### Spec Fields:
```
  - availabilityZone
  - size
  - volumeType
  - tags
```  
#### Dependencies:

### VPC
#### Description
#### Spec Fields:
```
  - cidrBlock
  - enableDnsSupport
  - enableDnsHostnames
  - instanceTenancy
  - tags
```  
#### Dependencies:

### AddRoleToInstanceProfile
#### Description
#### Spec Fields:
```
  - iamInstanceProfileName
  - iamRoleName
```  
#### Dependencies:
  - IAMInstanceProfile
  - Role

### IAMAttachRolePolicy
#### Description
#### Spec Fields:
```
  - iamPolicyName
  - iamRoleName
```  
#### Dependencies:
  - IAMPolicy
  - Role

### IAMInstanceProfile
#### Description
#### Spec Fields:
```
  - iamInstanceProfileName
  - path
```  
#### Dependencies:

### IAMPolicy
#### Description
#### Spec Fields:
```
  - description
  - path
  - policyDocument
  - policyName
```  
#### Dependencies:

### Role
#### Description
#### Spec Fields:
```
  - assumeRolePolicyDocument
  - description
  - maxSessionDuration
  - path
  - roleName
```  
#### Dependencies:

## Kloudformation Advanced Resources

### DockerSwarm
#### Description
#### Spec Fields:
```
  - numManagers
  - numWorkers
  - managerSize
  - workerSize
```  
#### Dependencies:

## Getting Started with Basic Resources

## Getting Started with Advanced Resources

## Notes
  - Need to make all names in CRDs specify aws or k8s for clarity. ex. awsKeyPairName or k8sKeyPairName.
  - Need to significantly shorten some names, and just change others for clarity and ease of use.
