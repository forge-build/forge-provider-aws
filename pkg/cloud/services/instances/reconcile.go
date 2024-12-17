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

package instances

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	infrav1 "github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
	awsforge "github.com/forge-build/forge-provider-aws/pkg/aws"
	awserrors "github.com/forge-build/forge-provider-aws/pkg/cloud/services/errors"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (s *Service) Reconcile(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Reconciling EC2 instance")

	// createOrGetInstance will handle creation and retrieval logic
	instance, err := s.createOrGetInstance(ctx)
	if err != nil {
		return err
	}
	s.scope.SetInstanceID(instance.InstanceId)

	var publicIP string
	if len(instance.NetworkInterfaces) > 0 && instance.NetworkInterfaces[0].Association.PublicIp != nil {
		publicIP = *instance.NetworkInterfaces[0].Association.PublicIp
	}

	// Ensure SSH credentials secret if applicable
	err = s.scope.EnsureCredentialsSecret(ctx, publicIP)
	if err != nil {
		return err
	}

	// Update scope with instance details
	s.scope.SetInstanceID(instance.InstanceId)
	s.scope.SetInstanceStatus(infrav1.InstanceStatus(strings.ToUpper(*instance.State.Name))) // e.g., "running", "pending", etc.

	log.Info("EC2 instance is ready", "InstanceID", *instance.InstanceId, "PublicIP", publicIP)
	return nil
}

func (s *Service) Delete(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Deleting EC2 instance")

	instanceID := s.scope.GetInstanceID()
	if instanceID == nil {
		log.Info("No instance ID to delete, skipping")
		return nil
	}

	// Check if the instance is managed
	isManaged, err := s.Client.IsManagedInstance(instanceID)
	if err != nil {
		return errors.Wrap(err, "failed to check if instance is managed")
	}

	if !isManaged {
		log.Info("Instance is not managed by forge, skipping deletion", "InstanceID", *instanceID)
		return nil
	}

	// Check current state of the instance
	instance, err := s.Client.FindInstanceByID(instanceID)
	if err != nil {
		if awserrors.IsNotFound(err) {
			log.Info("Instance already deleted", "InstanceID", *instanceID)
			s.scope.SetInstanceStatus(infrav1.InstanceStatusTerminated)
			return nil
		}
		return errors.Wrap(err, "failed to describe instance")
	}

	// If instance is already terminating or terminated, update status
	state := aws.StringValue(instance.State.Name)
	log.Info(fmt.Sprintf("Instance is %s", state), "InstanceID", *instanceID)
	s.scope.SetInstanceStatus(infrav1.InstanceStatus(strings.ToUpper(state)))

	if state == ec2.InstanceStateNameTerminated || state == ec2.InstanceStateNameShuttingDown {
		return nil
	}

	// Initiate termination if not already in progress
	log.Info("Terminating EC2 instance", "InstanceID", *instanceID)
	err = s.Client.TerminateInstance(instanceID)
	if err != nil {
		return nil
	}

	// Set status to Terminating
	s.scope.SetInstanceStatus("Terminating")
	log.Info("Termination initiated for EC2 instance", "InstanceID", *instanceID)

	return nil
}

func (s *Service) createOrGetInstance(ctx context.Context) (*ec2.Instance, error) {
	log := log.FromContext(ctx)
	instanceID := s.scope.GetInstanceID()
	// Check if we already have an InstanceID
	if instanceID != nil {
		// Describe the instance
		instance, err := s.Client.FindInstanceByID(instanceID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to find instance by ID")
		}
		if instance != nil {
			return instance, nil
		}
	}
	// Update scope with InstanceID
	params := awsforge.CreateInstanceParams{
		Name:            s.scope.Name(),
		InstanceType:    s.scope.InstanceType(),
		AmiID:           s.scope.AMI(),
		SubnetID:        *s.scope.SubnetID(),
		SecurityGroupID: *s.scope.SecurityGroupID(),
		Userdata:        *s.scope.UserData(),
		PublicIP:        *s.scope.PublicIP(),
	}
	instance, err := s.Client.CreateInstance(params)
	if err != nil {
		return nil, err
	}
	log.Info("EC2 instance created", "InstanceID", *instance.InstanceId)
	return instance, nil
}
