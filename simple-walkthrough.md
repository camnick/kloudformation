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

We're also going to be doing this in a separate namespace, to keep things tidy, so we'll also be using a Kubernetes object to create the namespace `kloudformation`

The namespace file is located in `example/walkthrough`, and all of the Kloudformation resources are located in `example/walkthrough/resources`

First, apply the namespace `kubectl apply -f example/walkthrough/kloudformation-namespace.yaml`

Optional: You can setup a second terminal window to watch the events as they occur with the command: `kubectl get events --watch --namespace kloudformation`  Later on you'll see that the order of resource creation doesn't matter, as objects that are dependent on others will retry until all prerequisites are met. For example, the EC2 Instance controller will keep attempting to create an instance until the required subnet, ec2 keypair, and security group are all present.

You could apply all of the following files at once, but let's do the first few individually, just to take note of some of what's going on.

Create the VPC by applying the VPC file: `kubectl apply -f example/walkthrough/resources/vpc.yaml`

Watch the log, as soon as it's ready, in another terminal window, check out the annotations of the VPC: `kubectl describe vpc --namespace kloudformation`

The annotations should look like this: `Annotations:  vpcid=vpc-xxxxxxxxxxxxxxxxx` with some identifier in the place of the string of x's.

Further down, you can see the events that occurred during creation: (Yours should look similar)
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

Again, check out the annotations: `kubectl describe ec2securitygroup --namespace kloudformation` You should only see the AWS Security Group ID: `Annotations:  ec2SecurityGroupId=sg-xxxxxxxxxxxxxxxxx`. The event log for creating the security group will look pretty much the same as the VPC's log.

Now, let's apply an ingress rule to the security group and see what happens: `kubectl apply -f example/walkthrough/resources/ingress-rule-ssh.yaml`

Take another gander at the EC2EC2SecurityGroup's annotations:
```
Annotations:  ec2SecurityGroupId=sg-xxxxxxxxxxxxxxxxx
              ingressRules=["ingressrule-walkthrough-ssh"]
```
There is now a list of ingress rules that are applied to the security group. Further down, you can see that the AuthorizeEC2SecurityGroupIngress controller has also placed a finalizers on the security group, to prevent deletion before all of the Kubernetes ingress rule objects have been cleaned up:
```
Finalizers:
    ec2securitygroups.ecc.aws.gotopple.com
    authorizeec2securitygroupingress.ecc.aws.gotopple.com
```

Now, let's add another ingress rule and see how that affects the security group:

`kubectl apply -f example/walkthrough/resources/ingress-rule-http.yaml`

Then check the security group's annotations one more time:

```
Annotations:  ec2SecurityGroupId=sg-xxxxxxxxxxxxxxxxx
              ingressRules=["ingressrule-walkthrough-ssh","ingressrule-walkthrough-http"]
```

Now both applied ingress rules are listed.

Remove the first ingress rule to see what happens next.

`kubectl delete -f example/walkthrough/resources/ingressrule-walkthrough-ssh.yaml`

The list is down to the one remaining rule, and the finalizer is still present. The ingress rule controller will remove the finalizer when the last rule is removed. Once all rules have been removed, the ingressRule list will show as `[]`.

Next, let's create the Subnet, using the file example/walkthrough/resources/subnet.yaml and the NAT Gateway with example/walkthrough/resources/nat-gateway.yaml.

The subnet should be created with no problem, but the NAT Gateway doesn't seem to work- take a look at it's event log.

```
Events:
  Type     Reason         Age                From                   Message
  ----     ------         ----               ----                   -------
  Warning  CreateFailure  4s (x11 over 16s)  natgateway-controller  EIP Allocation not found
```

Well, we don't have an EIP created for the NAT Gateway, even though the spec includes the name of the EIP allocation we'd like to use. Let's go ahead now and just create all of the remaining resources in the directory, including the EIP in question here: `kubectl apply -f example/walkthrough/resources/.`

Fairly shortly, your NAT Gateway's event log should look something like this:

```
Events:
  Type     Reason         Age               From                   Message
  ----     ------         ----              ----                   -------
  Warning  CreateFailure  1m (x15 over 2m)  natgateway-controller  EIP Allocation not found
  Normal   CreateAttempt  5s                natgateway-controller  Creating AWS NATGateway in us-west-2
  Normal   Created        5s                natgateway-controller  Created AWS NATGateway (nat-xxxxxxxxxxxxxxxxx)
  Normal   Annotated      5s                natgateway-controller  Added finalizer and annotations
  Normal   Tagged         4s                natgateway-controller  Added tags
```

Let's go back take another look at your EC2Security Group's annotations:

```
Annotations:  assignedToInstances=["i-xxxxxxxxxxxxxxxxx"]
              ec2SecurityGroupId=sg-xxxxxxxxxxxxxxxxx
              ingressRules=["ingressrule-walkthrough-ssh","ingressrule-walkthrough-http"]
              kubectl.kubernetes.io/last-applied-configuration={"apiVersion":"ecc.aws.gotopple.com/v1alpha1","kind":"EC2SecurityGroup","metadata":{"annotations":{},"labels":{"controller-tools.k8s.io":"1.0"},"name":...
```

You can see here that the EC2Instance(s) the security group is assigned to is also listed. The finalizers show what controllers have been working with the security group and need to perform some work before the security group can be safely deleted.

```
Finalizers:
    ec2securitygroups.ecc.aws.gotopple.com
    authorizeec2securitygroupingress.ecc.aws.gotopple.com
    ec2instances.ecc.aws.gotopple.com
```

In order to view the Tomcat webserver placeholder, we'll need to know the public IP address of the EC2 instance. At the moment, the ECInstance doesn't have the IP address listed in the annotations, so we'll need to get the information from the EIP resource.

`kubectl get eip --namespace kloudformation`

Should return:

```
NAME                          CREATED AT
eip-ec2instance-walkthrough   13m
eip-natgateway-walkthrough    13m
```

In this case, the names should make it obvious which EIP we are after. Get just the info on the EIP we want, to keep it simple.

`kubectl get eip  eip-ec2instance-walkthrough --namespace kloudformation -o yaml`

In the annotations you should see:

```
annotations:
    eipAllocationId: eipalloc-xxxxxxxxxxxxxxxxx
    eipPublicIp: xxx.xxx.xxx.xxx
```

Note the port that you need to access the server, 8888. You can confirm this by looking at the ingress rules and confirming that the EC2 Instance user data shows the server running on the same port.

`kubectl get authorizeec2securitygroupingress ingressrule-walkthrough-http --namespace kloudformation -o yaml`

Check the user data field in the object spec:

`kubectl describe ec2instance --namespace kloudformation`

Put the IP/port into your browser and give it a shot!
