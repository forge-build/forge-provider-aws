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

package images

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconcile ensures that a disk image (AMI) is created from an EC2 instance.
func (s *Service) Reconcile(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling image creation")

	// Ensure provisioner is ready
	if !s.scope.IsProvisionerReady() || s.scope.IsReady() {
		logger.Info("Not ready for exporting the image")
		return nil
	}

	instanceID := s.scope.GetInstanceID()
	if instanceID == nil {
		return errors.New("instance ID is not set, cannot create image")
	}

	amiName := s.scope.Name()
	logger.Info("Ensuring no existing AMI conflicts", "imageName", amiName)

	// Ensure no existing AMI conflicts
	if err := s.ensureAMIDoesNotExist(ctx, amiName); err != nil {
		return err
	}

	amiID, amiState, err := s.checkAMIStatus(ctx, amiName)
	if err != nil {
		return err
	}

	switch amiState {
	case "available":
		s.scope.SetArtifactRef(amiID)
		logger.Info("AMI is already available", "AMI ID", amiID)
	case "pending":
		logger.Info("AMI is still being created, waiting for readiness", "AMI ID", amiID)
	default:
		logger.Info("Creating AMI object...", amiName)
		err := s.createAMI(ctx, *instanceID, amiName)
		if err != nil {
			return err
		}
	}

	logger.Info("AMI reconciliation successful", "AMI ID", amiID)
	return nil
}

// ensureAMIDoesNotExist checks if an AMI exists and deletes it if its creation date is older than the Build's creation date.
func (s *Service) ensureAMIDoesNotExist(ctx context.Context, imageName string) error {
	logger := log.FromContext(ctx)

	// Describe AMIs with the given name
	output, err := s.ec2Client.DescribeImagesWithContext(ctx, &ec2.DescribeImagesInput{
		Owners: aws.StringSlice([]string{"self"}), // Owned AMIs
		Filters: []*ec2.Filter{
			{Name: aws.String("name"), Values: aws.StringSlice([]string{imageName})},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to describe AMIs")
	}
	buildCreationTime, err := time.Parse(time.RFC3339, s.scope.CreationDate())
	if err != nil {
		return err
	}
	// Loop through matching AMIs
	for _, image := range output.Images {
		amiID := aws.StringValue(image.ImageId)
		creationDateStr := aws.StringValue(image.CreationDate)

		// Parse AMI creation date
		amiCreationDate, err := time.Parse(time.RFC3339, creationDateStr)
		if err != nil {
			logger.Error(err, "Failed to parse AMI creation date", "AMI ID", amiID)
			continue
		}

		// Compare AMI creation date with Build's creation date
		if amiCreationDate.Before(buildCreationTime) {
			logger.Info("Deleting outdated AMI", "AMI ID", amiID, "CreationDate", creationDateStr)
			_, err := s.ec2Client.DeregisterImageWithContext(ctx, &ec2.DeregisterImageInput{
				ImageId: aws.String(amiID),
			})
			if err != nil {
				return errors.Wrapf(err, "failed to deregister outdated AMI %s", amiID)
			}
			logger.Info("Successfully deregistered outdated AMI", "AMI ID", amiID)
		} else {
			logger.Info("Existing AMI is up-to-date, skipping deletion", "AMI ID", amiID, "CreationDate", creationDateStr)
			return nil
		}
	}

	logger.Info("No existing AMI conflicts")
	return nil
}

// createAMI creates a new AMI from the instance's root volume.
func (s *Service) createAMI(ctx context.Context, instanceID, imageName string) error {
	input := &ec2.CreateImageInput{
		InstanceId:  aws.String(instanceID),
		Name:        aws.String(imageName),
		NoReboot:    aws.Bool(true), // Avoid rebooting the instance
		Description: aws.String(fmt.Sprintf("AMI created from instance %s", instanceID)),
	}

	_, err := s.ec2Client.CreateImageWithContext(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to create AMI")
	}

	return nil
}

func (s *Service) Delete(ctx context.Context) error {

	return nil
}

// checkAMIStatus checks if an AMI exists by name and returns its ID and state.
func (s *Service) checkAMIStatus(ctx context.Context, imageName string) (string, string, error) {
	output, err := s.ec2Client.DescribeImagesWithContext(ctx, &ec2.DescribeImagesInput{
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
