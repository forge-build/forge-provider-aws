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

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/forge-build/forge-provider-aws/pkg/cloud"
)

type vpcsInterface interface {
	FindVPCByIDOrName(vpcID, vpcName *string) (*ec2.Vpc, error)
	IsManagedVPC(vpcID *string) (bool, error)
	DeleteVPC(vpcID *string) error
	CreateVPC(input *ec2.CreateVpcInput) (*ec2.Vpc, error)
	DetachAndDeleteInternetGateway(vpcID *string) error
	CreateOrGetInternetGateway(ctx context.Context, vpcID string) (*ec2.InternetGateway, error)
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
	Client vpcsInterface
}

var _ cloud.Reconciler = &Service{}

// New returns Service from given scope.
func New(scope Scope) *Service {
	return &Service{
		scope:  scope,
		Client: scope.Cloud(),
	}
}
