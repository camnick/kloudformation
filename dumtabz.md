`func (*EC2) CreateRoute`
`func (c *EC2) CreateRoute(input *CreateRouteInput) (*CreateRouteOutput, error)``
CreateRoute API operation for Amazon Elastic Compute Cloud.

Creates a route in a route table within a VPC.

You must specify one of the following targets: internet gateway or virtual private gateway, NAT instance, NAT gateway, VPC peering connection, network interface, or egress-only internet gateway.

When determining how to route traffic, we use the route with the most specific match. For example, traffic is destined for the IPv4 address 192.0.2.3, and the route table includes the following two IPv4 routes:

* 192.0.2.0/24 (goes to some target A)

* 192.0.2.0/28 (goes to some target B)
Both routes apply to the traffic destined for 192.0.2.3. However, the second route in the list covers a smaller number of IP addresses and is therefore more specific, so we use that route to determine where to target the traffic.

For more information about route tables, see Route Tables (http://docs.aws.amazon.com/AmazonVPC/latest/UserGuide/VPC_Route_Tables.html) in the Amazon Virtual Private Cloud User Guide.

Returns awserr.Error for service API and SDK errors. Use runtime type assertions with awserr.Error's Code and Message methods to get detailed information about the error.

See the AWS API reference guide for Amazon Elastic Compute Cloud's API operation CreateRoute for usage and error information. See also, https://docs.aws.amazon.com/goto/WebAPI/ec2-2016-11-15/CreateRoute

▾ Example (Shared00)

To create a route This example creates a route for the specified route table. The route matches all traffic (0.0.0.0/0) and routes it to the specified Internet gateway.

Code:
```
svc := ec2.New(session.New())
input := &ec2.CreateRouteInput{
    DestinationCidrBlock: aws.String("0.0.0.0/0"),
    GatewayId:            aws.String("igw-c0a643a9"),
    RouteTableId:         aws.String("rtb-22574640"),
}

result, err := svc.CreateRoute(input)
if err != nil {
    if aerr, ok := err.(awserr.Error); ok {
        switch aerr.Code() {
        default:
            fmt.Println(aerr.Error())
        }
    } else {
        // Print the error, cast err to awserr.Error to get the Code and
        // Message from an error.
        fmt.Println(err.Error())
    }
    return
}

fmt.Println(result)
```
