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

	"github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
	awserrors "github.com/forge-build/forge-provider-aws/pkg/cloud/services/errors"
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

	log.Info("Creating Security Group", "VPCID", vpcID)
	sg, err := s.Client.CreateSecurityGroup(vpcID, s.scope.SecurityGroupName())
	if err != nil {
		return errors.Wrap(err, "failed to create Security Group")
	}

	// Add SSH ingress rule
	log.Info("Adding SSH ingress rule to Security Group", "SecurityGroupID", sgID)
	err = s.Client.AuthorizeSecurityGroupIngress(*sg.GroupId)
	if err != nil {
		return errors.Wrap(err, "failed to add SSH ingress rule to Security Group")
	}

	// Update the scope with the created Security Group ID
	s.scope.SetSecurityGroupID(sg.GroupId)
	log.Info("Successfully reconciled Security Group", "SecurityGroupID", *sg.GroupId)
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
	log.Info("Checking if Security Group is managed by Forge", "SecurityGroupID", sgID)
	isManaged, err := s.Client.IsManagedSecurityGroup(*sgID)
	if err != nil {
		return errors.Wrap(err, "failed to check if Security Group is managed")
	}

	if !isManaged {
		log.Info("Security Group is not managed by Forge, skipping deletion", "SecurityGroupID", *sgID)
		return nil
	}

	// Delete the Security Group
	log.Info("Deleting Security Group", "SecurityGroupID", *sgID)
	err = s.Client.DeleteSecurityGroup(sgID)
	if err != nil {
		return err
	}

	log.Info("Successfully deleted Security Group", "SecurityGroupID", *sgID)
	return nil
}
