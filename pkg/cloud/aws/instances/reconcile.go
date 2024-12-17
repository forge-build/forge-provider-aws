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
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	infrav1 "github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
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

	var publicIP string
	if instance.NetworkInterfaces[0].Association.PublicIp != nil {
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
	isManaged, err := s.isManagedInstance(ctx, *instanceID)
	if err != nil {
		return errors.Wrap(err, "failed to check if instance is managed")
	}

	if !isManaged {
		log.Info("Instance is not managed by forge, skipping deletion", "InstanceID", *instanceID)
		return nil
	}

	// Check current state of the instance
	instance, err := s.findInstanceByID(ctx, *instanceID)
	if err != nil {
		if isInstanceNotFound(err) {
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
	_, err = s.ec2Client.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: []*string{instanceID},
	})
	if err != nil {
		return errors.Wrap(err, "failed to terminate EC2 instance")
	}

	// Set status to Terminating
	s.scope.SetInstanceStatus("Terminating")
	log.Info("Termination initiated for EC2 instance", "InstanceID", *instanceID)

	return nil
}

func (s *Service) createOrGetInstance(ctx context.Context) (*ec2.Instance, error) {
	log := log.FromContext(ctx)

	// Check if we already have an InstanceID
	instanceID := s.scope.GetInstanceID()
	if instanceID != nil {
		// Describe the instance
		instance, err := s.findInstanceByID(ctx, *instanceID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to find instance by ID")
		}
		if instance != nil {
			log.Info("Using existing EC2 instance", "InstanceID", *instance.InstanceId)
			return instance, nil
		}
		log.Info("Instance ID is set but instance not found, will create a new one")
	}
	// Extract parameters from scope
	amiID := s.scope.AMI()
	if amiID == "" {
		return nil, errors.New("AMI ID not provided")
	}

	instanceType := s.scope.InstanceType()
	if instanceType == "" {
		return nil, errors.New("Instance type not provided")
	}

	subnetID := s.scope.SubnetID()
	sgID := s.scope.SecurityGroupID()

	// Build tags
	instanceName := s.scope.Name()
	tags := []*ec2.Tag{
		{Key: aws.String("Name"), Value: aws.String(instanceName)},
		{Key: aws.String("forge-managed"), Value: aws.String("true")},
	}

	// RunInstances input
	runInput := &ec2.RunInstancesInput{
		ImageId:      aws.String(amiID),
		InstanceType: aws.String(instanceType),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		UserData:     s.scope.UserData(),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeInstance),
				Tags:         tags,
			},
		},
	}

	// Network configuration to assign public IP
	networkInterface := &ec2.InstanceNetworkInterfaceSpecification{
		DeviceIndex:              aws.Int64(0), // Primary network interface
		AssociatePublicIpAddress: s.scope.PublicIP(),
	}

	if subnetID != nil {
		networkInterface.SubnetId = subnetID
	}

	if sgID != nil {
		networkInterface.Groups = aws.StringSlice([]string{*sgID})
	}

	runInput.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{networkInterface}

	// Block devices, networking config can be added here as needed.

	log.Info("Running new EC2 instance", "ImageID", amiID, "InstanceType", instanceType)
	runOutput, err := s.ec2Client.RunInstances(runInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run EC2 instance")
	}

	if len(runOutput.Instances) == 0 {
		return nil, errors.New("no instances launched")
	}

	instance := runOutput.Instances[0]
	// Update scope with InstanceID
	s.scope.SetInstanceID(instance.InstanceId)
	log.Info("EC2 instance created", "InstanceID", *instance.InstanceId)

	return instance, nil
}

func (s *Service) findInstanceByID(_ context.Context, instanceID string) (*ec2.Instance, error) {
	output, err := s.ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	})
	if err != nil {
		if isInstanceNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, res := range output.Reservations {
		for _, inst := range res.Instances {
			if aws.StringValue(inst.InstanceId) == instanceID {
				return inst, nil
			}
		}
	}

	return nil, nil
}

func (s *Service) isManagedInstance(_ context.Context, instanceID string) (bool, error) {
	output, err := s.ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	})
	if err != nil {
		if isInstanceNotFound(err) {
			// If the instance doesn't exist, no need to delete
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

func isInstanceNotFound(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "InvalidInstanceID.NotFound" {
			return true
		}
	}
	return false
}
