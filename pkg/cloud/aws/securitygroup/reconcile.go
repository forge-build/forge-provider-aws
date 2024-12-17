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

package securitygroup

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
	awserrors "github.com/forge-build/forge-provider-aws/pkg/cloud/aws/errors"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconcile ensures the Security Group is present and correctly configured.
func (s *Service) Reconcile(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Reconciling Security Group resources")

	// Check if the Security Group ID is defined by the user
	sgID := s.scope.SecurityGroupID()
	if sgID != nil {
		log.Info("Using existing Security Group", "SecurityGroupID", *sgID)
		return nil
	}

	// Create a new Security Group if not specified
	vpcID := s.scope.VPCID()
	if vpcID == nil {
		return errors.New("VPC ID is required to create a Security Group")
	}

	log.Info("Creating a new Security Group", "VPCID", *vpcID)
	sg, err := s.createSecurityGroup(ctx, *vpcID)
	if err != nil {
		return errors.Wrap(err, "failed to create Security Group")
	}

	// Add SSH ingress rule
	err = s.authorizeSecurityGroupIngress(ctx, *sg.GroupId)
	if err != nil {
		return errors.Wrap(err, "failed to add SSH ingress rule to Security Group")
	}

	// Update the scope with the created Security Group ID
	s.scope.SetSecurityGroupID(sg.GroupId)
	log.Info("Successfully reconciled Security Group", "SecurityGroupID", *sg.GroupId)
	return nil
}

// createSecurityGroup creates a new Security Group in the specified VPC.
func (s *Service) createSecurityGroup(ctx context.Context, vpcID string) (*ec2.CreateSecurityGroupOutput, error) {
	log := log.FromContext(ctx)
	sgName := s.scope.SecurityGroupName()

	log.Info("Creating Security Group", "Name", sgName, "VPCID", vpcID)
	output, err := s.sgClient.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   sgName,
		Description: aws.String("Security Group managed by Forge"),
		VpcId:       aws.String(vpcID),
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
func (s *Service) authorizeSecurityGroupIngress(ctx context.Context, sgID string) error {
	log := log.FromContext(ctx)
	log.Info("Adding SSH ingress rule to Security Group", "SecurityGroupID", sgID)

	_, err := s.sgClient.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
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

// Delete ensures the Security Group is deleted if managed by the system.
func (s *Service) Delete(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Deleting Security Group resources")

	if *s.scope.InstanceState() != v1alpha1.InstanceStatusTerminated {
		return awserrors.ErrInstanceNotTerminated
	}

	sgID := s.scope.SecurityGroupID()
	if sgID == nil {
		log.Info("No Security Group to delete")
		return nil
	}

	// Check if the Security Group is managed by the controller
	isManaged, err := s.isManagedSecurityGroup(ctx, *sgID)
	if err != nil {
		return errors.Wrap(err, "failed to check if Security Group is managed")
	}

	if !isManaged {
		log.Info("Security Group is not managed by Forge, skipping deletion", "SecurityGroupID", *sgID)
		return nil
	}

	// Delete the Security Group
	log.Info("Deleting Security Group", "SecurityGroupID", *sgID)
	_, err = s.sgClient.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(*sgID),
	})
	if err != nil {
		return errors.Wrap(err, "failed to delete Security Group")
	}

	log.Info("Successfully deleted Security Group", "SecurityGroupID", *sgID)
	return nil
}

// isManagedSecurityGroup checks if the Security Group is managed by Forge.
func (s *Service) isManagedSecurityGroup(ctx context.Context, sgID string) (bool, error) {
	log := log.FromContext(ctx)
	log.Info("Checking if Security Group is managed by Forge", "SecurityGroupID", sgID)

	// Describe the Security Group to check its tags
	output, err := s.sgClient.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{aws.String(sgID)},
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to describe Security Group")
	}

	// Check the tags for the "forge-managed" key
	if len(output.SecurityGroups) > 0 {
		for _, tag := range output.SecurityGroups[0].Tags {
			if aws.StringValue(tag.Key) == "forge-managed" && aws.StringValue(tag.Value) == "true" {
				log.Info("Security Group is managed by Forge", "SecurityGroupID", sgID)
				return true, nil
			}
		}
	}

	log.Info("Security Group is not managed by Forge", "SecurityGroupID", sgID)
	return false, nil
}
