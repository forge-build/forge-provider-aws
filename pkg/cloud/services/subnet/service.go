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

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/forge-build/forge-provider-aws/pkg/cloud"
	"github.com/go-logr/logr"
)

const ServiceName = "subnets-reconciler"

type subnetsInterface interface {
	CreateSubnet(ctx context.Context, vpcName string, vpcID *string) (*ec2.Subnet, error)
	DeleteSubnet(ctx context.Context, subnetID *string) error
	IsManagedSubnet(ctx context.Context, subnetID string) (bool, error)
	FindSubnetByID(ctx context.Context, subnetID string) (*ec2.Subnet, error)
}

type client interface {
	subnetsInterface
}

type Scope interface {
	cloud.Build
	Region() string
	VPCSpec() *ec2.CreateVpcInput
	VPCID() *string
	VPCName() *string
}

// Service implements networks reconciler.
type Service struct {
	scope  Scope
	Client client
	Log    logr.Logger
}

var _ cloud.Reconciler = &Service{}

// New returns Service from given scope.
func New(scope Scope) *Service {
	return &Service{
		scope:  scope,
		Client: scope.Cloud(),
		Log:    scope.Log(ServiceName),
	}
}
