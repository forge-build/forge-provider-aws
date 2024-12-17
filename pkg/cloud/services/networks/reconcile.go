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
	awsforge "github.com/forge-build/forge-provider-aws/pkg/aws"

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

	// Ensure Internet Gateway exists
	vpcID := *vpc.VpcId
	log.Info("Reconciling Internet Gateway for VPC", "VPCID", vpcID)
	igw, err := s.Client.CreateOrGetInternetGateway(ctx, vpcID)
	if err != nil {
		return errors.Wrap(err, "failed to reconcile Internet Gateway")
	}
	log.Info("Internet Gateway is ready", "IGWID", aws.StringValue(igw.InternetGatewayId))

	return nil
}

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
	isManagedVPC, err := s.Client.IsManagedVPC(vpcID)
	if err != nil {
		return errors.Wrap(err, "failed to check if VPC is managed")
	}

	if !isManagedVPC {
		log.Info("VPC is not managed by the system. Skipping deletion.", "VPCID", *vpcID)
		return nil
	}

	// Detach and delete the Internet Gateway
	err = s.Client.DetachAndDeleteInternetGateway(vpcID)
	if err != nil {
		return errors.Wrap(err, "failed to detach and delete Internet Gateway")
	}

	// Delete the VPC
	log.Info("Deleting VPC", "VPCID", *vpcID)
	err = s.Client.DeleteVPC(vpcID)
	if err != nil {
		return err
	}

	log.Info("Successfully deleted VPC", "VPCID", *vpcID)
	return nil
}

// createOrGetVPC creates a VPC if it doesn't exist.
func (s *Service) createOrGetVPC(ctx context.Context) (*ec2.Vpc, error) {
	// Try to find the VPC by ID or Name
	vpcID := s.scope.VPCID()
	vpcName := s.scope.VPCName()
	vpc, err := s.Client.FindVPCByIDOrName(vpcID, vpcName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to search for VPC")
	}
	if vpc != nil {
		// Update the spec with the found VPC details
		s.scope.SetVPCID(vpc.VpcId)
		name := awsforge.GetNameFromTags(vpc.Tags)
		s.scope.SetVPCName(name)
		return vpc, nil
	}

	// No existing VPC found; create a new one
	vpcSpec := s.scope.VPCSpec()
	vpc, err = s.Client.CreateVPC(vpcSpec)
	if err != nil {
		return nil, err
	}

	// Update the spec with the found VPC details
	s.scope.SetVPCID(vpc.VpcId)
	s.scope.SetVPCName(*vpcName)

	return vpc, nil
}
