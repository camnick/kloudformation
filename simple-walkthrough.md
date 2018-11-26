# A Basic Walkthrough with Kloudformation

At it's lowest level, Kloudformation is a 1:1 abstraction of AWS Cloudformation resources into Kubernetes objects.

First, clone the repo

<NEED TO INSERT BLOCK ABOUT CLONING AND SETTING UP DEPENDENCIES>


To get started with a simple setup, let's create an EC2 instance within a VPC that us running a Tomcat web server. In order to get this up and running, we'll need to create the following Kubernetes Kloudformation resources:

- VPC
- Subnet
- 2x EIP (Elastic IP Addresses)
- NATGateway (Will consume one of the EIP resources)
- InternetGateway
- InternetGatewayAttachment
- RouteTable
- RouteTableAssociation
- Route
- EC2SecurityGroup
- 2x AuthorizeEC2SecurityGroupIngress (Ingress rules for ssh and http. Sorry- this name is going to change to something much shorter)
- EC2Instance
- EIPAssociation (This will associate the second EIP with the EC2Instance)
- EC2KeyPair

We're also going to be doing this in a separate namespace, to keep things tidy, so we'll also be using a Kubernetes object to define the namespace `kloudformation`

The namespace file is located in `example/walkthrough`, and all of the Kloudformation resources are located in `example/walkthrough/resources`

First, apply the namespace `kubectl apply -f example/walkthrough/kloudformation-namespace.yaml`

Optional: You can setup a second terminal window to watch the events as they occur with the command: `kubectl get events --watch --namespace kloudformation` Things are going to go by quickly- but you'll see that the order of resource creation doesn't matter, as objects that are dependent on others will retry until all prerequisites are met. For example, the EC2 Instance controller will keep attempting to create an instance until the required subnet, ec2 keypair, and security group are all present.

You could apply all of the following files at once, but let's do the first few individually, just to take note of some of what's going on.

Apply the VPC file: `kubectl apply -f example/walkthrough/resources/vpc.yaml`

Watch the log, as soon as it's ready, in another terminal window, check out the annotations of the vpc: `kubectl describe vpc --namespace kloudformation`

The annotations should look like this: `Annotations:  vpcid=vpc-xxxxxxxxxxxxxxxxx` with some identifier in the place of the string of x's.

Further down, you can see the events that occurred during creation:
```
Events:
  Type    Reason         Age   From            Message
  ----    ------         ----  ----            -------
  Normal  CreateAttempt  3m    vpc-controller  Creating AWS VPC in us-west-2
  Normal  Created        3m    vpc-controller  Created AWS VPC (vpc-xxxxxxxxxxxxxxxxx)
  Normal  Annotated      3m    vpc-controller  Added finalizer and annotations
  Normal  Tagged         3m    vpc-controller  Added tags
```

Next, apply the EC2 Security Group file: `kubectl apply -f example/walkthrough/resources/securitygroup.yaml`

Again, check out the annotations. You should only see the AWS Security Group ID: `kubectl describe ec2securitygroup --namespace kloudformation`. The event log for creating the security group will look pretty much the same as the VPC's log.

Now, let's apply an ingress rules to the security group and see what happens: `kubectl apply -f example/walkthrough/resources/ingress-rule-ssh.yaml`
