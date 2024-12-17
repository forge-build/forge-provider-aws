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
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/forge-build/forge-provider-aws/pkg/cloud"
)

type subnetsInterface interface {
	CreateSubnet(input *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error)
	DeleteSubnet(input *ec2.DeleteSubnetInput) (*ec2.DeleteSubnetOutput, error)
	DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
}

type vpcsInterface interface {
	CreateVpc(input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error)
	DeleteVpc(input *ec2.DeleteVpcInput) (*ec2.DeleteVpcOutput, error)
	DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
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
	scope        Scope
	subnetClient subnetsInterface
	vpcClient    vpcsInterface
}

var _ cloud.Reconciler = &Service{}

// New returns Service from given scope.
func New(scope Scope) *Service {
	return &Service{
		scope:        scope,
		subnetClient: scope.Cloud(),
		vpcClient:    scope.Cloud(),
	}
}
