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

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/forge-build/forge-provider-aws/pkg/cloud"
)

// instancesInterface defines the EC2 operations needed for instances.
type instancesInterface interface {
	DescribeImagesWithContext(ctx context.Context, input *ec2.DescribeImagesInput, opts ...request.Option) (*ec2.DescribeImagesOutput, error)
	DeregisterImageWithContext(ctx context.Context, input *ec2.DeregisterImageInput, opts ...request.Option) (*ec2.DeregisterImageOutput, error)
	CreateImageWithContext(ctx context.Context, input *ec2.CreateImageInput, opts ...request.Option) (*ec2.CreateImageOutput, error)
}

// Scope defines the methods needed from the calling context (e.g., BuildScope).
// This should return parameters needed to create, identify, and configure the instance.
type Scope interface {
	cloud.Build
	IsProvisionerReady() bool
	IsReady() bool
	SetArtifactRef(reference string)
	CreationDate() string
}

// Service implements networks reconciler.
type Service struct {
	scope     Scope
	ec2Client instancesInterface
}

var _ cloud.Reconciler = &Service{}

// New returns Service from given scope.
func New(scope Scope) *Service {
	return &Service{
		scope:     scope,
		ec2Client: scope.Cloud(),
	}
}
