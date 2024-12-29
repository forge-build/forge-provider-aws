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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

// Reconcile ensures the AWS Subnet is present.
func (s *Service) Reconcile(ctx context.Context) error {
	s.Log.V(1).Info("Reconciling AWS Subnet resources")

	// Check if SubnetID is specified
	subnetID := s.scope.SubnetID()
	if subnetID != nil {
		s.Log.V(1).Info("Getting subnet using SubnetID", "SubnetID", subnetID)
		subnet, err := s.Client.FindSubnetByID(ctx, *subnetID)
		if err != nil {
			return errors.Wrap(err, "failed to find user-specified subnet")
		}
		s.scope.SetSubnet(subnet.SubnetId)
		return nil
	}

	// Create a new subnet
	s.Log.Info("No existing subnet found, creating a new subnet")
	newSubnet, err := s.Client.CreateSubnet(ctx, *s.scope.VPCName(), s.scope.VPCID())
	if err != nil {
		return errors.Wrap(err, "failed to create subnet")
	}
	s.scope.SetSubnet(newSubnet.SubnetId)

	s.Log.Info("Successfully reconciled subnet", "SubnetID", aws.StringValue(newSubnet.SubnetId))
	return nil
}

// Delete ensures the AWS Subnet is deleted if managed by the system.
func (s *Service) Delete(ctx context.Context) error {
	s.Log.V(1).Info("Deleting AWS Subnet resources")

	// Check if SubnetID is set in the scope
	subnetID := s.scope.SubnetID()
	if subnetID == nil {
		s.Log.Info("No SubnetID provided, skipping deletion")
		return nil
	}

	// Check if the subnet is managed by the controller
	isManaged, err := s.Client.IsManagedSubnet(ctx, *subnetID)
	if err != nil {
		return errors.Wrap(err, "failed to check if subnet is managed")
	}
	if !isManaged {
		s.Log.Info("Subnet is not managed by forge, skipping deletion", "SubnetID", subnetID)
		return nil
	}

	// Delete the subnet
	s.Log.Info("Deleting managed subnet", "SubnetID", subnetID)
	err = s.Client.DeleteSubnet(ctx, subnetID)

	if err != nil {

		return err
	}

	s.Log.Info("Successfully deleted subnet", "SubnetID", subnetID)
	return nil
}
