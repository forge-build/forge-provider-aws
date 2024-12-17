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

package networks

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconcile ensures the AWS VPC and related resources are present.
func (s *Service) Reconcile(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Reconciling AWS VPC resources")

	// Ensure VPC exists
	vpc, err := s.createOrGetVPC(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to reconcile VPC")
	}
	log.Info("VPC is ready", "VPCID", *vpc.VpcId)

	// Ensure Internet Gateway exists
	igw, err := s.createOrGetInternetGateway(ctx, aws.StringValue(vpc.VpcId))
	if err != nil {
		return errors.Wrap(err, "failed to reconcile Internet Gateway")
	}
	log.Info("Internet Gateway is ready", "IGWID", aws.StringValue(igw.InternetGatewayId))

	return nil
}

// Delete ensures the AWS VPC and related resources are deleted.
// Delete ensures the AWS VPC and related resources are deleted.
func (s *Service) Delete(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Deleting AWS VPC resources")

	vpcID := s.scope.VPCID()
	if vpcID == nil {
		log.Info("No VPC to delete")
		return nil
	}

	// Check if the VPC is managed
	isManagedVPC, err := s.isManagedVPC(ctx, *vpcID)
	if err != nil {
		return errors.Wrap(err, "failed to check if VPC is managed")
	}

	if !isManagedVPC {
		log.Info("VPC is not managed by the system. Skipping deletion.", "VPCID", *vpcID)
		return nil
	}

	// Step 1: Detach and delete the Internet Gateway
	err = s.detachAndDeleteInternetGateway(ctx, *vpcID)
	if err != nil {
		return errors.Wrap(err, "failed to detach and delete Internet Gateway")
	}

	// Step 2: Delete the VPC
	log.Info("Deleting VPC", "VPCID", *vpcID)
	_, err = s.ec2Client.DeleteVpc(&ec2.DeleteVpcInput{
		VpcId: aws.String(*vpcID),
	})
	if err != nil {
		return errors.Wrap(err, "failed to delete VPC")
	}

	log.Info("Successfully deleted VPC", "VPCID", *vpcID)
	return nil
}

// createOrGetVPC creates a VPC if it doesn't exist.
func (s *Service) createOrGetVPC(ctx context.Context) (*ec2.Vpc, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling AWS VPC resources")

	// Try to find the VPC by ID or Name
	vpc, err := s.findVPCByIDOrName(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to search for VPC")
	}
	if vpc != nil {
		// Update the spec with the found VPC details
		s.scope.SetVPCID(vpc.VpcId)
		name := getNameFromTags(vpc.Tags)
		s.scope.SetVPCName(name)
		return vpc, nil
	}

	// No existing VPC found; create a new one
	vpcSpec := s.scope.VPCSpec()
	if vpcSpec == nil {
		return nil, errors.New("no VPC spec provided, and no existing VPC found")
	}

	log.Info("Creating a new VPC", "CIDRBlock", *vpcSpec.CidrBlock)
	createOutput, err := s.ec2Client.CreateVpc(vpcSpec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create VPC")
	}

	// Update the spec with the found VPC details
	vpcName := s.scope.VPCName()
	s.scope.SetVPCID(createOutput.Vpc.VpcId)
	s.scope.SetVPCName(*vpcName)

	log.Info("Successfully created VPC", "VPCID", *createOutput.Vpc.VpcId, "Name", vpcName)
	return createOutput.Vpc, nil
}

// createOrGetInternetGateway creates an Internet Gateway if it doesn't exist and configures the route table.
func (s *Service) createOrGetInternetGateway(ctx context.Context, vpcID string) (*ec2.InternetGateway, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling Internet Gateway for VPC", "VPCID", vpcID)

	// Step 1: Check if an Internet Gateway already exists for the VPC
	igw, err := s.findInternetGateway(ctx, vpcID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find Internet Gateway")
	}
	if igw != nil {
		log.Info("Found existing Internet Gateway", "IGWID", aws.StringValue(igw.InternetGatewayId))
		err := s.configureRouteTable(ctx, vpcID, aws.StringValue(igw.InternetGatewayId))
		if err != nil {
			return nil, errors.Wrap(err, "failed to configure route table for Internet Gateway")
		}
		return igw, nil
	}

	// Step 2: Create a new Internet Gateway
	log.Info("Creating a new Internet Gateway")
	createOutput, err := s.igwClient.CreateInternetGateway(&ec2.CreateInternetGatewayInput{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Internet Gateway")
	}

	igwID := aws.StringValue(createOutput.InternetGateway.InternetGatewayId)

	// Step 3: Attach the Internet Gateway to the VPC
	log.Info("Attaching Internet Gateway to VPC", "IGWID", igwID, "VPCID", vpcID)
	_, err = s.igwClient.AttachInternetGateway(&ec2.AttachInternetGatewayInput{
		InternetGatewayId: createOutput.InternetGateway.InternetGatewayId,
		VpcId:             aws.String(vpcID),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to attach Internet Gateway to VPC")
	}

	// Step 4: Configure the Route Table
	err = s.configureRouteTable(ctx, vpcID, igwID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure route table for Internet Gateway")
	}

	log.Info("Successfully created and attached Internet Gateway", "IGWID", igwID)
	return createOutput.InternetGateway, nil
}

func (s *Service) findVPCByIDOrName(ctx context.Context) (*ec2.Vpc, error) {
	log := log.FromContext(ctx)

	// Check if the VPC exists by ID
	vpcID := s.scope.VPCID()
	if vpcID != nil {
		log.Info("Searching for VPC by ID", "VPCID", *vpcID)
		output, err := s.ec2Client.DescribeVpcs(&ec2.DescribeVpcsInput{
			VpcIds: []*string{vpcID},
		})
		if err == nil && len(output.Vpcs) > 0 {
			log.Info("Found VPC by ID", "VPCID", *vpcID)
			return output.Vpcs[0], nil
		}
		if err != nil {
			log.Error(err, "Failed to find VPC by ID, proceeding to search by Name")
		}
	}

	// Check if the VPC exists by Name
	vpcName := s.scope.VPCName()
	if vpcName != nil {
		log.Info("Searching for VPC by Name", "Name", vpcName)
		output, err := s.ec2Client.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("tag:Name"),
					Values: []*string{vpcName},
				},
			},
		})
		if err == nil && len(output.Vpcs) > 0 {
			log.Info("Found VPC by Name", "Name", *vpcName)
			return output.Vpcs[0], nil
		}
		if err != nil {
			log.Error(err, "Failed to find VPC by Name")
		}
	}

	// If neither ID nor Name matches, return nil
	return nil, nil
}

