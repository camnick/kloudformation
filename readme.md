# Kloudformation

  Kloudformation is a (proof of concept) 1:1 translation of AWS Cloudformation resources into Kubernetes using custom resource definitions and the Kubebuilder scaffolding. Additionally, the DockerSwarm resource (also proof of concept at this time) abstracts multiple resources into a ready to go Docker Swarm cluster with sensible(?) defaults.

## Kloudformation basic resources

### AuthorizeEC2SecurityGroupIngress
#### Description
#### Spec Fields:
  * ruleName
  * sourceCidrIp
  * ec2ec2SecurityGroupName
  * fromPort
  * toPort
  * ipProtocol

#### Dependencies:
  * EC2SecurityGroup

### EC2Instance
#### Description
#### Spec Fields:
  * imageId
  * instanceType
  * subnetName
  * userData
  * ec2KeyPair
  * ec2SecurityGroupName
  * tags

#### Dependencies:
  * Subnet
  * EC2KeyPair
  * EC2SecurityGroup

### EC2KeyPair
#### Description
#### Spec Fields:
  * ec2KeyPairName

#### Dependencies:

### EC2SecurityGroup
#### Description
#### Spec Fields:
  * ec2SecurityGroupName
  * vpcName
  * description
  * tags

#### Dependencies:
  * EC2SecurityGroup
  * VPC

### EC2VolumeAttachment
#### Description
#### Spec Fields:
  * devicePath
  * volumeName
  * ec2InstanceName
#### Dependencies:
  * Volume
  * EC2Instance

### EIP
#### Description
#### Spec Fields:
  * vpcName
  * tags
#### Dependencies:
  * VPC

### EIPAssociation
#### Description
#### Spec Fields:
  * allocationName
  * ec2InstanceName
#### Dependencies:
  * EIP
  * EC2Instance

### InternetGateway
#### Description
#### Spec Fields:
  * vpcName
  * tags
#### Dependencies:
  * VPC

### InternetGatewayAttachment
#### Description
#### Spec Fields:
  * vpcName
  * internetGatewayName

#### Dependencies:
  * VPC
  * InternetGateway

### NATGateway
#### Description
#### Spec Fields:
  * subnetName
  * eipAllocationName
  * tags
#### Dependencies:
  * Subnet
  * EIP

### Route
#### Description
  Route to an InternetGateway
#### Spec Fields:
  * destinationCidrBlock
  * routeTableName
  * gatewayName
#### Dependencies:
  * RouteTable
  * InternetGateway

### RouteTable
#### Description
#### Spec Fields:
  * vpcName
  * tags
#### Dependencies:
  * VPC

### RouteTableAssociation
#### Description
#### Spec Fields:
  * subnetName
  * routeTableName
#### Dependencies:
  * Subnet
  * RouteTable

### Subnet
#### Description
#### Spec Fields:
  * vpcName
  * availabilityZone
  * cidrBlock
  * tags
#### Dependencies:
  * VPC

### Volume
#### Description
#### Spec Fields:
  * availabilityZone
  * size
  * volumeType
  * tags
#### Dependencies:

### VPC
#### Description
#### Spec Fields:
  * cidrBlock
  * enableDnsSupport
  * enableDnsHostnames
  * instanceTenancy
  * tags
#### Dependencies:

### AddRoleToInstanceProfile
#### Description
#### Spec Fields:
  * iamInstanceProfileName
  * iamRoleName
#### Dependencies:
  * IAMInstanceProfile
  * Role

### IAMAttachRolePolicy
#### Description
#### Spec Fields:
  * iamPolicyName
  * iamRoleName
#### Dependencies:
  * IAMPolicy
  * Role

### IAMInstanceProfile
#### Description
#### Spec Fields:
  * iamInstanceProfileName
  * path
#### Dependencies:

### IAMPolicy
#### Description
#### Spec Fields:
  * description
  * path
  * policyDocument
  * policyName
#### Dependencies:

### Role
#### Description
#### Spec Fields:
  * assumeRolePolicyDocument
  * description
  * maxSessionDuration
  * path
  * roleName
#### Dependencies:

## Kloudformation Advanced Resources

### DockerSwarm
#### Description
#### Spec Fields:
  * numManagers
  * numWorkers
  * managerSize
  * workerSize
#### Dependencies:

## Getting Started with Basic Resources

## Getting Started with Advanced Resources

## Notes
  * Need to make all names in CRDs specify aws or k8s for clarity. ex. awsKeyPairName or k8sKeyPairName.
  * Need to significantly shorten some names, and just change others for clarity and ease of use.
