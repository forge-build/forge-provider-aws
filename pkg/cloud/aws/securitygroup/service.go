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
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/forge-build/forge-provider-aws/pkg/cloud"
)

type securityGroupInterface interface {
	DeleteSecurityGroup(input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error)
	CreateSecurityGroup(input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngress(input *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	DescribeSecurityGroups(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
}

type Scope interface {
	cloud.Build
	VPCSpec() *ec2.CreateVpcInput
	VPCName() *string
	SecurityGroupName() *string
	SecurityGroupID() *string
	SetSecurityGroupID(id *string)
}

// Service implements networks reconciler.
type Service struct {
	scope    Scope
	sgClient securityGroupInterface
}

var _ cloud.Reconciler = &Service{}

// New returns Service from given scope.
func New(scope Scope) *Service {
	return &Service{
		scope:    scope,
		sgClient: scope.Cloud(),
	}
}
