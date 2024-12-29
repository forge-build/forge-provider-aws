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

	"github.com/pkg/errors"
)

// Reconcile ensures that a disk image (AMI) is created from an EC2 instance.
func (s *Service) Reconcile(ctx context.Context) error {
	s.Log.V(1).Info("Reconciling image creation")

	// Ensure provisioner is ready
	if !s.scope.IsProvisionerReady() || s.scope.IsReady() {
		s.Log.V(1).Info("Not ready for exporting the image")
		return nil
	}

	instanceID := s.scope.GetInstanceID()
	if instanceID == nil {
		return errors.New("instance ID is not set, cannot create image")
	}

	amiName := s.scope.Name()

	// Ensure no existing AMI conflicts
	s.Log.V(1).Info("Ensuring no existing AMI conflicts", "imageName", amiName)
	if err := s.Client.EnsureAMIDoesNotExist(ctx, amiName, s.scope.CreationDate()); err != nil {
		return err
	}

	amiID, amiState, err := s.Client.CheckAMIStatus(ctx, amiName)
	if err != nil {
		return err
	}

	switch amiState {
	case "available":
		s.scope.SetArtifactRef(amiID)
		s.Log.Info("AMI is already available", "AMI ID", amiID)
	case "pending":
		s.Log.Info("AMI is still being created, waiting for readiness", "AMI ID", amiID)
	default:
		s.Log.Info("Creating AMI object...", amiName)
		err := s.Client.CreateAMI(ctx, *instanceID, amiName)
		if err != nil {
			return err
		}
	}

	s.Log.Info("AMI reconciliation successful", "AMI ID", amiID)
	return nil
}

func (s *Service) Delete(ctx context.Context) error {
	return nil
}
