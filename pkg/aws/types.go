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

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/service/ec2"
)

type CreateInstanceParams struct {
	Name            string
	AmiID           string
	InstanceType    string
	Userdata        string
	PublicIP        bool
	SubnetID        string
	SecurityGroupID string
}

type Interface interface {

	// EC2 Instance
	IsManagedInstance(instanceID *string) (bool, error)
	FindInstanceByID(instanceID *string) (*ec2.Instance, error)
	CreateInstance(input CreateInstanceParams) (*ec2.Instance, error)
	TerminateInstance(instanceID *string) error

	// Network
	FindVPCByIDOrName(vpcID, vpcName *string) (*ec2.Vpc, error)
	IsManagedVPC(vpcID *string) (bool, error)
	DeleteVPC(vpcID *string) error
	CreateVPC(input *ec2.CreateVpcInput) (*ec2.Vpc, error)

	// Security Group
	CreateSecurityGroup(vpcID, sgName *string) (*ec2.CreateSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngress(sgID string) error
	IsManagedSecurityGroup(sgID string) (bool, error)
	DeleteSecurityGroup(sgID *string) error

	// Subnets
	CreateSubnet(ctx context.Context, vpcName string, vpcID *string) (*ec2.Subnet, error)
	DeleteSubnet(ctx context.Context, subnetID *string) error
	IsManagedSubnet(ctx context.Context, subnetID string) (bool, error)
	FindSubnetByID(ctx context.Context, subnetID string) (*ec2.Subnet, error)

	// InternetGateway
	DetachAndDeleteInternetGateway(vpcID *string) error
	CreateOrGetInternetGateway(ctx context.Context, vpcID string) (*ec2.InternetGateway, error)

	// AMI Image
	CreateAMI(ctx context.Context, instanceID, imageName string) error
	EnsureAMIDoesNotExist(ctx context.Context, imageName, creationDate string) error
	ListAMIs(ctx context.Context, imageName string) ([]*ec2.Image, error)
	CheckAMIStatus(ctx context.Context, imageName string) (string, string, error)
}
