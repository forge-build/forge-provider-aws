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

package subnet

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	awserrors "github.com/forge-build/forge-provider-aws/pkg/cloud/aws/errors"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconcile ensures the AWS Subnet is present.
func (s *Service) Reconcile(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Reconciling AWS Subnet resources")

	// Step 1: Check if SubnetID is specified
	subnetID := s.scope.SubnetID()
	if subnetID != nil {
		log.Info("Using user-specified SubnetID", "SubnetID", subnetID)
		subnet, err := s.findSubnetByID(ctx, *subnetID)
		if err != nil {
			return errors.Wrap(err, "failed to find user-specified subnet")
		}
		s.scope.SetSubnet(subnet.SubnetId)
		return nil
	}

	// Step 3: Create a new subnet
	log.Info("No existing subnet found, creating a new subnet")
	newSubnet, err := s.createSubnet(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create subnet")
	}
	s.scope.SetSubnet(newSubnet.SubnetId)

	log.Info("Successfully reconciled subnet", "SubnetID", aws.StringValue(newSubnet.SubnetId))
	return nil
}

// Delete ensures the AWS Subnet is deleted if managed by the system.
func (s *Service) Delete(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Deleting AWS Subnet resources")

	// Step 1: Check if SubnetID is set in the scope
	subnetID := s.scope.SubnetID()
	if subnetID == nil {
		log.Info("No SubnetID provided, skipping deletion")
		return nil
	}

	// Step 2: Check if the subnet is managed by the controller
	isManaged, err := s.isManagedSubnet(ctx, *subnetID)
	if err != nil {
		return errors.Wrap(err, "failed to check if subnet is managed")
	}
	if !isManaged {
		log.Info("Subnet is not managed by forge, skipping deletion", "SubnetID", subnetID)
		return nil
	}

	// Step 3: Delete the subnet
	log.Info("Deleting managed subnet", "SubnetID", subnetID)
	_, err = s.subnetClient.DeleteSubnet(&ec2.DeleteSubnetInput{
		SubnetId: subnetID,
	})
	if err != nil {
		return errors.Wrap(err, "failed to delete subnet")
	}

	log.Info("Successfully deleted subnet", "SubnetID", subnetID)
	return nil
}

// isManagedSubnet checks if the subnet is tagged as managed by forge.
func (s *Service) isManagedSubnet(ctx context.Context, subnetID string) (bool, error) {
	log := log.FromContext(ctx)
	log.Info("Checking if subnet is managed by forge", "SubnetID", subnetID)

	// Describe the subnet by ID
	output, err := s.subnetClient.DescribeSubnets(&ec2.DescribeSubnetsInput{
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

func (s *Service) createSubnet(ctx context.Context) (*ec2.Subnet, error) {
	// Retrieve the VPC CIDR dynamically
	vpcID := s.scope.VPCID()
	if vpcID == nil {
		return nil, errors.New("VPC ID is not set in scope")
	}

	vpcCIDR, err := s.getVPCCIDR(ctx, *vpcID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve VPC CIDR")
	}
	// Retrieve existing subnets in the VPC
	output, err := s.subnetClient.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{Name: aws.String("vpc-id"), Values: []*string{s.scope.VPCID()}},
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

	createOutput, err := s.subnetClient.CreateSubnet(&ec2.CreateSubnetInput{
		VpcId:     s.scope.VPCID(),
		CidrBlock: aws.String(cidrBlock),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSubnet),
				Tags: []*ec2.Tag{
					{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("%s-subnet", *s.scope.VPCName()))},
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

func (s *Service) findSubnetByID(_ context.Context, subnetID string) (*ec2.Subnet, error) {
	output, err := s.subnetClient.DescribeSubnets(&ec2.DescribeSubnetsInput{
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

func (s *Service) getVPCCIDR(_ context.Context, vpcID string) (string, error) {
	// Use DescribeVpcs to fetch details of the VPC by ID
	output, err := s.vpcClient.DescribeVpcs(&ec2.DescribeVpcsInput{
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
