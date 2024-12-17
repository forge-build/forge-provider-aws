/*
Copyright 2024 The Forge contributors.

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

package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	awserrors "github.com/forge-build/forge-provider-aws/pkg/cloud/services/errors"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type AWSClient struct {
	EC2 *ec2.EC2
}

var _ Interface = &AWSClient{}

// NewAWSServices initializes AWS SDK clients based on the provided region.
func NewAWSClient(ctx context.Context, region string, credentialsRef *corev1.SecretReference, crClient client.Client) (AWSClient, error) {

	accessKey, secretKey, err := getAWSCredentialsFromSecret(ctx, credentialsRef, crClient)
	if err != nil {
		return AWSClient{}, err
	}
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
	})
	if err != nil {
		return AWSClient{}, err
	}

	return AWSClient{
		EC2: ec2.New(sess),
	}, nil
}

func (s *AWSClient) getVPCCIDR(_ context.Context, vpcID string) (string, error) {
	// Use DescribeVpcs to fetch details of the VPC by ID
	output, err := s.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{aws.String(vpcID)},
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to describe VPC with ID %s", vpcID)
	}

	if len(output.Vpcs) == 0 {
		return "", errors.Errorf("no VPC found with ID %s", vpcID)
	}

	// Extract the primary CIDR block
	vpc := output.Vpcs[0]
	if vpc.CidrBlock == nil {
		return "", errors.Errorf("VPC %s has no CIDR block", vpcID)
	}

	return *vpc.CidrBlock, nil
}

func (s *AWSClient) FindSubnetByID(_ context.Context, subnetID string) (*ec2.Subnet, error) {
	output, err := s.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
		SubnetIds: []*string{aws.String(subnetID)},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to describe subnet by ID")
	}
	if len(output.Subnets) == 0 {
		return nil, errors.New("subnet not found")
	}
	return output.Subnets[0], nil
}

// isManagedSubnet checks if the subnet is tagged as managed by forge.
func (s *AWSClient) IsManagedSubnet(ctx context.Context, subnetID string) (bool, error) {
	log := log.FromContext(ctx)
	log.Info("Checking if subnet is managed by forge", "SubnetID", subnetID)

	// Describe the subnet by ID
	output, err := s.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
		SubnetIds: []*string{aws.String(subnetID)},
	})
	if err != nil {
		if awserrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to describe subnet")
	}

	// Check for the forge-managed tag
	subnet := output.Subnets[0]
	for _, tag := range subnet.Tags {
		if aws.StringValue(tag.Key) == "forge-managed" && aws.StringValue(tag.Value) == "true" {
			return true, nil
		}
	}

	// Subnet is not managed
	return false, nil
}

func (s *AWSClient) CreateSubnet(ctx context.Context, vpcName string, vpcID *string) (*ec2.Subnet, error) {
	// Retrieve the VPC CIDR dynamically
	if vpcID == nil {
		return nil, errors.New("VPC ID is not set in scope")
	}

	vpcCIDR, err := s.getVPCCIDR(ctx, *vpcID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve VPC CIDR")
	}
	// Retrieve existing subnets in the VPC
	output, err := s.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{Name: aws.String("vpc-id"), Values: []*string{vpcID}},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to describe subnets")
	}

	// Collect used CIDRs
	var usedCIDRs []string
	for _, subnet := range output.Subnets {
		usedCIDRs = append(usedCIDRs, aws.StringValue(subnet.CidrBlock))
	}

	// Find an available CIDR
	subnetMask := 24 // Example: Create /24 subnets
	cidrBlock, err := findAvailableCIDR(vpcCIDR, usedCIDRs, subnetMask)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find available CIDR block")
	}

	// Create the subnet
	log := log.FromContext(ctx)
	log.Info("Creating subnet", "CIDRBlock", cidrBlock)

	createOutput, err := s.EC2.CreateSubnet(&ec2.CreateSubnetInput{
		VpcId:     vpcID,
		CidrBlock: aws.String(cidrBlock),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSubnet),
				Tags: []*ec2.Tag{
					{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("%s-subnet", vpcName))},
					{Key: aws.String("forge-managed"), Value: aws.String("true")},
				},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create subnet")
	}

	return createOutput.Subnet, nil
}

func (s *AWSClient) DeleteSubnet(ctx context.Context, subnetID *string) error {
	_, err := s.EC2.DeleteSubnet(&ec2.DeleteSubnetInput{
		SubnetId: subnetID,
	})
	if err != nil {
		return errors.Wrap(err, "failed to delete subnet")
	}
	return nil
}

// createSecurityGroup creates a new Security Group in the specified VPC.
func (s *AWSClient) CreateSecurityGroup(vpcID, sgName *string) (*ec2.CreateSecurityGroupOutput, error) {
	output, err := s.EC2.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   sgName,
		Description: aws.String("Security Group managed by Forge"),
		VpcId:       vpcID,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSecurityGroup),
				Tags: []*ec2.Tag{
					{Key: aws.String("Name"), Value: sgName},
					{Key: aws.String("forge-managed"), Value: aws.String("true")},
				},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Security Group")
	}
	return output, nil
}

// authorizeSecurityGroupIngress adds an SSH ingress rule to the specified Security Group.
func (s *AWSClient) AuthorizeSecurityGroupIngress(sgID string) error {
	_, err := s.EC2.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(sgID),
		IpPermissions: []*ec2.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int64(22),
				ToPort:     aws.Int64(22),
				IpRanges: []*ec2.IpRange{
					{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("Allow SSH from anywhere")},
				},
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to add ingress rule to Security Group")
	}
	return nil
}

// isManagedSecurityGroup checks if the Security Group is managed by Forge.
func (s *AWSClient) IsManagedSecurityGroup(sgID string) (bool, error) {
	// Describe the Security Group to check its tags
	output, err := s.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{aws.String(sgID)},
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to describe Security Group")
	}

	// Check the tags for the "forge-managed" key
	if len(output.SecurityGroups) > 0 {
		for _, tag := range output.SecurityGroups[0].Tags {
			if aws.StringValue(tag.Key) == "forge-managed" && aws.StringValue(tag.Value) == "true" {
				return true, nil
			}
		}
	}
	return false, nil
}

// DeleteSecurityGroup delete the Security Group
func (s *AWSClient) DeleteSecurityGroup(sgID *string) error {
	_, err := s.EC2.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
		GroupId: sgID,
	})
	if err != nil {
		return errors.Wrap(err, "failed to delete Security Group")
	}
	return nil
}

func (s *AWSClient) configureRouteTable(vpcID, igwID string) error {
	// Find the main route table for the VPC
	output, err := s.EC2.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
			{
				Name:   aws.String("association.main"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to describe route tables")
	}

	if len(output.RouteTables) == 0 {
		return errors.New("no main route table found for VPC")
	}

	routeTableID := aws.StringValue(output.RouteTables[0].RouteTableId)

	// Add a route to the Internet Gateway
	_, err = s.EC2.CreateRoute(&ec2.CreateRouteInput{
		RouteTableId:         aws.String(routeTableID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(igwID),
	})
	if err != nil {
		return errors.Wrap(err, "failed to add route to Internet Gateway")
	}

	return nil
}

func (s *AWSClient) IsManagedVPC(vpcID *string) (bool, error) {
	// Describe the VPC to get its tags
	output, err := s.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{vpcID},
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to describe VPC")
	}

	// Check the tags for the "forge-managed" key
	for _, vpc := range output.Vpcs {
		for _, tag := range vpc.Tags {
			if aws.StringValue(tag.Key) == "forge-managed" && aws.StringValue(tag.Value) == "true" {
				return true, nil
			}
		}
	}
	return false, nil
}

// DetachAndDeleteInternetGateway detaches and deletes the Internet Gateway attached to the VPC.
func (s *AWSClient) DetachAndDeleteInternetGateway(vpcID *string) error {
	// Describe Internet Gateways attached to the VPC
	output, err := s.EC2.DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("attachment.vpc-id"),
				Values: []*string{vpcID},
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to describe Internet Gateway")
	}

	for _, igw := range output.InternetGateways {
		igwID := aws.StringValue(igw.InternetGatewayId)

		// Detach the IGW from the VPC
		_, err := s.EC2.DetachInternetGateway(&ec2.DetachInternetGatewayInput{
			InternetGatewayId: igw.InternetGatewayId,
			VpcId:             vpcID,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to detach Internet Gateway %s from VPC %s", igwID, *vpcID)
		}

		// Delete the IGW
		_, err = s.EC2.DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{
			InternetGatewayId: igw.InternetGatewayId,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete Internet Gateway %s", igwID)
		}
	}

	return nil
}

func (s *AWSClient) findInternetGateway(vpcID string) (*ec2.InternetGateway, error) {
	// Describe Internet Gateways attached to the VPC
	output, err := s.EC2.DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("attachment.vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to describe Internet Gateways")
	}

	if len(output.InternetGateways) > 0 {
		return output.InternetGateways[0], nil
	}

	return nil, nil
}

func (s *AWSClient) FindVPCByIDOrName(vpcID, vpcName *string) (*ec2.Vpc, error) {
	// Check if the VPC exists by ID
	if vpcID != nil {
		output, err := s.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{
			VpcIds: []*string{vpcID},
		})
		if err == nil && len(output.Vpcs) > 0 {
			return output.Vpcs[0], nil
		}
		if err != nil {
		}
	}

	// Check if the VPC exists by Name
	if vpcName != nil {
		output, err := s.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("tag:Name"),
					Values: []*string{vpcName},
				},
			},
		})
		if err == nil && len(output.Vpcs) > 0 {
			return output.Vpcs[0], nil
		}
		if err != nil {
			return nil, errors.Wrap(err, "Failed to find VPC by Name")
		}
	}

	// If neither ID nor Name matches, return nil
	return nil, nil
}

func (s *AWSClient) DeleteVPC(vpcID *string) error {
	_, err := s.EC2.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: vpcID,
	})
	if err != nil {
		return errors.Wrap(err, "failed to delete VPC")
	}
	return nil
}

func (s *AWSClient) CreateVPC(input *ec2.CreateVpcInput) (*ec2.Vpc, error) {
	createOutput, err := s.EC2.CreateVpc(input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create VPC")
	}
	return createOutput.Vpc, nil
}

// createOrGetInternetGateway creates an Internet Gateway if it doesn't exist and configures the route table.
func (s *AWSClient) CreateOrGetInternetGateway(ctx context.Context, vpcID string) (*ec2.InternetGateway, error) {
	// Check if an Internet Gateway already exists for the VPC
	igw, err := s.findInternetGateway(vpcID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find Internet Gateway")
	}
	if igw != nil {
		err := s.configureRouteTable(vpcID, aws.StringValue(igw.InternetGatewayId))
		if err != nil {
			return nil, errors.Wrap(err, "failed to configure route table for Internet Gateway")
		}
		return igw, nil
	}

	// Create a new Internet Gateway
	createOutput, err := s.EC2.CreateInternetGateway(&ec2.CreateInternetGatewayInput{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Internet Gateway")
	}

	igwID := aws.StringValue(createOutput.InternetGateway.InternetGatewayId)

	// Attach the Internet Gateway to the VPC
	_, err = s.EC2.AttachInternetGateway(&ec2.AttachInternetGatewayInput{
		InternetGatewayId: createOutput.InternetGateway.InternetGatewayId,
		VpcId:             aws.String(vpcID),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to attach Internet Gateway to VPC")
	}

	// Configure the Route Table
	err = s.configureRouteTable(vpcID, igwID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure route table for Internet Gateway")
	}

	return createOutput.InternetGateway, nil
}

func (s *AWSClient) IsManagedInstance(instanceID *string) (bool, error) {
	output, err := s.EC2.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{instanceID},
	})
	if err != nil {
		if awserrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if len(output.Reservations) == 0 || len(output.Reservations[0].Instances) == 0 {
		return false, nil
	}

	instance := output.Reservations[0].Instances[0]
	for _, tag := range instance.Tags {
		if aws.StringValue(tag.Key) == "forge-managed" && aws.StringValue(tag.Value) == "true" {
			return true, nil
		}
	}

	return false, nil
}

func (s *AWSClient) FindInstanceByID(instanceID *string) (*ec2.Instance, error) {
	output, err := s.EC2.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{instanceID},
	})
	if err != nil {
		if awserrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, res := range output.Reservations {
		for _, inst := range res.Instances {
			if *inst.InstanceId == *instanceID {
				return inst, nil
			}
		}
	}

	return nil, nil
}

func (s *AWSClient) CreateInstance(input CreateInstanceParams) (*ec2.Instance, error) {
	// Check parmars
	if input.AmiID == "" {
		return nil, errors.New("AMI ID not provided")
	}

	if input.InstanceType == "" {
		return nil, errors.New("Instance type not provided")
	}

	// Network configuration to assign public IP
	networkInterface := &ec2.InstanceNetworkInterfaceSpecification{
		DeviceIndex:              aws.Int64(0), // Primary network interface
		AssociatePublicIpAddress: &input.PublicIP,
	}

	if input.SubnetID != "" {
		networkInterface.SubnetId = &input.SubnetID
	}

	if input.SecurityGroupID != "" {
		networkInterface.Groups = aws.StringSlice([]string{input.SecurityGroupID})
	}

	// Build tags
	tags := []*ec2.Tag{
		{Key: aws.String("Name"), Value: &input.Name},
		{Key: aws.String("forge-managed"), Value: aws.String("true")},
	}

	// RunInstances input
	runInput := &ec2.RunInstancesInput{
		ImageId:      &input.AmiID,
		InstanceType: &input.InstanceType,
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		UserData:     &input.Userdata,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeInstance),
				Tags:         tags,
			},
		},
		NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{networkInterface},
	}

	// Block devices, networking config can be added here as needed.

	runOutput, err := s.EC2.RunInstances(runInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run EC2 instance")
	}

	if len(runOutput.Instances) == 0 {
		return nil, errors.New("no instances launched")
	}

	return runOutput.Instances[0], nil
}

func (s *AWSClient) TerminateInstance(instanceID *string) error {
	_, err := s.EC2.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: []*string{instanceID},
	})
	if err != nil {
		return errors.Wrap(err, "failed to terminate EC2 instance")
	}
	return nil
}

// CheckAMIStatus checks if an AMI exists by name and returns its ID and state.
func (s *AWSClient) CheckAMIStatus(ctx context.Context, imageName string) (string, string, error) {
	output, err := s.EC2.DescribeImagesWithContext(ctx, &ec2.DescribeImagesInput{
		Owners: aws.StringSlice([]string{"self"}),
		Filters: []*ec2.Filter{
			{Name: aws.String("name"), Values: aws.StringSlice([]string{imageName})},
		},
	})
	if err != nil {
		return "", "", errors.Wrap(err, "failed to describe AMI status")
	}

	if len(output.Images) > 0 {
		ami := output.Images[0]
		return *ami.ImageId, aws.StringValue(ami.State), nil
	}

	return "", "", nil
}

// CreateAMI creates a new AMI from the instance's root volume.
func (s *AWSClient) CreateAMI(ctx context.Context, instanceID, imageName string) error {
	input := &ec2.CreateImageInput{
		InstanceId:  aws.String(instanceID),
		Name:        aws.String(imageName),
		NoReboot:    aws.Bool(true), // Avoid rebooting the instance
		Description: aws.String(fmt.Sprintf("AMI created from instance %s", instanceID)),
	}

	_, err := s.EC2.CreateImageWithContext(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to create AMI")
	}

	return nil
}

// ListAMIs with the given name
func (s *AWSClient) ListAMIs(ctx context.Context, imageName string) ([]*ec2.Image, error) {
	output, err := s.EC2.DescribeImagesWithContext(ctx, &ec2.DescribeImagesInput{
		Owners: aws.StringSlice([]string{"self"}), // Owned AMIs
		Filters: []*ec2.Filter{
			{Name: aws.String("name"), Values: []*string{&imageName}},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to describe AMIs")
	}
	return output.Images, nil
}

// EnsureAMIDoesNotExist checks if an AMI exists and deletes it if its creation date is older than the Build's creation date.
func (s *AWSClient) EnsureAMIDoesNotExist(ctx context.Context, imageName, creationDate string) error {
	Images, err := s.ListAMIs(ctx, imageName)
	if err != nil {
		return err
	}
	buildCreationTime, err := time.Parse(time.RFC3339, creationDate)
	if err != nil {
		return err
	}
	// Loop through matching AMIs
	for _, image := range Images {
		amiID := *image.ImageId
		creationDateStr := *image.CreationDate

		// Parse AMI creation date
		amiCreationDate, err := time.Parse(time.RFC3339, creationDateStr)
		if err != nil {
			continue
		}

		// Compare AMI creation date with Build's creation date
		if amiCreationDate.Before(buildCreationTime) {
			_, err := s.EC2.DeregisterImageWithContext(ctx, &ec2.DeregisterImageInput{
				ImageId: image.ImageId,
			})
			if err != nil {
				return errors.Wrapf(err, "failed to deregister outdated AMI %s", amiID)
			}
		}
	}

	return nil
}