func (s *Service) isManagedVPC(ctx context.Context, vpcID string) (bool, error) {
	log := log.FromContext(ctx)

	// Describe the VPC to get its tags
	output, err := s.ec2Client.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{aws.String(vpcID)},
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to describe VPC")
	}

	// Check the tags for the "forge-managed" key
	for _, vpc := range output.Vpcs {
		for _, tag := range vpc.Tags {
			if aws.StringValue(tag.Key) == "forge-managed" && aws.StringValue(tag.Value) == "true" {
				log.Info("VPC is managed by the system", "VPCID", vpcID)
				return true, nil
			}
		}
	}

	log.Info("VPC is not managed by the system", "VPCID", vpcID)
	return false, nil
}

func getNameFromTags(tags []*ec2.Tag) string {
	for _, tag := range tags {
		if aws.StringValue(tag.Key) == "Name" {
			return *tag.Value
		}
	}
	return ""
}

func (s *Service) findInternetGateway(ctx context.Context, vpcID string) (*ec2.InternetGateway, error) {
	log := log.FromContext(ctx)

	// Describe Internet Gateways attached to the VPC
	output, err := s.igwClient.DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
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
		log.Info("Internet Gateway found for VPC", "VPCID", vpcID, "IGWID", *output.InternetGateways[0].InternetGatewayId)
		return output.InternetGateways[0], nil
	}

	log.Info("No Internet Gateway found for VPC", "VPCID", vpcID)
	return nil, nil
}

// detachAndDeleteInternetGateway detaches and deletes the Internet Gateway attached to the VPC.
func (s *Service) detachAndDeleteInternetGateway(ctx context.Context, vpcID string) error {
	log := log.FromContext(ctx)

	// Describe Internet Gateways attached to the VPC
	log.Info("Looking for Internet Gateway attached to the VPC", "VPCID", vpcID)
	output, err := s.igwClient.DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("attachment.vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to describe Internet Gateway")
	}

	for _, igw := range output.InternetGateways {
		igwID := aws.StringValue(igw.InternetGatewayId)

		// Detach the IGW from the VPC
		log.Info("Detaching Internet Gateway from VPC", "IGWID", igwID, "VPCID", vpcID)
		_, err := s.igwClient.DetachInternetGateway(&ec2.DetachInternetGatewayInput{
			InternetGatewayId: igw.InternetGatewayId,
			VpcId:             aws.String(vpcID),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to detach Internet Gateway %s from VPC %s", igwID, vpcID)
		}

		// Delete the IGW
		log.Info("Deleting Internet Gateway", "IGWID", igwID)
		_, err = s.igwClient.DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{
			InternetGatewayId: igw.InternetGatewayId,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete Internet Gateway %s", igwID)
		}
	}

	return nil
}

func (s *Service) configureRouteTable(ctx context.Context, vpcID, igwID string) error {
	log := log.FromContext(ctx)
	log.Info("Configuring Route Table for Internet Gateway", "VPCID", vpcID, "IGWID", igwID)

	// Step 1: Find the main route table for the VPC
	output, err := s.ec2Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
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
	log.Info("Found main route table for VPC", "RouteTableID", routeTableID)

	// Step 2: Add a route to the Internet Gateway
	log.Info("Adding route to Internet Gateway in Route Table", "RouteTableID", routeTableID)
	_, err = s.ec2Client.CreateRoute(&ec2.CreateRouteInput{
		RouteTableId:         aws.String(routeTableID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(igwID),
	})
	if err != nil {
		return errors.Wrap(err, "failed to add route to Internet Gateway")
	}

	log.Info("Successfully configured Route Table for Internet Gateway", "RouteTableID", routeTableID)
	return nil
}
